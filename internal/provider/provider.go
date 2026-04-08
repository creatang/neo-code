package provider

import (
	"context"

	"neo-code/internal/provider/types"
)

// Provider 定义模型对话能力，通过 channel 推送流式事件给上层消费。
type Provider interface {
	Chat(ctx context.Context, req types.ChatRequest, events chan<- types.StreamEvent) error
}
