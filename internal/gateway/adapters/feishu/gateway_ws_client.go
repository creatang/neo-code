package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	gatewayauth "neo-code/internal/gateway/auth"
	"neo-code/internal/gateway/protocol"

	"golang.org/x/net/websocket"
)

type wsRPCResponse struct {
	Result       json.RawMessage
	RPCError     *protocol.JSONRPCError
	TransportErr error
}

// GatewayRPCError 表示网关返回的结构化错误。
type GatewayRPCError struct {
	Method      string
	Code        int
	GatewayCode string
	Message     string
}

func (e *GatewayRPCError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.GatewayCode) != "" {
		return fmt.Sprintf("gateway rpc %s failed (%s): %s", e.Method, e.GatewayCode, e.Message)
	}
	return fmt.Sprintf("gateway rpc %s failed: %s", e.Method, e.Message)
}

type gatewayWSBindingKey struct {
	SessionID string
	RunID     string
}

// GatewayWSClientOptions 描述网关 WS 客户端配置。
type GatewayWSClientOptions struct {
	URL       string
	Origin    string
	TokenFile string
	Logger    *log.Logger
	LoadToken func(path string) (string, error)
	Dial      func(url_, protocol, origin string) (*websocket.Conn, error)
}

// GatewayWSClient 提供网关 WS RPC 调用与事件接收。
type GatewayWSClient struct {
	url       string
	origin    string
	tokenFile string
	logger    *log.Logger
	loadToken func(path string) (string, error)
	dial      func(url_, protocol, origin string) (*websocket.Conn, error)

	writeMu  sync.Mutex
	mu       sync.Mutex
	conn     *websocket.Conn
	encoder  *json.Encoder
	pending  map[string]chan wsRPCResponse
	bindings map[gatewayWSBindingKey]protocol.BindStreamParams

	closeOnce     sync.Once
	closed        chan struct{}
	notifications chan protocol.JSONRPCNotification
	sequence      atomic.Uint64
}

// NewGatewayWSClient 创建网关 WS 客户端。
func NewGatewayWSClient(options GatewayWSClientOptions) (*GatewayWSClient, error) {
	url := strings.TrimSpace(options.URL)
	origin := strings.TrimSpace(options.Origin)
	if url == "" {
		url = defaultGatewayWSURL
	}
	if origin == "" {
		origin = defaultGatewayWSOrigin
	}
	loadToken := options.LoadToken
	if loadToken == nil {
		loadToken = gatewayauth.LoadTokenFromFile
	}
	dial := options.Dial
	if dial == nil {
		dial = websocket.Dial
	}
	logger := options.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "feishu-gateway-ws: ", log.LstdFlags)
	}

	return &GatewayWSClient{
		url:           url,
		origin:        origin,
		tokenFile:     strings.TrimSpace(options.TokenFile),
		logger:        logger,
		loadToken:     loadToken,
		dial:          dial,
		pending:       map[string]chan wsRPCResponse{},
		bindings:      map[gatewayWSBindingKey]protocol.BindStreamParams{},
		closed:        make(chan struct{}),
		notifications: make(chan protocol.JSONRPCNotification, 256),
	}, nil
}

// Notifications 返回 gateway.event 通知流。
func (c *GatewayWSClient) Notifications() <-chan protocol.JSONRPCNotification {
	if c == nil {
		return nil
	}
	return c.notifications
}

// Close 关闭客户端。
func (c *GatewayWSClient) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		close(c.closed)
		c.resetConnection(errors.New("gateway ws client closed"))
		close(c.notifications)
	})
	return nil
}

// Authenticate 显式认证连接。
func (c *GatewayWSClient) Authenticate(ctx context.Context) error {
	token, err := c.loadGatewayToken()
	if err != nil {
		return err
	}
	var frame map[string]any
	return c.Call(ctx, protocol.MethodGatewayAuthenticate, protocol.AuthenticateParams{
		Token: token,
	}, &frame)
}

// Ping 发送心跳。
func (c *GatewayWSClient) Ping(ctx context.Context) error {
	var frame map[string]any
	return c.Call(ctx, protocol.MethodGatewayPing, struct{}{}, &frame)
}

// BindStream 执行绑定并记录恢复点。
func (c *GatewayWSClient) BindStream(ctx context.Context, sessionID, runID string) error {
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if sessionID == "" {
		return errors.New("bind stream session_id is empty")
	}

	params := protocol.BindStreamParams{
		SessionID: sessionID,
		RunID:     runID,
		Channel:   "all",
	}
	var frame map[string]any
	if err := c.Call(ctx, protocol.MethodGatewayBindStream, params, &frame); err != nil {
		return err
	}

	c.mu.Lock()
	c.bindings[gatewayWSBindingKey{SessionID: sessionID, RunID: runID}] = params
	c.mu.Unlock()
	return nil
}

