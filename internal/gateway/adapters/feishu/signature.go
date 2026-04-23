package feishu

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SignatureVerifier 校验飞书 webhook 签名。
type SignatureVerifier struct {
	Secret  string
	MaxSkew time.Duration
	Now     func() time.Time
}

// Verify 校验签名与请求时间窗口。
func (v SignatureVerifier) Verify(timestamp, nonce, signature string, body []byte) error {
	secret := strings.TrimSpace(v.Secret)
	if secret == "" {
		return nil
	}
	ts := strings.TrimSpace(timestamp)
	if ts == "" {
		return errors.New("missing x-lark-request-timestamp")
	}
	nonce = strings.TrimSpace(nonce)
	if nonce == "" {
		return errors.New("missing x-lark-request-nonce")
	}
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return errors.New("missing x-lark-signature")
	}
	if err := validateTimestampSkew(ts, v.MaxSkew, v.Now); err != nil {
		return err
	}
	if !matchSignature(secret, ts, nonce, body, signature) {
		return errors.New("invalid signature")
	}
	return nil
}

func validateTimestampSkew(timestamp string, maxSkew time.Duration, nowFn func() time.Time) error {
	if maxSkew <= 0 {
		return nil
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	now := time.Now().UTC()
	if nowFn != nil {
		now = nowFn().UTC()
	}
	delta := now.Sub(time.Unix(seconds, 0).UTC())
	if delta < 0 {
		delta = -delta
	}
	if delta > maxSkew {
		return fmt.Errorf("timestamp skew exceeded: %s > %s", delta, maxSkew)
	}
	return nil
}

// matchSignature 兼容 hex/base64 两种常见编码。
func matchSignature(secret, timestamp, nonce string, body []byte, got string) bool {
	payload := timestamp + nonce + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	digest := mac.Sum(nil)
	hexSig := hex.EncodeToString(digest)
	b64Sig := base64.StdEncoding.EncodeToString(digest)
	gotLower := strings.ToLower(strings.TrimSpace(got))
	return hmac.Equal([]byte(gotLower), []byte(strings.ToLower(hexSig))) || hmac.Equal([]byte(got), []byte(b64Sig))
}
