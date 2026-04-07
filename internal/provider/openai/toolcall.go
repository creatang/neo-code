package openai

import (
	"context"
	"strings"

	providertypes "neo-code/internal/provider/types"
)

// mergeToolCallDelta 将单个 tool call delta 累积到 toolCalls map 中。
// 首次发现带名称的 delta 时发送 tool_call_start 事件；
// 每次收到 arguments 增量时发送 tool_call_delta 事件。
func mergeToolCallDelta(ctx context.Context, events chan<- providertypes.StreamEvent, toolCalls map[int]*providertypes.ToolCall, delta toolCallDelta) error {
	call, exists := toolCalls[delta.Index]
	if !exists {
		call = &providertypes.ToolCall{}
		toolCalls[delta.Index] = call
	}

	hadName := strings.TrimSpace(call.Name) != ""

	if id := strings.TrimSpace(delta.ID); id != "" {
		call.ID = id
	}
	if name := strings.TrimSpace(delta.Function.Name); name != "" {
		call.Name = name
	}

	if !hadName && strings.TrimSpace(call.Name) != "" {
		if err := emitToolCallStart(ctx, events, delta.Index, call.ID, call.Name); err != nil {
			return err
		}
	}

	// 发送参数增量事件（同一 chunk 可能同时携带 name 和 arguments）
	if args := delta.Function.Arguments; args != "" {
		call.Arguments += args
		if err := emitToolCallDelta(ctx, events, delta.Index, call.ID, args); err != nil {
			return err
		}
	}
	return nil
}
