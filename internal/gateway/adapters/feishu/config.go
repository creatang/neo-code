package feishu

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	defaultListenAddress        = "127.0.0.1:19091"
	defaultGatewayWSURL         = "ws://127.0.0.1:8080/ws"
	defaultGatewayWSOrigin      = "http://localhost:3000"
	defaultFeishuBaseURL        = "https://open.feishu.cn"
	defaultEventDedupeTTL       = 30 * time.Minute
	defaultSignatureMaxSkew     = 5 * time.Minute
	defaultWatchdogTimeout      = 2 * time.Minute
	defaultPingInterval         = 20 * time.Second
	defaultRequestTimeout       = 8 * time.Second
	defaultChunkFlushInterval   = 900 * time.Millisecond
	defaultChunkFlushMaxChars   = 420
	defaultReconnectBaseBackoff = 200 * time.Millisecond
	defaultReconnectMaxBackoff  = 3 * time.Second
	defaultAlertCooldown        = 2 * time.Minute
	defaultReconnectAlertThresh = 5
	defaultAuthFailureThresh    = 3
	defaultBacklogAlertThresh   = 30
)

// TenantRoute 描述按租户/群的路由策略。
type TenantRoute struct {
	Enabled        bool   `json:"enabled"`
	Streaming      bool   `json:"streaming"`
	Workdir        string `json:"workdir,omitempty"`
	AppID          string `json:"app_id,omitempty"`
	AppSecret      string `json:"app_secret,omitempty"`
	ReplyReceiveBy string `json:"reply_receive_by,omitempty"`
}

// RouteFile 是路由文件结构。
type RouteFile struct {
	Default TenantRoute            `json:"default"`
	Tenants map[string]TenantRoute `json:"tenants,omitempty"`
	Chats   map[string]TenantRoute `json:"chats,omitempty"`
}

// Config 是飞书适配器总配置。
type Config struct {
	ListenAddress        string
	GatewayWSURL         string
	GatewayWSOrigin      string
	GatewayTokenFile     string
	FeishuBaseURL        string
	SigningSecret        string
	SignatureMaxSkew     time.Duration
	EventDedupeTTL       time.Duration
	RequestTimeout       time.Duration
	WatchdogTimeout      time.Duration
	PingInterval         time.Duration
	ChunkFlushInterval   time.Duration
	ChunkFlushMaxChars   int
	StreamingEnabled     bool
	Routes               RouteFile
	ReconnectBaseBackoff time.Duration
	ReconnectMaxBackoff  time.Duration
	DedupeStoreFile      string
	AlertWebhookURL      string
	AlertCooldown        time.Duration
	ReconnectAlertThresh int
	AuthFailureThresh    int
	BacklogAlertThresh   int
}

// DefaultConfig 返回带默认值的配置。
func DefaultConfig() Config {
	return Config{
		ListenAddress:        defaultListenAddress,
		GatewayWSURL:         defaultGatewayWSURL,
		GatewayWSOrigin:      defaultGatewayWSOrigin,
		FeishuBaseURL:        defaultFeishuBaseURL,
		SignatureMaxSkew:     defaultSignatureMaxSkew,
		EventDedupeTTL:       defaultEventDedupeTTL,
		RequestTimeout:       defaultRequestTimeout,
		WatchdogTimeout:      defaultWatchdogTimeout,
		PingInterval:         defaultPingInterval,
		ChunkFlushInterval:   defaultChunkFlushInterval,
		ChunkFlushMaxChars:   defaultChunkFlushMaxChars,
		StreamingEnabled:     true,
		ReconnectBaseBackoff: defaultReconnectBaseBackoff,
		ReconnectMaxBackoff:  defaultReconnectMaxBackoff,
		AlertCooldown:        defaultAlertCooldown,
		ReconnectAlertThresh: defaultReconnectAlertThresh,
		AuthFailureThresh:    defaultAuthFailureThresh,
		BacklogAlertThresh:   defaultBacklogAlertThresh,
		Routes: RouteFile{
			Default: TenantRoute{
				Enabled:   true,
				Streaming: true,
			},
			Tenants: map[string]TenantRoute{},
			Chats:   map[string]TenantRoute{},
		},
	}
}

