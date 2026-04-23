package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Alert describes an adapter alert event.
type Alert struct {
	Name      string            `json:"name"`
	Severity  string            `json:"severity"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// AlertSink defines an alert delivery target.
type AlertSink interface {
	Send(ctx context.Context, alert Alert) error
}

// AlertManager applies cooldown suppression before alert delivery.
type AlertManager struct {
	sink     AlertSink
	cooldown time.Duration
	logger   *log.Logger

	mu   sync.Mutex
	last map[string]time.Time
}

// NewAlertManager creates an alert manager.
func NewAlertManager(sink AlertSink, cooldown time.Duration, logger *log.Logger) *AlertManager {
	if cooldown <= 0 {
		cooldown = defaultAlertCooldown
	}
	if logger == nil {
		logger = log.New(os.Stderr, "feishu-alert: ", log.LstdFlags)
	}
	return &AlertManager{
		sink:     sink,
		cooldown: cooldown,
		logger:   logger,
		last:     map[string]time.Time{},
	}
}

// Emit emits an alert if it is outside cooldown.
func (m *AlertManager) Emit(ctx context.Context, alert Alert) {
	if m == nil {
		return
	}
	name := strings.TrimSpace(alert.Name)
	if name == "" {
		return
	}
	now := alert.Timestamp
	if now.IsZero() {
		now = time.Now().UTC()
		alert.Timestamp = now
	}

	m.mu.Lock()
	lastAt, exists := m.last[name]
	if exists && now.Sub(lastAt) < m.cooldown {
		m.mu.Unlock()
		return
	}
	m.last[name] = now
	m.mu.Unlock()

	if m.sink == nil {
		m.logger.Printf("alert[%s] %s", name, strings.TrimSpace(alert.Message))
		return
	}
	if err := m.sink.Send(ctx, alert); err != nil {
		m.logger.Printf("send alert[%s] failed: %v", name, err)
	}
}

// WebhookAlertSink sends alerts to a webhook endpoint.
type WebhookAlertSink struct {
	URL        string
	HTTPClient *http.Client
}

// Send delivers an alert to webhook.
func (s WebhookAlertSink) Send(ctx context.Context, alert Alert) error {
	url := strings.TrimSpace(s.URL)
	if url == "" {
		return nil
	}
	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	payload := map[string]any{
		"msg_type": "text",
		"content": map[string]string{
			"text": fmt.Sprintf(
				"[NeoCode-Feishu-Alert] name=%s severity=%s message=%s labels=%v time=%s",
				alert.Name,
				alert.Severity,
				alert.Message,
				alert.Labels,
				alert.Timestamp.UTC().Format(time.RFC3339),
			),
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("alert webhook status: %d", response.StatusCode)
	}
	return nil
}
