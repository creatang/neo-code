package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"neo-code/internal/gateway"
)

const (
	defaultFeishuHTTPTimeout = 8 * time.Second
)

// Messenger 抽象飞书回写通道。
type Messenger interface {
	SendText(ctx context.Context, route TenantRoute, target MessageTarget, text string) error
	SendPermissionCard(ctx context.Context, route TenantRoute, target MessageTarget, card PermissionCardData) error
}

// HTTPMessengerOptions 描述飞书 HTTP 回写配置。
type HTTPMessengerOptions struct {
	BaseURL          string
	DefaultAppID     string
	DefaultAppSecret string
	HTTPClient       *http.Client
	Logger           *log.Logger
}

type cachedTenantToken struct {
	Token    string
	ExpireAt time.Time
}

// HTTPMessenger 通过飞书开放接口发消息。
type HTTPMessenger struct {
	baseURL          string
	defaultAppID     string
	defaultAppSecret string
	httpClient       *http.Client
	logger           *log.Logger

	mu         sync.Mutex
	tokenCache map[string]cachedTenantToken
}

// NewHTTPMessenger 创建默认回写实现。
func NewHTTPMessenger(options HTTPMessengerOptions) *HTTPMessenger {
	baseURL := strings.TrimSpace(options.BaseURL)
	if baseURL == "" {
		baseURL = defaultFeishuBaseURL
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultFeishuHTTPTimeout}
	}
	logger := options.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "feishu-messenger: ", log.LstdFlags)
	}
	return &HTTPMessenger{
		baseURL:          strings.TrimRight(baseURL, "/"),
		defaultAppID:     strings.TrimSpace(options.DefaultAppID),
		defaultAppSecret: strings.TrimSpace(options.DefaultAppSecret),
		httpClient:       client,
		logger:           logger,
		tokenCache:       map[string]cachedTenantToken{},
	}
}

// SendText 发送纯文本消息。
func (m *HTTPMessenger) SendText(ctx context.Context, route TenantRoute, target MessageTarget, text string) error {
	if m == nil {
		return errors.New("feishu messenger is nil")
	}
	bodyText := strings.TrimSpace(text)
	if bodyText == "" {
		return nil
	}
	chatID := strings.TrimSpace(target.ChatID)
	if chatID == "" {
		return errors.New("missing chat_id for reply")
	}
	token, err := m.getTenantToken(ctx, route, target.TenantKey)
	if err != nil {
		return err
	}

	contentRaw, _ := json.Marshal(map[string]string{
		"text": bodyText,
	})
	requestBody := map[string]any{
		"receive_id": chatID,
		"msg_type":   "text",
		"content":    string(contentRaw),
	}
	endpoint := m.baseURL + "/open-apis/im/v1/messages?receive_id_type=chat_id"
	return m.postFeishuJSON(ctx, endpoint, token, requestBody, nil)
}

// SendPermissionCard sends an interactive approval card to Feishu.
func (m *HTTPMessenger) SendPermissionCard(
	ctx context.Context,
	route TenantRoute,
	target MessageTarget,
	card PermissionCardData,
) error {
	if m == nil {
		return errors.New("feishu messenger is nil")
	}
	requestID := strings.TrimSpace(card.RequestID)
	if requestID == "" {
		return errors.New("missing permission request_id")
	}
	chatID := strings.TrimSpace(target.ChatID)
	if chatID == "" {
		return errors.New("missing chat_id for permission card")
	}
	token, err := m.getTenantToken(ctx, route, target.TenantKey)
	if err != nil {
		return err
	}

	payload := buildPermissionCardPayload(card)
	cardRaw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode permission card payload: %w", err)
	}
	requestBody := map[string]any{
		"receive_id": chatID,
		"msg_type":   "interactive",
		"content":    string(cardRaw),
	}

	endpoint := m.baseURL + "/open-apis/im/v1/messages?receive_id_type=chat_id"
	return m.postFeishuJSON(ctx, endpoint, token, requestBody, nil)
}

func buildPermissionCardPayload(card PermissionCardData) map[string]any {
	requestID := strings.TrimSpace(card.RequestID)
	title := "NeoCode 权限审批"
	detail := formatPermissionCardDetail(card)

	button := func(text string, decision string, buttonType string) map[string]any {
		return map[string]any{
			"tag":  "button",
			"type": buttonType,
			"text": map[string]any{
				"tag":     "plain_text",
				"content": text,
			},
			"value": map[string]any{
				"request_id": requestID,
				"decision":   decision,
			},
		}
	}

	return map[string]any{
		"config": map[string]any{
			"wide_screen_mode": true,
		},
		"header": map[string]any{
			"template": "orange",
			"title": map[string]any{
				"tag":     "plain_text",
				"content": title,
			},
		},
		"elements": []map[string]any{
			{
				"tag":     "markdown",
				"content": detail,
			},
			{
				"tag": "action",
				"actions": []map[string]any{
					button("允许本次", string(gateway.PermissionResolutionAllowOnce), "primary"),
					button("允许本会话", string(gateway.PermissionResolutionAllowSession), "default"),
					button("拒绝", string(gateway.PermissionResolutionReject), "danger"),
				},
			},
			{
				"tag": "note",
				"elements": []map[string]any{
					{
						"tag":     "plain_text",
						"content": "点击按钮后，回调携带 request_id/decision 到适配器。",
					},
				},
			},
		},
	}
}

