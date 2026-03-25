package service

import (
	"testing"

	"go-llm-demo/internal/server/domain"
)

// ---------------------------------------------------------
// 第一部分：模拟对象 (Mock)
// ---------------------------------------------------------

// mockSecurityConfigRepository 是一个“替身”对象
// 它实现了 domain.SecurityConfigRepository 接口，用来模拟从文件加载配置的过程。
// 这样我们在测试时，就不需要真的去磁盘上读那个 blacklist.yaml 了。
type mockSecurityConfigRepository struct {
	// 这里的三个字段分别代表黑、白、黄名单的内存数据
	black, white, yellow *domain.Config
}

// LoadAll 是“替身”必须实现的合同方法。
// 无论传入什么目录，它都直接返回我们在这里写死的数据。
func (m *mockSecurityConfigRepository) LoadAll(configDir string) (*domain.Config, *domain.Config, *domain.Config, error) {
	return m.black, m.white, m.yellow, nil
}

// ---------------------------------------------------------
// 第二部分：单元测试主体
// ---------------------------------------------------------

func TestSecurityService_Check(t *testing.T) {
	// 1. 准备测试用的“假规则”
	mockRepo := &mockSecurityConfigRepository{
		// 黑名单：绝对禁区。
		black: &domain.Config{
			Rules: []domain.Rule{
				// 情况 A：全能封禁。读写都不行。
				{Target: ".git/**", Read: "deny", Write: "deny"},
				// 情况 B：部分封禁。只准看，不准改（为了演示穿透）。
				// 假设我们禁止读取 .env，但“漏掉了”禁止写入。
				{Target: "**/.env", Read: "deny"},
				// 情况 C：命令封禁。
				{Command: "rm -rf **", Exec: "deny"},
			},
		},
		// 白名单：信任区域。
		white: &domain.Config{
			Rules: []domain.Rule{
				{Target: "src/**/*.go", Read: "allow"},
				{Command: "go version", Exec: "allow"},
			},
		},
		// 黄名单：确认区域。
		yellow: &domain.Config{
			Rules: []domain.Rule{
				{Target: "src/**/*.go", Write: "ask"},
				{Command: "go build **", Exec: "ask"},
			},
		},
	}

	// 2. 初始化服务，并注入替身仓储
	svc := NewSecurityService(mockRepo)
	if err := svc.Initialize(""); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}

	// 3. 定义测试剧本矩阵（表格驱动测试）
	// 我们把所有想测试的情况列在下面
	tests := []struct {
		name     string        // 测试的名字
		toolType string        // AI 试图调用的工具类型 (Read/Write/Bash/WebFetch)
		target   string        // AI 操作的目标 (文件名 或 命令字符串)
		want     domain.Action // 我们期望拦截器给出的裁定 (deny/allow/ask)
	}{
		// --- 黑名单拦截测试 ---
		{"完全拦截-禁止读取Git", "Read", ".git/config", domain.ActionDeny},
		{"完全拦截-禁止写入Git", "Write", ".git/HEAD", domain.ActionDeny},
		{"命令拦截-禁止删库", "Bash", "rm -rf /", domain.ActionDeny},

		// --- 路径规范化/绕过攻击测试 (安全核心) ---
		{"路径绕过-尝试跨出目录绕过黑名单", "Read", "src/../.git/config", domain.ActionDeny},
		{"路径绕过-冗余斜杠绕过", "Read", ".git///config", domain.ActionDeny},
		{"路径绕过-相对路径前缀", "Read", "./.git/config", domain.ActionDeny},

		// --- 规则穿透测试 ---
		{"拦截-黑名单明确禁止Read", "Read", "config/.env", domain.ActionDeny},
		{"穿透-黑名单未定义Write-落入兜底", "Write", "config/.env", domain.ActionAsk},

		// --- 白名单与黄名单匹配深度测试 ---
		{"白名单-允许阅读源码", "Read", "src/main.go", domain.ActionAllow},
		{"白名单-深层目录匹配", "Read", "src/internal/pkg/util.go", domain.ActionAllow},
		{"黄名单-修改代码需确认", "Write", "src/main.go", domain.ActionAsk},
		{"黄名单-命令匹配", "Bash", "go build main.go", domain.ActionAsk},
		{"白名单-精确命令匹配", "Bash", "go version", domain.ActionAllow},

		// --- 网络与兜底策略 ---
		{"网络-白名单域名子域", "WebFetch", "api.google.com", domain.ActionAllow},
		{"网络-不在名单内域名", "WebFetch", "hacker.com", domain.ActionAsk},
		{"兜底-完全未知工具", "SelfDestruct", "now", domain.ActionAsk},
		{"空目标字符串", "Read", "", domain.ActionAsk},
		{"大小写敏感性测试", "Read", ".GIT/config", domain.ActionAsk}, //依据实现，目前是敏感的
	}

	// 补充白名单网络规则以支持上面的测试
	mockRepo.white.Rules = append(mockRepo.white.Rules, domain.Rule{Domain: "*.google.com", Network: "allow"})

	// 4. 开始自动化表演，逐条运行剧本
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 调用 Check 方法获取实际结果
			got := svc.Check(tt.toolType, tt.target)

			// 验证结果
			if got != tt.want {
				t.Errorf("场景 [%s] 失败！\n 输入: %s(%s)\n 得到结果: %v\n 预期结果: %v",
					tt.name, tt.toolType, tt.target, got, tt.want)
			}
		})
	}
}
