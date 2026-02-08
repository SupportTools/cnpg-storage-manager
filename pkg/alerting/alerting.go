/*
Copyright 2025 SupportTools.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
	"github.com/supporttools/cnpg-storage-manager/pkg/metrics"
)

// AlertSeverity defines the severity of an alert
type AlertSeverity string

const (
	// AlertSeverityWarning is a warning alert
	AlertSeverityWarning AlertSeverity = "warning"
	// AlertSeverityCritical is a critical alert
	AlertSeverityCritical AlertSeverity = "critical"
	// AlertSeverityEmergency is an emergency alert
	AlertSeverityEmergency AlertSeverity = "emergency"
)

// Alert represents an alert to be sent
type Alert struct {
	ClusterName      string
	ClusterNamespace string
	Severity         AlertSeverity
	Message          string
	Details          map[string]string
	Timestamp        time.Time
}

// AlertManager handles sending alerts through various channels
type AlertManager struct {
	client          client.Client
	httpClient      *http.Client
	channels        []cnpgv1alpha1.AlertChannel
	suppressionMap  map[string]time.Time
	suppressionLock sync.RWMutex
}

// NewAlertManager creates a new alert manager
func NewAlertManager(c client.Client, channels []cnpgv1alpha1.AlertChannel) *AlertManager {
	return &AlertManager{
		client:         c,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		channels:       channels,
		suppressionMap: make(map[string]time.Time),
	}
}

// SendAlert sends an alert through all configured channels
func (m *AlertManager) SendAlert(ctx context.Context, alert *Alert) error {
	logger := log.FromContext(ctx)

	// Check if alert is suppressed
	if m.isSuppressed(alert) {
		logger.V(1).Info("Alert suppressed", "cluster", alert.ClusterName, "severity", alert.Severity)
		metrics.RecordAlertSuppressed(alert.ClusterName, alert.ClusterNamespace, "duplicate")
		return nil
	}

	var lastErr error
	sentCount := 0

	for _, channel := range m.channels {
		var err error
		switch channel.Type {
		case cnpgv1alpha1.AlertChannelTypeAlertmanager:
			err = m.sendToAlertmanager(ctx, alert, channel)
		case cnpgv1alpha1.AlertChannelTypeSlack:
			err = m.sendToSlack(ctx, alert, channel)
		case cnpgv1alpha1.AlertChannelTypePagerDuty:
			err = m.sendToPagerDuty(ctx, alert, channel)
		default:
			logger.Info("Unknown alert channel type", "type", channel.Type)
			continue
		}

		if err != nil {
			logger.Error(err, "Failed to send alert", "channel", channel.Type)
			lastErr = err
		} else {
			sentCount++
			metrics.RecordAlertSent(alert.ClusterName, alert.ClusterNamespace, string(alert.Severity), string(channel.Type))
		}
	}

	// Add to suppression map
	m.addSuppression(alert)

	if sentCount == 0 && lastErr != nil {
		return fmt.Errorf("failed to send alert through any channel: %w", lastErr)
	}

	return nil
}

// sendToAlertmanager sends an alert to Prometheus Alertmanager
func (m *AlertManager) sendToAlertmanager(ctx context.Context, alert *Alert, channel cnpgv1alpha1.AlertChannel) error {
	if channel.Endpoint == "" {
		return fmt.Errorf("alertmanager endpoint not configured")
	}

	alertPayload := []map[string]interface{}{
		{
			"labels": map[string]string{
				"alertname": "CNPGStorageAlert",
				"cluster":   alert.ClusterName,
				"namespace": alert.ClusterNamespace,
				"severity":  string(alert.Severity),
			},
			"annotations": map[string]string{
				"summary":     alert.Message,
				"description": fmt.Sprintf("Storage alert for CNPG cluster %s/%s", alert.ClusterNamespace, alert.ClusterName),
			},
			"generatorURL": fmt.Sprintf("http://cnpg-storage-manager/clusters/%s/%s", alert.ClusterNamespace, alert.ClusterName),
		},
	}

	// Add custom details to labels
	if labels, ok := alertPayload[0]["labels"].(map[string]string); ok {
		for k, v := range alert.Details {
			labels[k] = v
		}
	}

	body, err := json.Marshal(alertPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal alertmanager payload: %w", err)
	}

	endpoint := channel.Endpoint
	if endpoint[len(endpoint)-1] != '/' {
		endpoint += "/"
	}
	endpoint += "api/v2/alerts"

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create alertmanager request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send alertmanager request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("alertmanager returned status %d", resp.StatusCode)
	}

	return nil
}

// sendToSlack sends an alert to Slack
func (m *AlertManager) sendToSlack(ctx context.Context, alert *Alert, channel cnpgv1alpha1.AlertChannel) error {
	// Get webhook URL from secret
	webhookURL, err := m.getSecretValue(ctx, channel.WebhookSecret, "webhook-url")
	if err != nil {
		return fmt.Errorf("failed to get slack webhook URL: %w", err)
	}

	// Build Slack message
	color := "#36a64f" // green
	switch alert.Severity {
	case AlertSeverityWarning:
		color = "#ffcc00" // yellow
	case AlertSeverityCritical:
		color = "#ff6600" // orange
	case AlertSeverityEmergency:
		color = "#ff0000" // red
	}

	payload := map[string]interface{}{
		"channel": channel.Channel,
		"attachments": []map[string]interface{}{
			{
				"color":  color,
				"title":  fmt.Sprintf("CNPG Storage Alert - %s", alert.Severity),
				"text":   alert.Message,
				"fields": buildSlackFields(alert),
				"ts":     alert.Timestamp.Unix(),
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send slack request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	return nil
}

// sendToPagerDuty sends an alert to PagerDuty
func (m *AlertManager) sendToPagerDuty(ctx context.Context, alert *Alert, channel cnpgv1alpha1.AlertChannel) error {
	// Get routing key from secret
	routingKey, err := m.getSecretValue(ctx, channel.RoutingKeySecret, "routing-key")
	if err != nil {
		return fmt.Errorf("failed to get pagerduty routing key: %w", err)
	}

	// Map severity to PagerDuty severity
	pdSeverity := "info"
	switch alert.Severity {
	case AlertSeverityWarning:
		pdSeverity = "warning"
	case AlertSeverityCritical:
		pdSeverity = "error"
	case AlertSeverityEmergency:
		pdSeverity = "critical"
	}

	payload := map[string]interface{}{
		"routing_key":  routingKey,
		"event_action": "trigger",
		"dedup_key":    fmt.Sprintf("cnpg-storage-%s-%s", alert.ClusterNamespace, alert.ClusterName),
		"payload": map[string]interface{}{
			"summary":   alert.Message,
			"severity":  pdSeverity,
			"source":    fmt.Sprintf("%s/%s", alert.ClusterNamespace, alert.ClusterName),
			"component": "cnpg-storage-manager",
			"group":     "storage",
			"class":     "storage-alert",
			"custom_details": map[string]interface{}{
				"cluster_name":      alert.ClusterName,
				"cluster_namespace": alert.ClusterNamespace,
				"severity":          string(alert.Severity),
				"details":           alert.Details,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal pagerduty payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://events.pagerduty.com/v2/enqueue", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create pagerduty request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send pagerduty request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("pagerduty returned status %d", resp.StatusCode)
	}

	return nil
}

// getSecretValue retrieves a value from a Kubernetes secret
func (m *AlertManager) getSecretValue(ctx context.Context, secretName, key string) (string, error) {
	if secretName == "" {
		return "", fmt.Errorf("secret name is empty")
	}

	// Parse namespace/name if provided
	namespace := "default"
	name := secretName
	if idx := bytes.IndexByte([]byte(secretName), '/'); idx != -1 {
		namespace = secretName[:idx]
		name = secretName[idx+1:]
	}

	var secret corev1.Secret
	if err := m.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &secret); err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, name, err)
	}

	value, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %s not found in secret %s/%s", key, namespace, name)
	}

	return string(value), nil
}

// isSuppressed checks if an alert should be suppressed
func (m *AlertManager) isSuppressed(alert *Alert) bool {
	m.suppressionLock.RLock()
	defer m.suppressionLock.RUnlock()

	key := fmt.Sprintf("%s/%s/%s", alert.ClusterNamespace, alert.ClusterName, alert.Severity)
	lastSent, ok := m.suppressionMap[key]
	if !ok {
		return false
	}

	// Suppress if sent within the last 5 minutes
	return time.Since(lastSent) < 5*time.Minute
}

// addSuppression adds an alert to the suppression map
func (m *AlertManager) addSuppression(alert *Alert) {
	m.suppressionLock.Lock()
	defer m.suppressionLock.Unlock()

	key := fmt.Sprintf("%s/%s/%s", alert.ClusterNamespace, alert.ClusterName, alert.Severity)
	m.suppressionMap[key] = time.Now()
}

// ClearSuppression clears suppression for a specific cluster
func (m *AlertManager) ClearSuppression(clusterNamespace, clusterName string) {
	m.suppressionLock.Lock()
	defer m.suppressionLock.Unlock()

	prefix := fmt.Sprintf("%s/%s/", clusterNamespace, clusterName)
	for key := range m.suppressionMap {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(m.suppressionMap, key)
		}
	}
}

// buildSlackFields builds Slack attachment fields from alert details
func buildSlackFields(alert *Alert) []map[string]interface{} {
	fields := []map[string]interface{}{
		{
			"title": "Cluster",
			"value": fmt.Sprintf("%s/%s", alert.ClusterNamespace, alert.ClusterName),
			"short": true,
		},
		{
			"title": "Severity",
			"value": string(alert.Severity),
			"short": true,
		},
	}

	for k, v := range alert.Details {
		fields = append(fields, map[string]interface{}{
			"title": k,
			"value": v,
			"short": true,
		})
	}

	return fields
}

// UpdateChannels updates the configured alert channels
func (m *AlertManager) UpdateChannels(channels []cnpgv1alpha1.AlertChannel) {
	m.channels = channels
}
