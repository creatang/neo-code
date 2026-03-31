# Provider 层迭代计划

> 基于主流 Agent Coding 产品（Cursor、Cline、Aider、Copilot CLI）的 Provider 层实现模式分析，结合 NeoCode 当前架构现状，制定本迭代计划。

---

## 1. 现状分析

### 当前架构

```
TUI → Runtime → Service(config.Manager + Registry) → Registry.Build → Driver(OpenAI-compatible)
                                      ↗
                                  Config.Manager (YAML + 环境变量)
```

- **统一抽象**：`Provider` 接口（`Chat` + SSE 流式事件），定义在 `internal/provider/provider.go`
- **驱动注册**：`Registry` + `DriverDefinition`，支持运行时注册新驱动（`internal/provider/registry.go`）
- **服务层**：`Service` 封装 provider 选择、模型列表、实例构建，**强依赖 `config.Manager`**（`internal/provider/service.go`）
- **配置层**：`config.Manager` 管理 YAML 配置 + 环境变量，`ProviderConfig` 支持 `driver` 字段（`internal/config/model.go`）
- **内置 provider**：4 个（openai / gemini / openll / qiniu），**全部复用 `"openai"` driver**（`internal/config/builtin_providers.go`）
- **请求/响应**：`ChatRequest` / `ChatResponse` / `StreamEvent` 统一模型（`internal/provider/types.go`）
- **错误处理**：3 个哨兵错误（`ErrProviderNotFound` / `ErrModelNotFound` / `ErrDriverNotFound`），全部为 `errors.New`
- **HTTP 客户端**：裸 `http.Client`（90s 超时），无重试、无容错（`internal/provider/openai/openai.go:48`）

### 关键事实

1. **当前只有一个 driver 实现**：所有 4 个内置 provider 的 `driver` 字段均为 `"openai"`，`Registry` 中只注册了一个 driver。
2. **用户自定义 provider 已部分可用**：`ProviderConfig` 结构体天然支持在 `config.yaml` 中添加任意 OpenAI 兼容的 provider，只要指定 `driver: openai` 并配置 `base_url`、`api_key_env`、`model(s)` 即可。`Service` 层的 `ListProviders` 和 `SelectProvider` 已经能处理这些配置。
3. **Runtime 不需要模型元数据**：当前 runtime 只是将 `Tools` 透传给 provider，由 provider（即 OpenAI 协议端）自行决定是否支持 tool_call。runtime 层**没有**根据模型能力做策略决策的逻辑。
4. **环境变量密钥是行业标准**：gh CLI、AWS CLI、OpenAI 官方 CLI 均使用环境变量，系统钥匙串需要平台特定代码（macOS Keychain、Windows DPAPI、Linux Secret Service），复杂度高，CLI 场景暂不需要。

### 与主流实现的差距

| 维度 | 主流实践 | NeoCode 现状 | 差距 | 优先级 |
|------|---------|-------------|------|--------|
| 错误体系 | 分层错误码（认证/限流/超时/参数/服务） | 3 个哨兵错误，`parseError` 返回 `errors.New` | 无法区分可重试/不可重试错误 | **P0** |
| 传输层 | 指数退避重试、连接池 | 裸 `http.Client`（90s 超时） | 无重试、网络抖动直接失败 | **P0** |
| 模型元数据 | 上下文窗口、能力标签、定价 | `ModelDescriptor` 仅 ID + Name | runtime 当前不需要，但为上层决策预留 | P1 |
| 模型发现 | 静态内置 + 动态 API 发现 + 本地缓存 | 静态硬编码字符串（`builtin_providers.go`） | 服务端新增模型需升级客户端 | P1 |
| Provider 扩展 | 用户自定义 provider + 配置 | **已支持**：`config.yaml` 中添加 `ProviderConfig` 即可 | 缺少文档，用户体验待改善 | P2 |
| 密钥安全 | 系统钥匙串 / 加密存储 | 环境变量 | CLI 行业标准，暂不需升级 | — |
| 健康检查 | 连接探针、状态展示 | 无 | 缺少 provider 可达性检测 | P2 |

---

## 2. 迭代计划

### Phase 1：基础治理（优先级 P0）

> 目标：让现有架构具备生产级可用性，不改变对外接口。

#### 1.1 错误码标准化

**问题**：所有错误都是 `errors.New`，无法区分认证失败、限流、超时等，无法支持智能重试。

**实际约束**：不同 provider 的错误格式差异极大。OpenAI 返回 `{"error": {"message": "...", "type": "...", "code": "..."}}`，但其他兼容 OpenAI 协议的 provider（如 Gemini 转接、自托管模型）可能返回完全不同的格式。试图完整映射所有错误码会导致 `parseError` 过度复杂且需持续维护。

**方案**（务实版）：

