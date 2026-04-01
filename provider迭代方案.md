# Provider 层迭代计划

> 基于主流 Agent Coding 产品（Cursor、Claude Code、Aider、Cline、OpenCode、Windsurf、GitHub Copilot）的 Provider 层实现模式分析，结合 NeoCode 当前架构现状，制定本迭代计划。

---

## 0. Provider 层职责边界

Provider 层向上层 Agent 逻辑屏蔽 LLM 厂商差异，核心职责：

1. **协议适配**：通过 `Provider` 接口（`Chat` + `chan<- StreamEvent`）将不同厂商 API 归一化。
2. **认证封装**：统一注入 API Key 等认证信息，凭据不向上泄漏。
3. **传输容错**：处理网络超时、连接中断等传输层错误，实现幂等重试。
4. **模型元数据**：向上暴露 `context_window`、能力标签等模型原生参数，供编排层做策略决策。
5. **能力降级**：对上游请求的模型不支持的特性，返回明确错误或兼容降级。

> **边界红线**：Provider 层仅处理 LLM API 原生的错误和元数据，不涉及 Agent 任务级降级、fallback 模型调度（属于 Runtime 层职责）。

---

## 1. 现状分析

### 当前架构

```
TUI → Runtime → ProviderFactory(Build) → Registry.Build → Driver(OpenAI-compatible)
  │       │              ↑
  │       │        provider.Service (同时实现 ProviderFactory 接口)
  │       │              ↑
  │    config.Manager ───┘
  └── provider.Service (TUI /provider 命令直接使用)
                                  ↗
                              Config.Manager (YAML + 环境变量)
```

- **统一抽象**：`Provider` 接口（`Chat` 方法，通过 `chan<- StreamEvent` 传出流式事件），定义在 `internal/provider/provider.go`
- **驱动注册**：`Registry` + `DriverDefinition`，支持运行时注册新驱动（`internal/provider/registry.go`）。`Registry` 仅支持 `Register`，不支持 `Unregister`（当前阶段无需动态清理）。
- **服务层**：`Service` 封装 provider 选择、模型列表、实例构建，**强依赖 `config.Manager`**（`internal/provider/service.go`）。同时实现 `ProviderFactory` 接口，作为 Runtime 层构建 provider 实例的桥梁。
- **工厂接口**：`ProviderFactory`（定义在 `internal/runtime/runtime.go`），接口仅含 `Build` 方法，Runtime 通过此接口解耦对 `Service` / `Registry` 的直接依赖。默认 fallback 为 `provider.NewRegistry()`。
- **配置层**：`config.Manager` 管理 YAML 配置 + 环境变量，`ProviderConfig` 支持 `driver` 字段（`internal/config/model.go`）
- **内置 provider**：4 个（openai / gemini / openll / qiniu），**全部复用 `"openai"` driver**（`internal/config/builtin_providers.go`）
- **请求/响应**：`ChatRequest` / `ChatResponse` / `StreamEvent` 统一模型（`internal/provider/types.go`）
- **错误处理**：3 个哨兵错误（`ErrProviderNotFound` / `ErrModelNotFound` / `ErrDriverNotFound`），全部为 `errors.New`。`parseError` 返回 `fmt.Errorf` 或 `errors.New` 包装的错误，未区分错误类型。
- **HTTP 客户端**：裸 `http.Client`（90s 超时），无重试、无容错（`internal/provider/openai/openai.go`）

### 关键事实

1. **当前只有一个 driver 实现**：所有 4 个内置 provider 的 `driver` 字段均为 `"openai"`，`Registry` 中只注册了一个 driver。
2. **用户自定义 provider 当前不可用**：`Config.Providers` 的 YAML tag 为 `"-"`（不序列化），`persistedConfig` 中没有 `Providers` 字段，用户在 `config.yaml` 中添加的自定义 provider 会在下次保存时被丢弃。Providers 完全由 `builtin.DefaultProviders()` 内置生成。
3. **Runtime 使用粗粒度消息裁剪**：runtime 当前将 `Tools` 透传给 provider，不根据模型能力做策略决策。但已有 `trimMessages` 方法（`maxContextTurns = 10` 硬编码）做按轮次裁剪，引入 `context_window` 后可演进为基于 token 的精确裁剪。
4. **API Key 完全依赖环境变量**：`ProviderConfig.APIKeyEnv` 字段存储环境变量名，运行时通过 `os.Getenv` 读取。这是 CLI 行业标准做法（gh CLI、AWS CLI），但缺少日志脱敏和配置文件权限管控。