func formatPermissionCardDetail(card PermissionCardData) string {
	lines := []string{
		"**审批请求**",
		fmt.Sprintf("- request_id: `%s`", strings.TrimSpace(card.RequestID)),
	}
	if value := strings.TrimSpace(card.ToolName); value != "" {
		lines = append(lines, fmt.Sprintf("- tool: `%s`", value))
	}
	if value := strings.TrimSpace(card.ActionType); value != "" {
		lines = append(lines, fmt.Sprintf("- action: `%s`", value))
	}
	if value := strings.TrimSpace(card.Operation); value != "" {
		lines = append(lines, fmt.Sprintf("- operation: `%s`", value))
	}
	if value := strings.TrimSpace(card.TargetType); value != "" {
		lines = append(lines, fmt.Sprintf("- target_type: `%s`", value))
	}
	if value := strings.TrimSpace(card.Target); value != "" {
		lines = append(lines, fmt.Sprintf("- target: `%s`", value))
	}
	if value := strings.TrimSpace(card.Reason); value != "" {
		lines = append(lines, fmt.Sprintf("- reason: %s", value))
	}
	if value := strings.TrimSpace(card.DecisionHint); value != "" {
		lines = append(lines, fmt.Sprintf("- hint: %s", value))
	}
	return strings.Join(lines, "\n")
}

func (m *HTTPMessenger) getTenantToken(ctx context.Context, route TenantRoute, tenantKey string) (string, error) {
	appID := strings.TrimSpace(route.AppID)
	if appID == "" {
		appID = m.defaultAppID
	}
	appSecret := strings.TrimSpace(route.AppSecret)
	if appSecret == "" {
		appSecret = m.defaultAppSecret
	}
	if appID == "" || appSecret == "" {
		return "", errors.New("feishu app credentials are not configured")
	}

	cacheKey := strings.TrimSpace(tenantKey) + "|" + appID
	now := time.Now().UTC()
	m.mu.Lock()
	if cached, exists := m.tokenCache[cacheKey]; exists && strings.TrimSpace(cached.Token) != "" && now.Before(cached.ExpireAt) {
		m.mu.Unlock()
		return cached.Token, nil
	}
	m.mu.Unlock()

	requestBody := map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	}
	var response struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	endpoint := m.baseURL + "/open-apis/auth/v3/tenant_access_token/internal"
	if err := m.postFeishuJSON(ctx, endpoint, "", requestBody, &response); err != nil {
		return "", err
	}
	if response.Code != 0 || strings.TrimSpace(response.TenantAccessToken) == "" {
		return "", fmt.Errorf("request tenant access token failed: code=%d msg=%s", response.Code, strings.TrimSpace(response.Msg))
	}
	expireSec := response.Expire
	if expireSec <= 0 {
		expireSec = 7200
	}
	expireAt := now.Add(time.Duration(expireSec-120) * time.Second)

	m.mu.Lock()
	m.tokenCache[cacheKey] = cachedTenantToken{
		Token:    strings.TrimSpace(response.TenantAccessToken),
		ExpireAt: expireAt,
	}
	m.mu.Unlock()
	return strings.TrimSpace(response.TenantAccessToken), nil
}

func (m *HTTPMessenger) postFeishuJSON(
	ctx context.Context,
	endpoint string,
	accessToken string,
	requestBody any,
	responseBody any,
) error {
	if m == nil {
		return errors.New("feishu messenger is nil")
	}
	raw, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("encode feishu request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("build feishu request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	if strings.TrimSpace(accessToken) != "" {
		request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	}

	response, err := m.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call feishu api: %w", err)
	}
	defer response.Body.Close()

	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	decoder := json.NewDecoder(response.Body)
	if responseBody == nil {
		if err := decoder.Decode(&body); err != nil {
			return fmt.Errorf("decode feishu response: %w", err)
		}
		if body.Code != 0 {
			return fmt.Errorf("feishu api failed: code=%d msg=%s", body.Code, strings.TrimSpace(body.Msg))
		}
	} else {
		if err := decoder.Decode(responseBody); err != nil {
			return fmt.Errorf("decode feishu response: %w", err)
		}
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("feishu api status: %d", response.StatusCode)
	}
	return nil
}