// Validate 校验并归一化配置。
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("feishu adapter config is nil")
	}
	c.ListenAddress = strings.TrimSpace(c.ListenAddress)
	c.GatewayWSURL = strings.TrimSpace(c.GatewayWSURL)
	c.GatewayWSOrigin = strings.TrimSpace(c.GatewayWSOrigin)
	c.GatewayTokenFile = strings.TrimSpace(c.GatewayTokenFile)
	c.FeishuBaseURL = strings.TrimSpace(c.FeishuBaseURL)
	c.SigningSecret = strings.TrimSpace(c.SigningSecret)
	c.DedupeStoreFile = strings.TrimSpace(c.DedupeStoreFile)
	c.AlertWebhookURL = strings.TrimSpace(c.AlertWebhookURL)

	if c.ListenAddress == "" {
		c.ListenAddress = defaultListenAddress
	}
	if c.GatewayWSURL == "" {
		c.GatewayWSURL = defaultGatewayWSURL
	}
	if c.GatewayWSOrigin == "" {
		c.GatewayWSOrigin = defaultGatewayWSOrigin
	}
	if c.FeishuBaseURL == "" {
		c.FeishuBaseURL = defaultFeishuBaseURL
	}
	if c.SignatureMaxSkew <= 0 {
		c.SignatureMaxSkew = defaultSignatureMaxSkew
	}
	if c.EventDedupeTTL <= 0 {
		c.EventDedupeTTL = defaultEventDedupeTTL
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = defaultRequestTimeout
	}
	if c.WatchdogTimeout <= 0 {
		c.WatchdogTimeout = defaultWatchdogTimeout
	}
	if c.PingInterval <= 0 {
		c.PingInterval = defaultPingInterval
	}
	if c.ChunkFlushInterval <= 0 {
		c.ChunkFlushInterval = defaultChunkFlushInterval
	}
	if c.ChunkFlushMaxChars <= 0 {
		c.ChunkFlushMaxChars = defaultChunkFlushMaxChars
	}
	if c.ReconnectBaseBackoff <= 0 {
		c.ReconnectBaseBackoff = defaultReconnectBaseBackoff
	}
	if c.ReconnectMaxBackoff <= 0 {
		c.ReconnectMaxBackoff = defaultReconnectMaxBackoff
	}
	if c.ReconnectMaxBackoff < c.ReconnectBaseBackoff {
		return fmt.Errorf("invalid reconnect backoff: max(%s) < base(%s)", c.ReconnectMaxBackoff, c.ReconnectBaseBackoff)
	}
	if c.AlertCooldown <= 0 {
		c.AlertCooldown = defaultAlertCooldown
	}
	if c.ReconnectAlertThresh <= 0 {
		c.ReconnectAlertThresh = defaultReconnectAlertThresh
	}
	if c.AuthFailureThresh <= 0 {
		c.AuthFailureThresh = defaultAuthFailureThresh
	}
	if c.BacklogAlertThresh <= 0 {
		c.BacklogAlertThresh = defaultBacklogAlertThresh
	}
	if c.Routes.Tenants == nil {
		c.Routes.Tenants = map[string]TenantRoute{}
	}
	if c.Routes.Chats == nil {
		c.Routes.Chats = map[string]TenantRoute{}
	}
	return nil
}

// LoadRoutesFromFile 从 json 文件加载路由。
func LoadRoutesFromFile(path string) (RouteFile, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return RouteFile{}, nil
	}
	raw, err := os.ReadFile(trimmed)
	if err != nil {
		return RouteFile{}, fmt.Errorf("read route file: %w", err)
	}
	var routes RouteFile
	if err := json.Unmarshal(raw, &routes); err != nil {
		return RouteFile{}, fmt.Errorf("decode route file: %w", err)
	}
	if routes.Tenants == nil {
		routes.Tenants = map[string]TenantRoute{}
	}
	if routes.Chats == nil {
		routes.Chats = map[string]TenantRoute{}
	}
	return routes, nil
}