### 主流 Agent Coding 供应商管理模式

| 模式 | 核心特征 | 代表产品 | 适用场景 |
|------|---------|---------|---------|
| **个人轻量版** | 配置驱动 + 静态注册，零依赖 | Claude Code、Aider、Kimi CLI | 个人开发者 |
| **开源工程化版** | 动态注册 + 本地持久化 + 生命周期管理 | OpenCode、Pi-Mono | 中小团队 / 二次开发 |
| **企业级生产版** | 全生命周期 + 服务治理 + 合规审计 | GitHub Copilot 企业版 | 大型组织 |

### 与主流实现的差距

| 维度 | 主流实践 | NeoCode 现状 | 差距 | 优先级 |
|------|---------|-------------|------|--------|
| 错误体系 | 分层错误码（认证/限流/超时/参数/服务） | 3 个哨兵错误，`parseError` 不区分错误类型 | 无法判断可重试性 | **P0** |
| 传输层 | 指数退避重试、连接池 | 裸 `http.Client`（90s 超时） | 无重试、网络抖动直接失败 | **P0** |
| 模型元数据 | 上下文窗口、能力标签、定价 | `ModelDescriptor` 仅 ID + Name | 为上层策略决策预留 | P1 |
| 模型发现 | 静态内置 + 动态 API 发现 + 本地缓存 | 静态硬编码（`builtin_providers.go`） | 服务端新增模型需升级客户端 | P1 |
| 供应商管理 | 内置 + 用户自定义 + 配置持久化 | **不可用**：`Providers` 不持久化，用户配置会被丢弃 | 需统一本地存储层解决 | P1 |
| API Key 安全 | 系统钥匙串 / 加密配置 + 环境变量 | 环境变量（行业标准） | 缺少日志脱敏和配置文件权限管控 | P1 |
| 健康检查 | 连接探针、状态展示 | 无 | 缺少 provider 可达性检测 | P2 |

---

## 2. 迭代计划

### Phase 1：基础治理（P0）

> 目标：让现有架构具备生产级可用性，不改变对外接口。

#### 1.1 错误码标准化

**问题**：`parseError` 返回的 `error` 未区分认证失败、限流、超时等，无法支持智能重试。

**约束**：不同 provider 的错误格式差异极大（OpenAI 兼容端点可能返回完全不同的 body 结构）。试图完整映射所有私有错误码会导致 `parseError` 过度复杂且需持续维护。

**方案**（务实版，仅区分可重试/不可重试）：

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

**方案：**

路径 A：driver 内部替换 Transport

- 实现 `http.RoundTripper` 中间件 `RetryTransport`
- 指数退避：仅对 `Retryable` 错误重试，最大 3 次，基础间隔 1s，抖动 ±500ms
- 先在 openai driver 中启用，后续新 driver 可复用

路径 B（修改 `Build` 签名注入 `BuildOption`）：

**能力边界**：`RetryTransport` 仅处理**请求级重试**（连接失败、5xx 初始响应等）。SSE 流式传输**中途断连**不在 MVP 重试范围内——`consumeStream` 仅通过 `io.EOF` 返回已接收的部分内容，保证"有部分结果总比没有好"的降级行为。

**涉及文件**：
- `internal/provider/transport/retry.go` — 新增 `RetryTransport`
- `internal/provider/transport/retry_test.go` — 新增
- `internal/provider/openai/openai.go` — 构造 client 时注入 retry transport

---

### Phase 2：元数据、发现与安全（P1）

> 目标：为上层决策提供模型元数据，支持动态模型列表，增强 API Key 安全管理。

#### 2.1 模型元数据系统

**问题**：`ModelDescriptor` 只有 ID + Name，上层无法做上下文窗口裁剪、能力判断等策略。

**演进关系**：runtime 当前用 `trimMessages`（硬编码 `maxContextTurns = 10`）做**按轮次的粗粒度裁剪**。引入 `context_window` 后可演进为**基于 token 的精确裁剪**。两种策略可并存：先按 token 预算裁剪，再用轮次上限兜底。

**方案**：

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

