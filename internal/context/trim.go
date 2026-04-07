package context

import (
	"neo-code/internal/context/internalcompact"
	providertypes "neo-code/internal/provider/types"
)

const maxRetainedMessageSpans = 10

// trimMessages 按消息分段裁剪上下文，并始终保护最近一条明确用户指令所在尾部。
func trimMessages(messages []providertypes.Message) []providertypes.Message {
	spans := internalcompact.BuildMessageSpans(messages)
	if len(spans) <= maxRetainedMessageSpans {
		return append([]providertypes.Message(nil), messages...)
	}

	start := spans[len(spans)-maxRetainedMessageSpans].Start
	if protectedStart, ok := internalcompact.ProtectedTailStart(spans); ok && protectedStart < start {
		start = protectedStart
	}
	return append([]providertypes.Message(nil), messages[start:]...)
}