// Call 发起一次 RPC 调用。
func (c *GatewayWSClient) Call(ctx context.Context, method string, params any, result any) error {
	if c == nil {
		return errors.New("gateway ws client is nil")
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return errors.New("gateway rpc method is empty")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := c.ensureConnected(ctx); err != nil {
		return err
	}

	requestID := c.nextRequestID()
	responseCh := make(chan wsRPCResponse, 1)

	c.mu.Lock()
	c.pending[requestID] = responseCh
	conn := c.conn
	encoder := c.encoder
	c.mu.Unlock()

	if conn == nil || encoder == nil {
		c.removePending(requestID)
		return errors.New("gateway ws connection is unavailable")
	}

	idJSON, _ := json.Marshal(requestID)
	request := protocol.JSONRPCRequest{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      json.RawMessage(idJSON),
		Method:  method,
	}
	if params != nil {
		paramsRaw, err := json.Marshal(params)
		if err != nil {
			c.removePending(requestID)
			return fmt.Errorf("encode params: %w", err)
		}
		request.Params = json.RawMessage(paramsRaw)
	}

	c.writeMu.Lock()
	writeErr := encoder.Encode(request)
	c.writeMu.Unlock()
	if writeErr != nil {
		c.removePending(requestID)
		c.resetConnection(fmt.Errorf("gateway ws write failed: %w", writeErr))
		return fmt.Errorf("gateway ws write failed: %w", writeErr)
	}

	select {
	case <-ctx.Done():
		c.removePending(requestID)
		return ctx.Err()
	case response := <-responseCh:
		if response.TransportErr != nil {
			return response.TransportErr
		}
		if response.RPCError != nil {
			return &GatewayRPCError{
				Method:      method,
				Code:        response.RPCError.Code,
				GatewayCode: protocol.GatewayCodeFromJSONRPCError(response.RPCError),
				Message:     strings.TrimSpace(response.RPCError.Message),
			}
		}
		if result != nil && len(response.Result) > 0 {
			if err := json.Unmarshal(response.Result, result); err != nil {
				return fmt.Errorf("decode gateway result: %w", err)
			}
		}
		return nil
	}
}

func (c *GatewayWSClient) ensureConnected(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.mu.Lock()
	if c.conn != nil && c.encoder != nil {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	conn, err := c.dial(c.url, "", c.origin)
	if err != nil {
		return fmt.Errorf("dial gateway ws: %w", err)
	}

	c.mu.Lock()
	if c.conn != nil {
		c.mu.Unlock()
		_ = conn.Close()
		return nil
	}
	c.conn = conn
	c.encoder = json.NewEncoder(conn)
	c.mu.Unlock()

	go c.readLoop(conn)

	if err := c.Authenticate(ctx); err != nil {
		c.resetConnection(err)
		return err
	}
	if err := c.replayBindings(ctx); err != nil {
		c.resetConnection(err)
		return err
	}
	return nil
}

func (c *GatewayWSClient) replayBindings(ctx context.Context) error {
	c.mu.Lock()
	bindings := make([]protocol.BindStreamParams, 0, len(c.bindings))
	for _, item := range c.bindings {
		bindings = append(bindings, item)
	}
	c.mu.Unlock()

	for _, binding := range bindings {
		var frame map[string]any
		if err := c.Call(ctx, protocol.MethodGatewayBindStream, binding, &frame); err != nil {
			return err
		}
	}
	return nil
}

func (c *GatewayWSClient) readLoop(conn *websocket.Conn) {
	for {
		select {
		case <-c.closed:
			return
		default:
		}

		var raw string
		if err := websocket.Message.Receive(conn, &raw); err != nil {
			c.resetConnection(fmt.Errorf("gateway ws read failed: %w", err))
			return
		}

		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, `"type":"heartbeat"`) {
			continue
		}

		var envelope struct {
			ID     json.RawMessage        `json:"id"`
			Result json.RawMessage        `json:"result"`
			Error  *protocol.JSONRPCError `json:"error"`
			Method string                 `json:"method"`
			Params json.RawMessage        `json:"params"`
		}
		if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
			c.logger.Printf("decode gateway ws payload failed: %v", err)
			continue
		}

		if strings.TrimSpace(envelope.Method) != "" {
			notification := protocol.JSONRPCNotification{
				JSONRPC: protocol.JSONRPCVersion,
				Method:  strings.TrimSpace(envelope.Method),
				Params:  envelope.Params,
			}
			select {
			case <-c.closed:
				return
			case c.notifications <- notification:
			default:
				c.logger.Printf("gateway notification queue is full, dropping %s", notification.Method)
			}
			continue
		}

		responseID, err := decodeResponseID(envelope.ID)
		if err != nil || responseID == "" {
			continue
		}
		c.mu.Lock()
		ch, exists := c.pending[responseID]
		if exists {
			delete(c.pending, responseID)
		}
		c.mu.Unlock()
		if !exists {
			continue
		}
		ch <- wsRPCResponse{
			Result:   envelope.Result,
			RPCError: envelope.Error,
		}
	}
}

func (c *GatewayWSClient) removePending(requestID string) {
	c.mu.Lock()
	delete(c.pending, requestID)
	c.mu.Unlock()
}

func (c *GatewayWSClient) resetConnection(reason error) {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.encoder = nil
	pending := c.pending
	c.pending = map[string]chan wsRPCResponse{}
	c.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	for _, ch := range pending {
		ch <- wsRPCResponse{TransportErr: reason}
	}
}

func (c *GatewayWSClient) nextRequestID() string {
	next := c.sequence.Add(1)
	return "feishu-" + strconv.FormatUint(next, 10)
}

func (c *GatewayWSClient) loadGatewayToken() (string, error) {
	if c.loadToken == nil {
		return "", errors.New("gateway token loader is nil")
	}
	token, err := c.loadToken(c.tokenFile)
	if err != nil {
		return "", fmt.Errorf("load gateway token: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("gateway token is empty")
	}
	return token, nil
}

func decodeResponseID(raw json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "", errors.New("empty id")
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString), nil
	}
	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		return strings.TrimSpace(asNumber.String()), nil
	}
	return "", errors.New("unsupported id type")
}