- 内置一份静态元数据映射（Go map），仅覆盖高频模型
- 未命中的模型保持原有行为（仅 ID + Name），向后兼容

**涉及文件**：
- `internal/provider/types.go` — 扩展 `ModelDescriptor`
- `internal/provider/metadata.go` — 新增，静态元数据表 + 查询函数
- `internal/provider/service.go` — `modelDescriptors()` 注入元数据

#### 2.2 动态模型发现

**问题**：模型列表硬编码在 `builtin_providers.go`，服务端新增模型后需升级客户端。

**方案**：

- 新增 `Discovery` 接口：OpenAI 兼容 provider 通过 `GET /models` 端点拉取模型 ID 列表
- 发现结果与静态 `Models` 合并：去重，优先级为 **用户自定义 > 动态发现 > 内置静态**
- 发现结果缓存到 SQLite `model_cache` 表（依赖 Phase 2.5），默认 TTL 24h

**涉及文件**：
- `internal/provider/discovery/discovery.go` — 新增
- `internal/provider/service.go` — `ListModels` 集成发现逻辑
- `internal/store/providers.go` — 模型缓存读写（Phase 2.5 后）

#### 2.3 API Key 安全增强

**问题**：缺少日志脱敏和配置文件权限管控。

**方案**（务实增强，不改核心架构）：

1. **日志脱敏**：API Key 仅保留前缀掩码（如 `sk-proj-****...****`）。
2. **配置文件权限管控**：
   - **当前现状**：`Loader.Save` 使用 `os.WriteFile(path, data, 0o644)`，所有用户可读，需修复。
   - **Linux/Mac**：设为 `0600`（仅当前用户可读）。
   - **Windows**：`os.Chmod` 仅影响只读属性。考虑到跨平台复杂度，Windows 暂依赖默认 NTFS 权限（通常已限制为当前用户），后续迭代再细化。
3. **API Key 持久化**（依赖 Phase 2.5）：定义 `CredentialResolver` 接口，当前默认从 `os.Getenv` 读取。后续在 SQLite 中增加 `credentials` 表，用 AES-GCM 加密存储 API Key，密钥派生自机器指纹（如用户目录路径 + 机器 ID 的 SHA-256）。用户不再需要手动配置环境变量，同时避免系统密钥环的跨平台适配问题。

**涉及文件**：
- `internal/provider/credential/credential.go` — 新增
- `internal/provider/credential/mask.go` — 新增，API Key 脱敏工具
- `internal/config/loader.go` — 配置文件写入时设置权限

---

### Phase 2.5：统一本地存储层（可考虑）

> 目标：解决数据存储碎片化问题，为用户自定义 provider、会话管理、模型缓存等提供统一的持久化基础。

**问题**：当前存储已经碎片化——配置在 YAML、会话在 JSON 文件（`~/.neocode/sessions/{id}.json`）、模型缓存计划用 JSON，每新增一种持久化需求就散落一个文件。长期来看维护和备份成本高。

**方案**：引入 SQLite 作为统一本地存储，单文件 `~/.neocode/neocode.db`。

| 对比 | 当前（散文件） | SQLite（统一） |
|------|-------------|---------------|
| 配置 | `config.yaml` | 保留（少量启动配置仍用 YAML，人类可读） |
| 用户自定义 provider | 不支持 | `providers` 表 |
| 会话历史 | `sessions/{id}.json`（N 个文件） | `sessions` + `messages` 表 |
| 模型缓存 | 计划中 `cache/models/*.json` | `model_cache` 表 |
| 原子写入 | 手动 rename | SQLite 内置事务 |
| 并发安全 | 各模块自行加锁 | SQLite WAL 模式 |

**技术选型**：使用 `modernc.org/sqlite`（纯 Go，零 CGO 依赖），不引入额外系统依赖。跨平台兼容（Windows / macOS / Linux）。

**涉及文件**：
- `internal/store/store.go` — 新增，数据库初始化 + 通用接口
- `internal/store/providers.go` — 新增，provider CRUD
- `internal/store/sessions.go` — 新增，替代 `JSONSessionStore`
- `internal/store/migrations.go` — 新增，schema 版本管理

> **注意**：`config.yaml` 保留用于人类可编辑的少量启动配置（`selected_provider`、`workdir`、`shell`）。结构化业务数据（provider 列表、会话历史、缓存）迁移到 SQLite。

