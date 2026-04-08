package tools

import (
	"context"
	"errors"
	"sort"
	"strings"

	providertypes "neo-code/internal/provider/types"
	"neo-code/internal/security"
)

type Registry struct {
	tools                map[string]Tool
	microCompactPolicies map[string]MicroCompactPolicy
}

func NewRegistry() *Registry {
	return &Registry{
		tools:                map[string]Tool{},
		microCompactPolicies: map[string]MicroCompactPolicy{},
	}
}

func (r *Registry) Register(tool Tool) {
	if tool == nil {
		return
	}
	name := strings.ToLower(tool.Name())
	r.tools[name] = tool
	switch tool.MicroCompactPolicy() {
	case MicroCompactPolicyPreserveHistory:
		r.microCompactPolicies[name] = MicroCompactPolicyPreserveHistory
	default:
		r.microCompactPolicies[name] = MicroCompactPolicyCompact
	}
}

func (r *Registry) Get(name string) (Tool, error) {
	tool, ok := r.tools[strings.ToLower(name)]
	if !ok {
		return nil, errors.New("tool: not found")
	}
	return tool, nil
}

// Supports reports whether a tool is registered.
func (r *Registry) Supports(name string) bool {
	_, err := r.Get(name)
	return err == nil
}

// MicroCompactPolicy 返回指定工具名的 micro compact 策略；未知工具按默认可压缩处理。
func (r *Registry) MicroCompactPolicy(name string) MicroCompactPolicy {
	if r == nil {
		return MicroCompactPolicyCompact
	}
	policy, ok := r.microCompactPolicies[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return MicroCompactPolicyCompact
	}
	if policy == MicroCompactPolicyPreserveHistory {
		return MicroCompactPolicyPreserveHistory
	}
	return MicroCompactPolicyCompact
}

func (r *Registry) GetSpecs() []providertypes.ToolSpec {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	specs := make([]providertypes.ToolSpec, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		specs = append(specs, providertypes.ToolSpec{
			Name:        tool.Name(),
			Description: tool.Description(),
			Schema:      tool.Schema(),
		})
	}
	return specs
}

func (r *Registry) ListSchemas() []providertypes.ToolSpec {
	return r.GetSpecs()
}

// ListAvailableSpecs returns all registered tool specs.
func (r *Registry) ListAvailableSpecs(ctx context.Context, input SpecListInput) ([]providertypes.ToolSpec, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.GetSpecs(), nil
}

func (r *Registry) Execute(ctx context.Context, input ToolCallInput) (ToolResult, error) {
	tool, err := r.Get(input.Name)
	if err != nil {
		content := FormatError(input.Name, NormalizeErrorReason(input.Name, err), "")
		return ToolResult{
			ToolCallID: input.ID,
			Name:       input.Name,
			Content:    content,
			IsError:    true,
		}, err
	}

	result, execErr := tool.Execute(ctx, input)
	result.ToolCallID = input.ID
	if execErr != nil {
		result.IsError = true
		if strings.TrimSpace(result.Content) == "" {
			result.Content = FormatError(result.Name, NormalizeErrorReason(result.Name, execErr), "")
		}
		return result, execErr
	}
	return result, nil
}

// RememberSessionDecision 对纯 Registry 管理器不生效，保留接口以满足 runtime 依赖。
func (r *Registry) RememberSessionDecision(sessionID string, action security.Action, scope SessionPermissionScope) error {
	return errors.New("tools: session permission memory is unsupported by registry manager")
}
