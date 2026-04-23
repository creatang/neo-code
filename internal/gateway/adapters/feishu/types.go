package feishu

import "time"

// IncomingEnvelope 表示飞书 webhook 的统一入口包裹。
type IncomingEnvelope struct {
	Schema    string         `json:"schema,omitempty"`
	Type      string         `json:"type,omitempty"`
	Challenge string         `json:"challenge,omitempty"`
	Token     string         `json:"token,omitempty"`
	Header    IncomingHeader `json:"header,omitempty"`
	Event     IncomingEvent  `json:"event,omitempty"`
}

// IncomingHeader 表示飞书事件头。
type IncomingHeader struct {
	EventID    string `json:"event_id,omitempty"`
	EventType  string `json:"event_type,omitempty"`
	AppID      string `json:"app_id,omitempty"`
	TenantKey  string `json:"tenant_key,omitempty"`
	CreateTime string `json:"create_time,omitempty"`
	Token      string `json:"token,omitempty"`
}

// IncomingEvent 表示飞书消息事件的最小字段集合。
type IncomingEvent struct {
	ThreadID string          `json:"thread_id,omitempty"`
	ChatID   string          `json:"chat_id,omitempty"`
	Message  IncomingMessage `json:"message,omitempty"`
	Sender   IncomingSender  `json:"sender,omitempty"`
}

// IncomingMessage 表示飞书消息体。
type IncomingMessage struct {
	MessageID   string `json:"message_id,omitempty"`
	RootID      string `json:"root_id,omitempty"`
	ParentID    string `json:"parent_id,omitempty"`
	ThreadID    string `json:"thread_id,omitempty"`
	ChatID      string `json:"chat_id,omitempty"`
	MessageType string `json:"message_type,omitempty"`
	Content     string `json:"content,omitempty"`
}

// IncomingSender 表示飞书发送方。
type IncomingSender struct {
	SenderID IncomingSenderID `json:"sender_id,omitempty"`
}

// IncomingSenderID 表示飞书发送方 ID。
type IncomingSenderID struct {
	UserID  string `json:"user_id,omitempty"`
	OpenID  string `json:"open_id,omitempty"`
	UnionID string `json:"union_id,omitempty"`
}

// NormalizedMessage 是适配器内部使用的标准消息。
type NormalizedMessage struct {
	EventID      string
	EventType    string
	AppID        string
	TenantKey    string
	ChatID       string
	ThreadID     string
	MessageID    string
	MessageType  string
	Text         string
	SenderUserID string
	ReceivedAt   time.Time
}

// MessageTarget 表示回写目标。
type MessageTarget struct {
	TenantKey string
	ChatID    string
	ThreadID  string
	MessageID string
	UserID    string
}

// PermissionCallback 表示审批回调输入。
type PermissionCallback struct {
	RequestID string `json:"request_id"`
	Decision  string `json:"decision"`
}

// PermissionCardData describes the actionable content for a Feishu permission card.
type PermissionCardData struct {
	RequestID    string
	ToolName     string
	ActionType   string
	Operation    string
	TargetType   string
	Target       string
	Reason       string
	DecisionHint string
}

// ActionCallbackPayload is a normalized subset of Feishu action callback payload.
type ActionCallbackPayload struct {
	OpenMessageID string         `json:"open_message_id,omitempty"`
	OpenChatID    string         `json:"open_chat_id,omitempty"`
	UserID        string         `json:"user_id,omitempty"`
	Action        ActionCallback `json:"action,omitempty"`
	Event         ActionEvent    `json:"event,omitempty"`
}

// ActionEvent wraps card callback payloads nested under event.*.
type ActionEvent struct {
	Action ActionCallback `json:"action,omitempty"`
}

// ActionCallback captures action values submitted by card buttons.
type ActionCallback struct {
	Value map[string]any `json:"value,omitempty"`
}

// CancelCallback 表示取消回调输入。
type CancelCallback struct {
	RunID string `json:"run_id"`
}

// RunRecord 保存 run 运行时关联信息。
type RunRecord struct {
	RunID       string
	SessionID   string
	RequestID   string
	EventID     string
	TenantKey   string
	Target      MessageTarget
	AcceptedAt  time.Time
	LastEventAt time.Time
	Streaming   bool
	Completed   bool
	LastError   string
}
