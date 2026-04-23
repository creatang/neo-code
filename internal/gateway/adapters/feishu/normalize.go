package feishu

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// BuildSessionID 生成稳定的 session_id = hash(app_id:chat_id:thread_id)。
func BuildSessionID(appID, chatID, threadID string) string {
	normalizedThread := strings.TrimSpace(threadID)
	if normalizedThread == "" {
		normalizedThread = strings.TrimSpace(chatID)
	}
	raw := strings.TrimSpace(appID) + ":" + strings.TrimSpace(chatID) + ":" + normalizedThread
	sum := sha256.Sum256([]byte(raw))
	return "fs_" + hex.EncodeToString(sum[:16])
}

func normalizeIncomingMessage(envelope IncomingEnvelope, now time.Time) (NormalizedMessage, error) {
	msg := NormalizedMessage{
		EventID:      strings.TrimSpace(envelope.Header.EventID),
		EventType:    strings.TrimSpace(envelope.Header.EventType),
		AppID:        strings.TrimSpace(envelope.Header.AppID),
		TenantKey:    strings.TrimSpace(envelope.Header.TenantKey),
		ChatID:       strings.TrimSpace(envelope.Event.ChatID),
		ThreadID:     strings.TrimSpace(envelope.Event.ThreadID),
		MessageID:    strings.TrimSpace(envelope.Event.Message.MessageID),
		MessageType:  strings.TrimSpace(envelope.Event.Message.MessageType),
		SenderUserID: strings.TrimSpace(envelope.Event.Sender.SenderID.UserID),
		ReceivedAt:   now.UTC(),
	}
	if msg.ChatID == "" {
		msg.ChatID = strings.TrimSpace(envelope.Event.Message.ChatID)
	}
	if msg.ThreadID == "" {
		msg.ThreadID = strings.TrimSpace(envelope.Event.Message.ThreadID)
	}
	if msg.EventID == "" {
		return NormalizedMessage{}, fmt.Errorf("missing event_id")
	}
	if msg.AppID == "" {
		return NormalizedMessage{}, fmt.Errorf("missing app_id")
	}
	if msg.ChatID == "" {
		return NormalizedMessage{}, fmt.Errorf("missing chat_id")
	}
	if msg.MessageID == "" {
		msg.MessageID = msg.EventID
	}
	msg.Text = decodeTextContent(envelope.Event.Message.Content)
	if strings.TrimSpace(msg.Text) == "" {
		return NormalizedMessage{}, fmt.Errorf("unsupported or empty message content")
	}
	return msg, nil
}

func decodeTextContent(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}

	var contentObj struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(trimmed), &contentObj); err == nil && strings.TrimSpace(contentObj.Text) != "" {
		return strings.TrimSpace(contentObj.Text)
	}
	return strings.TrimSpace(content)
}