- 定义 `ProviderError` 结构体，核心只区分 **可重试/不可重试**：
  ```go
  type ProviderErrorCode int

  const (
      ErrCodeUnknown    ProviderErrorCode = iota
      ErrCodeAuthFailed                   // 401/403 — 不可重试
      ErrCodeRateLimited                  // 429 — 可重试
      ErrCodeTimeout                      // 超时 — 可重试
      ErrCodeInvalidRequest               // 400 — 不可重试
      ErrCodeServerError                  // 5xx — 可重试
      ErrCodeContextLength                // 上下文超长 — 不可重试
  )

  type ProviderError struct {
      Code       ProviderErrorCode
      HTTPStatus int
      Message    string
      Retryable  bool
  }
  ```
- OpenAI 驱动的 `parseError` 基于 HTTP 状态码 + 可选的 body `code` 字段做简单映射
- `ProviderError` 实现 `error` 接口，上层可通过类型断言判断 `Retryable`
- 不追求覆盖所有 provider 的私有错误码，只处理通用 HTTP 语义

**涉及文件**：
- `internal/provider/errors.go` — 新增 `ProviderError` 定义
- `internal/provider/openai/openai.go` — `parseError` 改造（返回 `*ProviderError`）
- `internal/provider/errors_test.go` — 新增
- `internal/provider/openai/openai_test.go` — 补充错误映射测试

#### 1.2 传输层重试

**问题**：网络抖动、偶发 5xx 直接失败，用户体验差。

**实际约束**：当前 `Provider` 的 `client` 字段是 `*http.Client`，在 `New(cfg)` 中直接构造。`DriverDefinition.Build` 签名为 `func(ctx, ResolvedProviderConfig) (Provider, error)`，Registry 调用 Builder 时没有机会注入自定义 Transport。

要支持重试中间件有两种路径：

**路径 A（推荐）**：在 driver 内部替换 `http.Client.Transport`：
```go
func New(cfg config.ResolvedProviderConfig) (*Provider, error) {
    client := &http.Client{
        Timeout:   90 * time.Second,
        Transport: transport.NewRetry(http.DefaultTransport, transport.DefaultRetryConfig()),
    }
    return &Provider{cfg: cfg, client: client}, nil
}
```
- 优点：不改变 `DriverDefinition.Build` 接口签名
- 缺点：retry 逻辑耦合在 driver 实现中

**路径 B**：修改 `DriverDefinition.Build` 签名或引入 `BuildOption`：
```go
type BuildOption func(*BuildOptions)
type BuildOptions struct { HTTPClient *http.Client }
```
- 优点：transport 可在 Registry 层统一注入
- 缺点：改动面大，影响所有 driver

**方案（路径 A）**：

- 实现 `http.RoundTripper` 中间件 `RetryTransport`
- 指数退避：仅对 `Retryable` 错误重试，最大 3 次，基础间隔 1s，抖动 ±500ms
- 可配置参数：最大重试次数、基础间隔、最大间隔
- 先在 openai driver 中启用，后续新 driver 可复用

**涉及文件**：
- `internal/provider/transport/retry.go` — 新增 `RetryTransport`
- `internal/provider/transport/retry_test.go` — 新增
- `internal/provider/openai/openai.go` — 构造 client 时注入 retry transport


---

### Phase 2：元数据与发现（优先级 P1）

> 目标：为上层决策提供模型元数据，支持动态模型列表。

#### 2.1 模型元数据系统

**问题**：`ModelDescriptor` 只有 ID + Name，上层无法做上下文窗口裁剪、能力判断等策略。

**实际约束**：当前 runtime 并不消费模型元数据（只是透传 Tools）。但为后续扩展（上下文裁剪、模型路由）预留元数据接口是合理的。

**方案**：

- 扩展 `ModelDescriptor`，增加可选字段（均为 `omitempty`，向后兼容）：
  ```go
  type ModelCapabilities struct {
      ToolCall         bool `json:"tool_call,omitempty"`
      Vision           bool `json:"vision,omitempty"`
      Streaming        bool `json:"streaming,omitempty"`
      JSONMode         bool `json:"json_mode,omitempty"`
      ExtendedThinking bool `json:"extended_thinking,omitempty"`
  }

  type ModelDescriptor struct {
      ID              string            `json:"id"`
      Name            string            `json:"name"`
      Description     string            `json:"description,omitempty"`
      ContextWindow   int               `json:"context_window,omitempty"`
      MaxOutputTokens int               `json:"max_output_tokens,omitempty"`
      Capabilities    ModelCapabilities `json:"capabilities,omitempty"`
  }
  ```
- 内置一份静态元数据映射（Go map 或嵌入的 YAML），仅覆盖高频使用的模型
- Service 层的 `modelDescriptors()` 查表填充元数据，未命中的模型保持原有行为（仅 ID + Name）
- 不引入配置文件格式的变更，元数据完全内置

