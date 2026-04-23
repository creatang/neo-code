package feishu

import "strings"

const (
	retryClassNone      = "none"
	retryClassImmediate = "immediate"
	retryClassBackoff   = "backoff"
	retryClassReauth    = "reauth"
)

type gatewayCodePolicy struct {
	RetryClass string
	Retryable  bool
}

var gatewayErrorPolicies = map[string]gatewayCodePolicy{
	"invalid_frame":              {RetryClass: retryClassNone, Retryable: false},
	"invalid_action":             {RetryClass: retryClassNone, Retryable: false},
	"invalid_multimodal_payload": {RetryClass: retryClassNone, Retryable: false},
	"missing_required_field":     {RetryClass: retryClassNone, Retryable: false},
	"unsupported_action":         {RetryClass: retryClassNone, Retryable: false},
	"internal_error":             {RetryClass: retryClassBackoff, Retryable: true},
	"timeout":                    {RetryClass: retryClassBackoff, Retryable: true},
	"unauthorized":               {RetryClass: retryClassReauth, Retryable: true},
	"access_denied":              {RetryClass: retryClassNone, Retryable: false},
	"resource_not_found":         {RetryClass: retryClassNone, Retryable: false},
}

func shouldRetryGatewayCode(gatewayCode string, attempt int, maxAttempts int) bool {
	if attempt >= maxAttempts {
		return false
	}
	code := strings.TrimSpace(strings.ToLower(gatewayCode))
	if code == "" {
		return attempt < maxAttempts
	}
	policy, exists := gatewayErrorPolicies[code]
	if !exists {
		return attempt < maxAttempts
	}
	return policy.Retryable
}

func gatewayRetryClass(gatewayCode string) string {
	code := strings.TrimSpace(strings.ToLower(gatewayCode))
	if code == "" {
		return retryClassBackoff
	}
	if policy, exists := gatewayErrorPolicies[code]; exists {
		return policy.RetryClass
	}
	return retryClassBackoff
}
