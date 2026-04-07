package utils

import (
	"testing"

	tuistate "neo-code/internal/tui/state"
)

func TestPickerLabelFromMode(t *testing.T) {
	if got := PickerLabelFromMode(tuistate.PickerProvider); got != "provider" {
		t.Fatalf("expected provider label, got %q", got)
	}
	if got := PickerLabelFromMode(tuistate.PickerModel); got != "model" {
		t.Fatalf("expected model label, got %q", got)
	}
	if got := PickerLabelFromMode(tuistate.PickerFile); got != "file" {
		t.Fatalf("expected file label, got %q", got)
	}
	if got := PickerLabelFromMode(tuistate.PickerMode(99)); got != "none" {
		t.Fatalf("expected default picker label none, got %q", got)
	}
}

func TestRequestedWorkdirForRun(t *testing.T) {
	if got := RequestedWorkdirForRun("", "/repo"); got != "/repo" {
		t.Fatalf("expected current workdir when active session is blank, got %q", got)
	}
	if got := RequestedWorkdirForRun("session-1", "/repo"); got != "" {
		t.Fatalf("expected empty requested workdir when active session exists, got %q", got)
	}
}

func TestIsBusy(t *testing.T) {
	if IsBusy(false, false) {
		t.Fatalf("expected idle state")
	}
	if !IsBusy(true, false) || !IsBusy(false, true) || !IsBusy(true, true) {
		t.Fatalf("expected busy state when any operation is running")
	}
}

func TestFocusLabelFromPanel(t *testing.T) {
	const (
		sessions   = "Sessions"
		transcript = "Transcript"
		activity   = "Activity"
		composer   = "Composer"
	)

	if got := FocusLabelFromPanel(tuistate.PanelSessions, sessions, transcript, activity, composer); got != sessions {
		t.Fatalf("expected sessions label, got %q", got)
	}
	if got := FocusLabelFromPanel(tuistate.PanelTranscript, sessions, transcript, activity, composer); got != transcript {
		t.Fatalf("expected transcript label, got %q", got)
	}
	if got := FocusLabelFromPanel(tuistate.PanelActivity, sessions, transcript, activity, composer); got != activity {
		t.Fatalf("expected activity label, got %q", got)
	}
	if got := FocusLabelFromPanel(tuistate.PanelInput, sessions, transcript, activity, composer); got != composer {
		t.Fatalf("expected composer label, got %q", got)
	}
}

func TestTrimHelpers(t *testing.T) {
	if got := TrimRunes("abcdef", 3); got != "abcdef" {
		t.Fatalf("expected original text when limit < 4, got %q", got)
	}
	if got := TrimRunes("abcdef", 5); got != "ab..." {
		t.Fatalf("expected rune-safe truncation, got %q", got)
	}

	if got := TrimMiddle("abcdef", 6); got != "abcdef" {
		t.Fatalf("expected no trim when limit < 7, got %q", got)
	}
	if got := TrimMiddle("abcdefghij", 7); got != "ab...ij" {
		t.Fatalf("expected middle trim output, got %q", got)
	}
}

func TestFallbackAndClamp(t *testing.T) {
	if got := Fallback("value", "fallback"); got != "value" {
		t.Fatalf("expected value when non-empty, got %q", got)
	}
	if got := Fallback("   ", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback for blank value, got %q", got)
	}

	if got := Clamp(-1, 0, 10); got != 0 {
		t.Fatalf("expected clamp to min, got %d", got)
	}
	if got := Clamp(11, 0, 10); got != 10 {
		t.Fatalf("expected clamp to max, got %d", got)
	}
	if got := Clamp(5, 0, 10); got != 5 {
		t.Fatalf("expected in-range value unchanged, got %d", got)
	}
}