**涉及文件**：
- `internal/provider/types.go` — 扩展 `ModelDescriptor` 和新增 `ModelCapabilities`
- `internal/provider/metadata.go` — 新增，静态元数据表 + 查询函数
- `internal/provider/service.go` — `modelDescriptors()` 注入元数据
- `internal/provider/metadata_test.go` — 新增
- `internal/provider/service_test.go` — 补充

#### 2.2 动态模型发现

**问题**：模型列表硬编码在 `builtin_providers.go`，服务端新增模型后需升级客户端。

**方案**：

- 新增 `Discovery` 接口：
  ```go
  type Discovery interface {
      Discover(ctx context.Context, cfg config.ResolvedProviderConfig) ([]string, error)
  }
  ```
- OpenAI 兼容 provider 通过 `GET /models` 端点拉取模型 ID 列表
- 发现结果与静态 `Models` 合并：去重，用户配置优先
- 可选本地文件缓存（`~/.neocode/cache/models/{provider}.json`），默认 TTL 24h，避免每次启动请求

**涉及文件**：
- `internal/provider/discovery/discovery.go` — 新增
- `internal/provider/discovery/discovery_test.go` — 新增
- `internal/provider/service.go` — `ListModels` 集成发现逻辑
- `internal/config/config.go` — 新增缓存相关配置（可选）

---

### Phase 3：体验增强（优先级 P2）

> 目标：改善用户侧体验，非架构性改动。

#### 3.1 用户自定义 Provider 文档与引导

**现状**：用户自定义 provider **功能上已可用**，但缺少文档和 UX 引导。

**方案**：

- 在 `docs/guides/adding-providers.md` 中补充用户配置自定义 provider 的完整示例
- TUI 的 `/provider add` 命令（如有）或 config 校验时给出友好提示
- 在 README 中说明支持的 provider 扩展方式

#### 3.2 健康检查

**方案**：通过轻量探针端点（如 `GET /models`）检测 provider 可达性，TUI 展示连接状态（绿色/红色/黄色）。

**涉及文件**：
- `internal/provider/health/health.go` — 新增

#### 3.3 成本追踪（远期可选）

**方案**：基于 `Usage` 数据 + 模型定价表，记录并展示每次会话的 token 消耗和预估费用。

---

## 3. 优先级与依赖

```
Phase 1.1 错误码  ──→ Phase 1.2 传输重试（强依赖：需 Retryable 判断）
                                            │
Phase 2.1 元数据  ──────────────────────────┤ （独立路径）
Phase 2.2 动态发现 ─────────────────────────┘ （独立路径，元数据合并不强阻塞）

Phase 3.x 体验增强 ─────────────────────────── （独立，可随时插入）
```

---

## 4. 兼容性说明

- **Phase 1 向后兼容**：`Provider` 接口不变，`ChatRequest` / `ChatResponse` 不变。`ProviderError` 通过类型断言使用，不影响现有 `error` 处理代码。
- **Phase 2 向后兼容**：`ModelDescriptor` 新增字段均为 `omitempty`，不配置元数据时行为与现有完全一致。动态发现为可选功能，不启用时走静态列表。
- **Phase 3 独立模块**：健康检查和成本追踪作为独立模块，不影响现有主链路。

---

## 5. 目录结构变更

```
internal/provider/
├── types.go              # 扩展 ModelDescriptor + 新增 ModelCapabilities
├── errors.go             # [新增] ProviderError 定义
├── metadata.go           # [新增] 静态模型元数据表
├── metadata_test.go      # [新增]
├── errors_test.go        # [新增]
├── service.go            # 注入元数据、集成动态发现
├── registry.go           # 不变
├── provider.go           # 不变
├── transport/
│   ├── retry.go          # [新增] 重试中间件
│   └── retry_test.go     # [新增]
├── discovery/
│   ├── discovery.go      # [新增] 动态模型发现
│   └── discovery_test.go # [新增]
└── health/
    └── health.go         # [新增] 健康检查（Phase 3.2）
```

---

## 6. 设计原则

1. **最小闭环**：每次改动保持主链路可用（`TUI → Runtime → Service → Registry → Driver`），不做无关重构。
2. **不提前泛化**：当前只有 1 个 driver 实现、4 个内置 provider，不为"未来可能的多 driver 场景"引入复杂抽象。
3. **务实优先**：错误码只区分可重试/不可重试，不追求完整错误码枚举；重试走 driver 内部 Transport 替换，不改 Build 接口签名。
4. **配置安全**：遵循 AGENTS.md 规则，环境变量名入配置，明文 API Key 不入库、不提交。
5. **测试同步**：修改 `provider`、`tools`、`runtime` 时同步补充测试，重点覆盖错误映射、重试退避、元数据查询边界。
