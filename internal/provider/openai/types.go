package openai

// 以下类型定义了 OpenAI Chat Completions API 的请求和响应结构体，
// 仅在 openai 子包内部使用，不对外暴露。

// chatCompletionRequest 表示 /chat/completions 端点的请求体。
type chatCompletionRequest struct {
	Model      string                 `json:"model"`
	Messages   []openAIMessage        `json:"messages"`
	Tools      []openAIToolDefinition `json:"tools,omitempty"`
	ToolChoice string                 `json:"tool_choice,omitempty"`
	Stream     bool                   `json:"stream"`
}

// openAIMessage 表示 OpenAI 协议中的消息格式。
type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

// openAIToolDefinition 表示工具定义的 OpenAI 格式。
type openAIToolDefinition struct {
	Type     string                   `json:"type"`
	Function openAIFunctionDefinition `json:"function"`
}

// openAIFunctionDefinition 表示函数描述的 OpenAI 格式。
type openAIFunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// openAIToolCall 表示响应中工具调用的 OpenAI 格式。
type openAIToolCall struct {
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function openAIFunctionCall `json:"function"`
}

// openAIFunctionCall 表示函数调用参数的 OpenAI 格式。
type openAIFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// chatCompletionChunk 表示 SSE 流式响应中的单个 chunk（内部使用，非导出）。
type chatCompletionChunk struct {
	Choices []struct {
		Index        int        `json:"index"`
		Delta        chunkDelta `json:"delta"`
		FinishReason string     `json:"finish_reason"`
	} `json:"choices"`
	Usage *openAIUsage `json:"usage,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// chunkDelta 表示流式 chunk 中的增量内容。
type chunkDelta struct {
	Role      string          `json:"role,omitempty"`
	Content   string          `json:"content,omitempty"`
	ToolCalls []toolCallDelta `json:"tool_calls,omitempty"`
}

// toolCallDelta 表示流式 tool call 增量。
type toolCallDelta struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function openAIFunctionCall `json:"function"`
}

// openAIUsage 表示 token 使用统计。
type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// openAIErrorResponse 表示 API 错误响应。
type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}
