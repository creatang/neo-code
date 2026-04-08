package tools

// MicroCompactPolicy 描述工具历史结果参与 read-time micro compact 的策略。
type MicroCompactPolicy string

const (
	// MicroCompactPolicyCompact 表示工具历史结果默认参与 micro compact 清理。
	MicroCompactPolicyCompact MicroCompactPolicy = ""
	// MicroCompactPolicyPreserveHistory 表示工具历史结果应显式保留，不参与 micro compact 清理。
	MicroCompactPolicyPreserveHistory MicroCompactPolicy = "preserve_history"
)
