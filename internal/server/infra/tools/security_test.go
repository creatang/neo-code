package tools

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestApproveSecurityAskLogsInvalidContext(t *testing.T) {
	resetSecurityApprovals()

	var buf bytes.Buffer
	restoreLogs := captureToolLogs(&buf)
	defer restoreLogs()

	ApproveSecurityAsk("", "target")

	if !strings.Contains(buf.String(), "invalid security approval context") {
		t.Fatalf("expected invalid approval context to be logged, got %q", buf.String())
	}
}

func TestConsumeSecurityAskApprovalLogsInvalidContext(t *testing.T) {
	resetSecurityApprovals()

	var buf bytes.Buffer
	restoreLogs := captureToolLogs(&buf)
	defer restoreLogs()

	if consumeSecurityAskApproval("Bash", "") {
		t.Fatal("expected invalid approval context to be rejected")
	}
	if !strings.Contains(buf.String(), "invalid security approval context") {
		t.Fatalf("expected invalid approval context to be logged, got %q", buf.String())
	}
}

func TestSecurityApprovalRoundTrip(t *testing.T) {
	resetSecurityApprovals()

	ApproveSecurityAsk("Bash", "echo hello")

	if !consumeSecurityAskApproval("Bash", "echo hello") {
		t.Fatal("expected approval to be consumed once")
	}
	if consumeSecurityAskApproval("Bash", "echo hello") {
		t.Fatal("expected approval to be one-time")
	}
}

func resetSecurityApprovals() {
	securityAskApprovalMu.Lock()
	securityAskApprovals = map[string]int{}
	securityAskApprovalMu.Unlock()
}

func captureToolLogs(buf *bytes.Buffer) func() {
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()

	log.SetOutput(buf)
	log.SetFlags(0)
	log.SetPrefix("")

	return func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	}
}
