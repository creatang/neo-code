package status

import (
	"strings"
	"testing"

	tuistate "neo-code/internal/tui/state"
)

func TestBuildFromUIState(t *testing.T) {
	state := tuistate.UIState{
		ActiveSessionID:    "session-1",
		ActiveSessionTitle: "My Session",
		ActiveRunID:        "run-1",
		IsAgentRunning:     true,
		IsCompacting:       false,
		CurrentProvider:    "openai",
		CurrentModel:       "gpt-5.4",
		CurrentWorkdir:     "/repo",
		CurrentTool:        "filesystem_read_file",
		ToolStates: []tuistate.ToolState{
			{ToolCallID: "call-1"},
			{ToolCallID: "call-2"},
		},
		TokenUsage: tuistate.TokenUsageState{
			RunTotalTokens:     12,
			SessionTotalTokens: 34,
		},
		ExecutionError: "boom",
	}

	snapshot := BuildFromUIState(state, 7, "transcript", "provider")
	if snapshot.ActiveSessionID != "session-1" || snapshot.ActiveRunID != "run-1" {
		t.Fatalf("unexpected snapshot identifiers: %+v", snapshot)
	}
	if snapshot.ToolStateCount != 2 || snapshot.RunTotalTokens != 12 || snapshot.SessionTotalTokens != 34 {
		t.Fatalf("unexpected snapshot counters: %+v", snapshot)
	}
	if snapshot.FocusLabel != "transcript" || snapshot.PickerLabel != "provider" || snapshot.MessageCount != 7 {
		t.Fatalf("unexpected snapshot labels: %+v", snapshot)
	}
}

func TestFormat(t *testing.T) {
	formatted := Format(Snapshot{
		ActiveSessionID:    "",
		ActiveSessionTitle: "",
		ActiveRunID:        " ",
		IsAgentRunning:     false,
		IsCompacting:       false,
		CurrentProvider:    "openai",
		CurrentModel:       "gpt-5.4",
		CurrentWorkdir:     "/repo",
		CurrentTool:        "",
		ToolStateCount:     1,
		RunTotalTokens:     2,
		SessionTotalTokens: 3,
		ExecutionError:     "",
		FocusLabel:         "composer",
		PickerLabel:        "",
		MessageCount:       4,
	}, "Draft Session")

	expectedParts := []string{
		"Session: Draft Session",
		"Session ID: <draft>",
		"Run ID: <none>",
		"Running: no",
		"Picker: none",
		"Current Tool: <none>",
		"Error: <none>",
	}
	for _, part := range expectedParts {
		if !strings.Contains(formatted, part) {
			t.Fatalf("expected formatted status to contain %q, got:\n%s", part, formatted)
		}
	}

	running := Format(Snapshot{
		ActiveSessionID:    "session-2",
		ActiveSessionTitle: "Named Session",
		ActiveRunID:        "run-2",
		IsCompacting:       true,
		CurrentProvider:    "openai",
		CurrentModel:       "gpt-5.4-mini",
		CurrentWorkdir:     "/repo",
		CurrentTool:        "tool-x",
		ToolStateCount:     2,
		RunTotalTokens:     10,
		SessionTotalTokens: 20,
		ExecutionError:     "failed",
		FocusLabel:         "activity",
		PickerLabel:        "model",
		MessageCount:       5,
	}, "Ignored Draft")

	if !strings.Contains(running, "Session: Named Session") || !strings.Contains(running, "Running: yes") {
		t.Fatalf("expected running status to keep explicit values, got:\n%s", running)
	}
}
