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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cnpgv1alpha1 "github.com/supporttools/cnpg-storage-manager/api/v1alpha1"
)

func TestAlertManager_Suppression(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	manager := NewAlertManager(client, nil)

	alert := &Alert{
		ClusterName:      "test-cluster",
		ClusterNamespace: "default",
		Severity:         AlertSeverityWarning,
		Message:          "Test alert",
		Timestamp:        time.Now(),
	}

	// Should not be suppressed initially
	if manager.isSuppressed(alert) {
		t.Error("expected alert to not be suppressed initially")
	}

	// Add suppression
	manager.addSuppression(alert)

	// Should be suppressed now
	if !manager.isSuppressed(alert) {
		t.Error("expected alert to be suppressed after adding")
	}

	// Different severity should not be suppressed
	alert2 := &Alert{
		ClusterName:      "test-cluster",
		ClusterNamespace: "default",
		Severity:         AlertSeverityCritical,
		Message:          "Test alert",
		Timestamp:        time.Now(),
	}
	if manager.isSuppressed(alert2) {
		t.Error("expected different severity to not be suppressed")
	}

	// Clear suppression
	manager.ClearSuppression("default", "test-cluster")
	if manager.isSuppressed(alert) {
		t.Error("expected alert to not be suppressed after clearing")
	}
}

func TestAlertManager_AlertmanagerPayload(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	var receivedPayload []map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/alerts" {
			t.Errorf("expected path /api/v1/alerts, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&receivedPayload); err != nil {
			t.Errorf("failed to decode payload: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	channels := []cnpgv1alpha1.AlertChannel{
		{
			Type:     cnpgv1alpha1.AlertChannelTypeAlertmanager,
			Endpoint: server.URL,
		},
	}
	manager := NewAlertManager(client, channels)

	alert := &Alert{
		ClusterName:      "test-cluster",
		ClusterNamespace: "default",
		Severity:         AlertSeverityCritical,
		Message:          "Storage usage critical",
		Details: map[string]string{
			"usage_percent": "95",
		},
		Timestamp: time.Now(),
	}

	err := manager.sendToAlertmanager(context.Background(), alert, channels[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(receivedPayload) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(receivedPayload))
	}

	labels := receivedPayload[0]["labels"].(map[string]interface{})
	if labels["alertname"] != "CNPGStorageAlert" {
		t.Errorf("expected alertname CNPGStorageAlert, got %v", labels["alertname"])
	}
	if labels["cluster"] != "test-cluster" {
		t.Errorf("expected cluster test-cluster, got %v", labels["cluster"])
	}
	if labels["severity"] != "critical" {
		t.Errorf("expected severity critical, got %v", labels["severity"])
	}
}

func TestAlertManager_SlackPayload(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	var receivedPayload map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&receivedPayload); err != nil {
			t.Errorf("failed to decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create secret with webhook URL
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "slack-webhook",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"webhook-url": []byte(server.URL),
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(secret).Build()
	channels := []cnpgv1alpha1.AlertChannel{
		{
			Type:          cnpgv1alpha1.AlertChannelTypeSlack,
			WebhookSecret: "default/slack-webhook",
			Channel:       "#alerts",
		},
	}
	manager := NewAlertManager(client, channels)

	alert := &Alert{
		ClusterName:      "test-cluster",
		ClusterNamespace: "default",
		Severity:         AlertSeverityWarning,
		Message:          "Storage usage warning",
		Timestamp:        time.Now(),
	}

	err := manager.sendToSlack(context.Background(), alert, channels[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedPayload["channel"] != "#alerts" {
		t.Errorf("expected channel #alerts, got %v", receivedPayload["channel"])
	}

	attachments := receivedPayload["attachments"].([]interface{})
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
}

func TestAlertManager_PagerDutySecretRetrieval(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create secret with routing key
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pagerduty-key",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"routing-key": []byte("test-routing-key"),
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(secret).Build()
	channels := []cnpgv1alpha1.AlertChannel{
		{
			Type:             cnpgv1alpha1.AlertChannelTypePagerDuty,
			RoutingKeySecret: "default/pagerduty-key",
		},
	}
	manager := NewAlertManager(client, channels)

	ctx := context.Background()

	// Test secret retrieval
	key, err := manager.getSecretValue(ctx, "default/pagerduty-key", "routing-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "test-routing-key" {
		t.Errorf("expected test-routing-key, got %s", key)
	}

	// Test missing secret
	_, err = manager.getSecretValue(ctx, "default/nonexistent", "key")
	if err == nil {
		t.Error("expected error for missing secret")
	}

	// Test missing key in secret
	_, err = manager.getSecretValue(ctx, "default/pagerduty-key", "nonexistent-key")
	if err == nil {
		t.Error("expected error for missing key")
	}
}

func TestBuildSlackFields(t *testing.T) {
	alert := &Alert{
		ClusterName:      "test-cluster",
		ClusterNamespace: "default",
		Severity:         AlertSeverityCritical,
		Details: map[string]string{
			"usage_percent": "95",
			"threshold":     "85",
		},
	}

	fields := buildSlackFields(alert)

	// Should have at least cluster and severity fields
	if len(fields) < 2 {
		t.Errorf("expected at least 2 fields, got %d", len(fields))
	}

	// Check cluster field
	found := false
	for _, f := range fields {
		if f["title"] == "Cluster" {
			found = true
			if f["value"] != "default/test-cluster" {
				t.Errorf("expected cluster value default/test-cluster, got %v", f["value"])
			}
		}
	}
	if !found {
		t.Error("expected to find Cluster field")
	}
}