---

### Phase 3：体验增强（P2）

> 目标：改善用户侧体验。

#### 3.1 用户自定义 Provider 支持

**前置依赖**：Phase 2.5 统一本地存储层。

**方案**：

1. **持久化**：用户通过 `/provider add` 添加的 provider 写入 `providers` 表，启动时与内置列表合并（用户自定义优先级更高，同名覆盖）。
2. **配置校验**：`driver` 字段有效、`base_url` 格式合法、至少一个 model。
3. **文档与引导**：在 `docs/guides/adding-providers.md` 中补充完整示例。

#### 3.2 健康检查

**方案**：通过 `GET /models` 端点检测 provider 可达性，TUI 展示连接状态。

**涉及文件**：
- `internal/provider/health/health.go` — 新增

#### 3.3 成本追踪（远期可选）

**方案**：基于 `Usage` 数据 + 模型定价表，记录并展示 token 消耗和预估费用。

---

## 3. 优先级与依赖

```
Phase 1.1 错误码  ──→ Phase 1.2 传输重试（强依赖：需 Retryable 判断）
                                            │
Phase 2.1 元数据  ──────────────────────────┤ （独立路径）
Phase 2.2 动态发现 ─────────────────────────┤ （独立路径，缓存依赖 2.5）
Phase 2.3 API Key 安全 ─────────────────────┤ （独立路径）
Phase 2.5 统一存储  ────────────────────────┘ （独立路径，但 Phase 3.1 强依赖）
                    │
Phase 3.1 自定义 Provider ────────────────── （除Phase 2.5，可选其他方式存储）
Phase 3.2 健康检查  ──────────────────────── （独立）
Phase 3.3 成本追踪  ──────────────────────── （可选，依赖 2.1）
```

---

## 4. 兼容性说明

- **Phase 1 向后兼容**：`Provider` 接口不变，`ChatRequest` / `ChatResponse` 不变。`ProviderError` 通过类型断言使用，不影响现有 `error` 处理代码。
- **Phase 2 向后兼容**：`ModelDescriptor` 新增字段均为 `omitempty`，不配置元数据时行为与现有一致。动态发现为可选功能。`CredentialResolver` 默认实现与当前环境变量行为一致。统一存储层引入时，需编写迁移脚本将现有 `sessions/*.json` 导入 SQLite。
- **Phase 3 向后兼容**：用户自定义 provider 存入 SQLite，不影响内置 provider。启动时合并策略保证用户配置优先级。

---

## 5. 目录结构变更

```
internal/
├── provider/
│   ├── types.go                # 扩展 ModelDescriptor
│   ├── errors.go               # [新增] ProviderError
│   ├── metadata.go             # [新增] 静态模型元数据表
│   ├── service.go              # 注入元数据、集成动态发现
│   ├── registry.go             # 不变
│   ├── provider.go             # 不变
│   ├── credential/
│   │   ├── credential.go       # [新增] CredentialResolver
│   │   ├── mask.go             # [新增] API Key 脱敏
│   │   └── credential_test.go  # [新增]
│   ├── transport/
│   │   ├── retry.go            # [新增] 重试中间件
│   │   └── retry_test.go       # [新增]
│   ├── discovery/
│   │   ├── discovery.go        # [新增] 动态模型发现
│   │   └── discovery_test.go   # [新增]
│   └── health/
│       └── health.go           # [新增] 健康检查（Phase 3.2）
```

---

## 6. 设计原则

1. **最小闭环**：每次改动保持主链路可用（`TUI → Runtime → ProviderFactory → Registry.Build → Driver`），不做无关重构。Runtime 通过 `ProviderFactory` 接口解耦，不直接依赖 `Service`。
2. **不提前泛化**：当前只有 1 个 driver、4 个内置 provider，不为"未来可能的多 driver 场景"引入复杂抽象。
3. **务实优先**：错误码只区分可重试/不可重试；重试走 driver 内部 Transport 替换；API Key 管理保持环境变量方案，仅补充安全加固。
4. **配置安全**：遵循 AGENTS.md 规则，明文 API Key 不提交、不落盘日志。当前通过环境变量名入配置；后续 API Key 加密存储到 SQLite，密钥派生自机器指纹。
5. **测试同步**：修改 `provider`、`tools`、`runtime` 时同步补充测试。
