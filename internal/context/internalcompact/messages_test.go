package internalcompact

import (
	"testing"

	providertypes "neo-code/internal/provider/types"
)

func TestBuildMessageSpansPreservesToolBlocksAndProtectedTail(t *testing.T) {
	t.Parallel()

	messages := []providertypes.Message{
		{Role: providertypes.RoleUser, Content: "old"},
		{
			Role: providertypes.RoleAssistant,
			ToolCalls: []providertypes.ToolCall{
				{ID: "call-1", Name: "filesystem_read_file", Arguments: "{}"},
			},
		},
		{Role: providertypes.RoleTool, ToolCallID: "call-1", Content: "result"},
		{Role: providertypes.RoleAssistant, Content: "after tool"},
		{Role: providertypes.RoleUser, Content: "latest explicit instruction"},
		{Role: providertypes.RoleAssistant, Content: "latest answer"},
	}

	spans := BuildMessageSpans(messages)
	if len(spans) != 5 {
		t.Fatalf("expected 5 spans, got %+v", spans)
	}
	if spans[1].Start != 1 || spans[1].End != 3 || spans[1].MessageCount != 2 {
		t.Fatalf("expected assistant/tool messages to share one span, got %+v", spans[1])
	}

	protectedStart, ok := ProtectedTailStart(spans)
	if !ok {
		t.Fatalf("expected protected tail")
	}
	if protectedStart != 4 {
		t.Fatalf("expected latest explicit user instruction span protected, got %d", protectedStart)
	}
}

func TestRetainedStartForKeepRecentMessagesHonorsProtectedTail(t *testing.T) {
	t.Parallel()

	spans := []MessageSpan{
		{Start: 0, End: 1, MessageCount: 1},
		{Start: 1, End: 2, MessageCount: 1},
		{Start: 2, End: 3, MessageCount: 1, Protected: true},
		{Start: 3, End: 4, MessageCount: 1},
		{Start: 4, End: 5, MessageCount: 1},
	}

	start := RetainedStartForKeepRecentMessages(spans, 2)
	if start != 2 {
		t.Fatalf("expected protected tail start 2, got %d", start)
	}
}
