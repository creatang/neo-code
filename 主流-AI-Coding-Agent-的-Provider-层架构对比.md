## 1. 总览对比

| 维度 | NeoCode | OpenCode | Aider | Claude Code | Cline | Cursor | Windsurf |
|------|---------|----------|-------|-------------|-------|--------|----------|
| **语言** | Go | Go | Python | TypeScript (Bun) | TypeScript (Node) | TypeScript (Electron) | TypeScript (Electron) |
| **开源** | 是 | 是 | 是 | 否（已泄露） | 是 | 否 | 否 |
| **Provider 抽象** | 自研接口 | 自研接口（各厂商 Go SDK） | 自研上层 + LiteLLM 通信 | 自研（仅 Anthropic 生态） | 自研 Provider 接口（工厂模式） | 闭源，云端路由 | 闭源，自研为主 |
| **支持模型数** | 4（内置） | 75+（含用户自定义） | 200+ | Claude 全系列 + Bedrock + Vertex | 20+（含 OpenAI 兼容端点） | 5-10 | 自研 + 第三方 |
| **流式** | SSE chan | chan ProviderEvent | litellm stream | Anthropic SSE streaming | Anthropic/OpenAI SSE | 闭源 | 闭源 |
| **工具调用** | function calling | BaseTool 接口 | tool_use prompt（不依赖原生 function calling） | 原生 tool_use (40+ tools) | 原生 tool_use + MCP 扩展 | 系统提示词指导 | 全面 IDE 控制 |
| **认证** | 环境变量 | 环境变量 + OAuth + 自定义配置 | 环境变量 + .env + 自定义配置 | API Key + 订阅 + 原生客户端认证 | API Key（UI/SecretStorage） | 云端托管 / 用户 Key | 云端托管 |
| **自定义 Provider** | 不持久化（已知缺陷） | 支持（config.toml / opencode.json / 环境变量） | 通过 LiteLLM 配置 | 不支持（仅 Anthropic 生态） | 支持（OpenAI 兼容端点） | 不支持 | 不支持 |
| **上下文裁剪** | 轮次裁剪 (maxContextTurns=10) | 无明确裁剪 | 提示词窗口 + repo map | 自动 compaction + prompt cache | 无明确裁剪 | 语义检索 | 全项目索引 |

---

## 2. 各产品详细分析

### 2.1 NeoCode

**定位**：Go 实现的 TUI AI Coding Agent，最小可用闭环。

**Provider 层架构**：

```
TUI → Runtime → ProviderFactory(Build) → Registry.Build → Driver(OpenAI-compatible)
```

- **统一抽象**：`Provider` 接口（`Chat` 方法 + `chan<- StreamEvent`），所有驱动通过 OpenAI 兼容协议统一。
- **驱动注册**：`Registry` + `DriverDefinition`，当前仅注册了一个 `openai` 驱动，所有 4 个内置 provider 共用。
- **服务层**：`Service` 实现 `ProviderFactory` 接口，封装 provider 选择、模型列表、实例构建。
- **流式**：`StreamEvent` 类型枚举（`StreamText`、`StreamToolCall`、`StreamError`、`StreamDone`），通过 Go channel 传出。
- **工具调用**：runtime 将 `tools` 的 JSON Schema 透传给 provider，由 OpenAI 兼容协议处理 function calling。
- **认证**：`os.Getenv` 读取环境变量名（`ProviderConfig.APIKeyEnv`），CLI 行业标准做法。
- **上下文裁剪**：`trimMessages` 方法，硬编码 `maxContextTurns = 10` 按轮次裁剪。
- **模型元数据**：当前无模型能力元数据，runtime 不根据模型能力做策略决策。

**关键特点**：
- 纯 Go 实现，零 CGO 依赖
- 当前只支持 OpenAI 兼容协议，通过 `base_url` 适配不同厂商
- 用户自定义 provider 不持久化（YAML tag `"-"`），是已知缺陷
- 架构设计清晰，职责分层明确

### 2.2 OpenCode

**定位**：Go 实现的终端 AI Agent，支持 75+ LLM provider。

**Provider 层架构**：

```go
type Provider interface {
    SendMessages(ctx context.Context, messages []message.Message, tools []tools.BaseTool) (*ProviderResponse, error)
    StreamResponse(ctx context.Context, messages []message.Message, tools []tools.BaseTool) <-chan ProviderEvent
    Model() models.Model
}
```

- **统一抽象**：`Provider` 接口同时提供 `SendMessages`（非流式）和 `StreamResponse`（流式），通过工厂函数 `NewProvider(providerName, opts...)` 创建。
- **驱动实现**：采用两层架构——公共 `Provider` 接口供 Agent 消费，内部 `ProviderClient` 接口由各厂商客户端实现，`baseProvider` 包装器桥接两层。各 provider 使用**对应厂商的官方 Go SDK**（`anthropic.Client`、`openai.Client`、`genai.Client`），而非统一的 AI SDK 中间层。其中 GROQ、OpenRouter、xAI、Local 通过复用 `openai.Client` 并设置不同 `BaseURL` 实现。
- **支持的 provider**：Anthropic（含 Bedrock）、OpenAI、Gemini（含 VertexAI）、GROQ、OpenRouter、xAI、Local、GitHub Copilot 等内置 provider。
- **流式事件**：`ProviderEvent` 通过 Go channel 传递，事件类型丰富——`EventContentStart`、`EventContentDelta`、`EventThinkingDelta`、`EventToolUseStart/Delta/Stop`、`EventComplete`、`EventError`、`EventWarning`。
- **工具调用**：原生 `tools.BaseTool` 接口，`ProviderResponse.ToolCalls` 返回结构化的工具调用列表，含 `TokenUsage` 统计。
- **认证**：环境变量 + OAuth（针对 Copilot/OpenCode Zen），支持 `/connect` 命令交互式认证。
- **自定义 Provider 支持**：用户可通过多种方式接入自定义供应商：
  - **环境变量**：设置 `OPENAI_API_KEY` + `OPENAI_BASE_URL`（OpenAI 兼容）或 `ANTHROPIC_API_KEY` + `ANTHROPIC_BASE_URL`（Anthropic 兼容）。
  - **配置文件**：`config.toml` 中定义自定义 provider（指定 `api_key` 和 `base_url`），或 `opencode.json` 中使用 `@ai-sdk/openai-compatible` npm 包配置。
  - **模型格式**：使用 `vendor/model-name` 格式（如 `anthropic/claude-sonnet-4.6`）。
  - 凭据存储在 `~/.local/share/opencode/auth.json`，支持 75+ 预注册模型 + 用户自定义模型。
- **配置**：`config.toml` 集中管理 provider 和 model 配置，支持 per-model 全局设置。
- **上下文裁剪**：无显式的自动裁剪机制，依赖模型本身的上下文窗口限制。

**与 NeoCode 的对比**：
- OpenCode 的 `Provider` 接口更完整，同时暴露了流式和非流式能力
- 事件类型粒度更细（区分 content/thinking/tool use 的 start/delta/stop）
- 采用各厂商官方 Go SDK 而非统一协议，协议差异在内部 `ProviderClient` 层收敛
- GROQ/OpenRouter/xAI/Local 复用 `openai.Client` + BaseURL，说明其也依赖 OpenAI 兼容协议覆盖长尾 provider
- 原生支持用户自定义 provider（config.toml / 环境变量 / opencode.json），自定义 provider 不持久化是 NeoCode 的已知缺陷，而 OpenCode 已解决
- Token 使用统计是响应的一部分

### 2.3 Aider

**定位**：Python 终端 AI Pair Programming 工具，支持 200+ 模型。

**Provider 层架构**：

```
Aider CLI → Model (models.py) → LiteLLM → 各厂商 API
```

- **统一抽象**：将 LLM 通信委托给 [LiteLLM](https://github.com/BerriAI/litellm)，但**在 LiteLLM 之上构建了丰富的自研适配层**。`Model` 类是上层业务抽象，管理提示词编排、repo map 生成、diff 应用等。自研部分包括：
  - **模型注册与发现**：模糊匹配、模型别名系统（如 `sonnet` → Claude 3 Sonnet）
  - **成本追踪**：`model-metadata.json` 管理技术规格，自研成本计算流程
  - **无限输出续写**：针对支持助手预填充的模型（Claude、DeepSeek），检测截断并拼接续写
  - **推理模型适配**：为 OpenAI o1/o3、DeepSeek R1 等配置 `thinking_tokens`、`reasoning_effort`
  - **Provider 特定功能**：提示缓存配置、OpenRouter OAuth 认证、差异化错误重试策略
- **LiteLLM 的价值**：LiteLLM 提供统一的 `completion()` 接口，内部完成 OpenAI / Anthropic / Google / Azure / Bedrock 等上百个 provider 的协议转换、重试、限流、计费。Aider 只需 `litellm.completion(model=model_name, messages=..., stream=True, tools=...)`。
- **双模型架构**：支持 Architect/Editor 模式——主模型（如 Claude Sonnet）做代码推理，编辑模型（如 GPT-4o-mini）做 diff 编辑。还有 weak_model 用于摘要等轻量任务。三个模型可以来自不同 provider。
- **流式**：通过 `litellm.completion(stream=True)` 获取流式响应，Aider 在上层处理 token 累积和展示。
- **工具调用**：**不使用原生 function calling**，而是将工具能力编码到系统提示词中（repo map、file edit、shell command 等），模型输出特定格式的文本指令，Aider 解析执行。这意味着 Aider 能兼容不支持 function calling 的模型。
- **认证**：环境变量（`ANTHROPIC_API_KEY`、`OPENAI_API_KEY` 等），也支持 `.env` 文件。LiteLLM 支持虚拟 key 管理。
- **上下文裁剪**：repo map 策略——对代码库生成结构摘要（文件树 + 每个文件的类/函数签名），只将相关摘要注入提示词而非全文。有 `max-chat-history-tokens` 配置项控制上下文窗口。

**关键特点**：
- **底层通信依赖 LiteLLM**，但自研适配层非常丰富，不是 LiteLLM 的简单包装
- 提示词工程替代原生工具调用，兼容性极广但精度较低
- 双/三模型架构是独特设计，通过分工提升效率

### 2.4 Claude Code

**定位**：Anthropic 官方 CLI Agent，深度绑定 Claude 生态。

**Provider 层架构**：

```
Claude CLI → src/services/api → Anthropic API / Bedrock / Vertex AI
```

- **Provider 抽象**：**仅支持 Anthropic 生态**——Anthropic 直连、AWS Bedrock、Google Vertex AI 三个后端。不需要通用 provider 抽象，因为所有后端都使用 Anthropic Messages API。
- **流式**：使用 Anthropic SSE streaming，终端渲染层有游戏引擎级别的优化——`Int32Array` 字符池、位掩码样式编码、补丁合并器，声称"token streaming 期间 stringWidth 调用减少 ~50x"。
- **工具调用**：原生 `tool_use`，内置 40+ 工具（文件读写、终端执行、git 操作、MCP 工具等）。每条 bash 命令经过 23 项安全检查（阻止 Zsh 内建命令、等号展开绕过、unicode 零宽空格注入等）。
- **认证**：多层机制：
  - API Key（`ANTHROPIC_API_KEY`）
  - 订阅认证（Claude Pro/Max）
  - **原生客户端认证**（Native Client Attestation）：Bun 的 Zig 原生 HTTP 栈在请求发出前替换 `cch=00000` 占位符为计算哈希，证明请求来自真实 Claude Code 二进制
- **上下文管理**：
  - 自动 compaction（`autoCompact`）——超出上下文窗口时自动摘要
  - **Prompt 缓存**重度优化：跟踪 14 种缓存失效向量，使用"sticky latch"防止模式切换导致缓存失效
  - Connector-text summarization（反蒸馏机制）——工具调用间的助手文本被服务端摘要并签名，后续可恢复
- **多 Agent**：`coordinatorMode` 支持多 Agent 协作，编排算法通过**系统提示词**实现（非代码），指导 worker agent 的工作分配和审查。

**关键特点**：
- 单 provider 生态，无通用抽象需求
- Prompt 缓存经济学驱动架构设计（每 token 都有成本）
- 反蒸馏机制（假工具注入、connector-text 摘要）是独特设计
- 安全模型极其细致（Zsh 威胁模型、shell 注入防护）

### 2.5 Cline

**定位**：VS Code 开源 AI Agent 插件，支持多种 LLM provider。

**Provider 层架构**：

```
VS Code Extension → ApiHandler (抽象基类) → AnthropicProvider / OpenAiProvider / ...
```

- **Provider 抽象**：`ApiHandler` 接口定义了统一的 `createMessage()` 方法，通过工厂模式（`src/api/index.ts`）根据 `ApiProvider` 枚举动态创建对应处理器。各 provider 实现：
  - `AnthropicProvider`：直连 Anthropic API
  - `OpenAiProvider`：OpenAI + **OpenAI 兼容 API**（包括 OpenRouter、本地 Ollama、LM Studio 等，通过 `openAiBaseUrl` 配置自定义端点）
  - `BedrockProvider`：AWS Bedrock
  - `VertexProvider`：Google Vertex AI
  - `AzureProvider`：Azure OpenAI
  - `GeminiProvider`：Google Gemini
  - 另支持 Groq、Together AI、Perplexity AI、xAI、Replicate、Fireworks AI、Nvidia NIM、Lepton AI、DeepSeek、01.ai 等，**总计 20+ provider**
- **自定义 Provider 支持**：用户可通过选择 `openai` 类型并配置自定义 `openAiBaseUrl` 来接入任何 OpenAI 兼容 API 端点（如 LocalAI、text-generation-webui、自托管模型等），无需修改代码。对于协议不兼容的全新 provider，需修改核心代码（扩展 `ApiProvider` 枚举、实现新 `ApiHandler`、更新工厂函数和 UI 组件）。
- **流式**：各 provider 独立实现 SSE 流式解析，统一转换为 Cline 内部的消息事件。
- **工具调用**：使用 provider 的原生 tool_use 能力，支持 Bash、File Read/Write/Edit、Browser Action 等内置工具，也支持 MCP 扩展工具。
- **认证**：API Key 在 UI 中配置，存储在 VS Code 的 globalState / secretStorage 中。支持 LiteLLM Proxy 作为统一中转（企业场景）。
- **上下文裁剪**：无显式自动裁剪，依赖模型的上下文窗口限制。用户可通过"compact"命令手动压缩对话。

**关键特点**：
- 面向 VS Code 生态，集成度优先
- 支持 20+ provider，其中 OpenAI 兼容端点覆盖了大部分自定义场景
- 支持 LiteLLM Proxy 作为企业级 API 网关
- 支持 MCP 扩展工具
- 添加全新协议的 provider 需修改核心代码，对最终用户不是零配置功能

### 2.6 Cursor

**定位**：AI-native IDE（VS Code fork），闭源商业化产品。

**Provider 层架构**：

```
Cursor IDE → Cursor Cloud (路由层) → OpenAI / Anthropic API
```

- **Provider 抽象**：**闭源**。所有 LLM 请求通过 Cursor 云端路由，用户无法直接控制 provider 选择逻辑。Cursor 充当 API 网关，在服务端决定使用哪个模型。
- **支持的 provider**：OpenAI（GPT-4o/mini 等）、Anthropic（Claude Sonnet/Opus），以及 Cursor 自研的 fast 模型（推测为小参数量模型用于快速补全）。
- **流式**：闭源，推测使用各 provider 的原生 SSE。
- **工具调用**：通过**系统提示词**精细指导模型使用 IDE 内置工具（文件编辑、终端、搜索等）。与 Aider 类似，不完全依赖原生 function calling，但 Cursor IDE 层面有更深度的工具集成。
- **认证**：
  - 订阅制（Cursor Pro/Business），API 调用费用包含在订阅中
  - 也可使用用户自己的 API Key（BYOK），但请求仍经过 Cursor 服务器
- **上下文管理**：
  - 启动时对项目构建**向量索引**，支持语义搜索
  - `.cursor/rules` 文件作为项目级长期记忆
  - 会话历史不自动跨会话保存

**关键特点**：
- 云端路由架构，provider 层对用户完全不可见
- 语义索引是核心能力，上下文检索质量高
- 不支持离线/本地模型

### 2.7 Windsurf (Codeium)

**定位**：Codeium 的 AI-native IDE（VS Code fork），Cascade Agent 系统。

**Provider 层架构**：

```
Windsurf IDE → Cascade Engine → Codeium Cloud → 自研模型 / 第三方 API
```

- **Provider 抽象**：**闭源**。以 Codeium 自研代码模型为主（包括 "Sonnet" 级别模型），企业用户可自选引擎或私有部署。
- **上下文管理**：
  - 本地预扫描全代码库构建语义索引（类似 Cursor）
  - **Memories 系统**：自动持久化保存会话内容和用户规则，实现**跨会话记忆**
  - 实时集成 IDE 状态（光标位置、终端输出）作为上下文
- **流式**：Flows 模式实现实时人机交互，AI 动作与开发者操作同步。
- **工具调用**：最全面的 IDE 控制能力——文件操作、终端执行、测试运行、浏览器控制等，通过 MCP 插件扩展。

**关键特点**：
- 跨会话记忆（Memories）是区别于 Cursor 的核心特性
- 实时 IDE 状态感知提供更丰富的上下文
- Flows 模式强调人机协作的流畅性

---

## 2.8 供应商与模型管理

### 2.8.1 供应商管理机制

各 Agent 在供应商注册、发现和配置方面的实现差异:

| 维度 | NeoCode | OpenCode | Aider | Claude Code | Cline | Cursor | Windsurf |
|------|---------|----------|-------|-------------|-------|--------|----------|
| **供应商注册** | Registry + DriverDefinition | 预注册 + 用户自定义配置 | LiteLLM 内置 + model-metadata.json | 硬编码 (仅 Anthropic 生态) | 预注册工厂模式 | 云端路由 | 云端路由 |
| **供应商配置存储** | YAML (`~/.neocode/config.yaml`) | config.toml + opencode.json + auth.json | `.env` + `litellm` 配置 | 环境变量 + 客户端配置 | VS Code globalState | 云端账户系统 | 云端账户系统 |
| **供应商发现** | 预定义列表 (4个内置) | 75+ 预注册 + 用户自定义 | 200+ 模型,模糊匹配 | 单一供应商 | 枚举列表 (20+) | 不暴露 | 不暴露 |
| **自定义供应商支持** | 不持久化 (已知缺陷) | 完整支持 (3种配置方式) | 完整支持 (LiteLLM 配置) | 不支持 | OpenAI 兼容端点 | 不支持 | 不支持 |

### NeoCode 的供应商管理

```go
// Registry 注册机制
type Registry struct {
    drivers map[string]DriverDefinition
}

type DriverDefinition struct {
    Name        string
    Builder     DriverBuilder
    Description string
}
```

- **当前实现**: 仅注册一个 `openai` 驱动,通过 `base_url` 适配不同厂商
- **配置方式**: `ProviderConfig` 中指定 `driver: openai` 和 `base_url`
- **缺陷**: 用户自定义 provider 的配置字段带 `yaml:"-"` 标签,不持久化

### OpenCode 的供应商管理

```toml
# config.toml 示例
[providers.openai]
api_key_env = "OPENAI_API_KEY"
base_url = "https://api.openai.com/v1"

[providers.custom]
api_key = "sk-xxx"  # 或使用 api_key_env
base_url = "https://custom-llm.example.com/v1"
```

- **三层配置**:
  1. 环境变量 (`OPENAI_API_KEY` + `OPENAI_BASE_URL`)
  2. `config.toml` (结构化配置)
  3. `opencode.json` (使用 `@ai-sdk/openai-compatible`)
- **凭据存储**: `~/.local/share/opencode/auth.json` (OAuth 场景)
- **模型格式**: `vendor/model-name` (如 `anthropic/claude-sonnet-4.6`)

### Aider 的供应商管理

```bash
# LiteLLM 支持的模型格式
aider --model openai/gpt-4
aider --model anthropic/claude-3-sonnet
aider --model openrouter/anthropic/claude-3.5-sonnet
```

- **核心机制**: 完全委托给 LiteLLM,上层无供应商概念
- **模型发现**: `Model` 类实现模糊匹配和别名系统 (如 `sonnet` → Claude 3 Sonnet)
- **元数据管理**: `model-metadata.json` 存储技术规格和成本信息

### Cline 的供应商管理

```typescript
// 工厂模式创建 provider
export function buildApiHandler(options: ApiOptions): ApiHandler {
  switch (options.apiProvider) {
    case 'anthropic':
      return new AnthropicProvider(options)
    case 'openai':
      return new OpenAiProvider(options) // 支持 openAiBaseUrl
    case 'bedrock':
      return new BedrockProvider(options)
    // ... 20+ provider
  }
}
```

- **枚举驱动**: `ApiProvider` 枚举定义所有支持的供应商
- **自定义端点**: 选择 `openai` 类型 + 配置 `openAiBaseUrl`
- **扩展限制**: 添加全新协议的供应商需修改核心代码

---

## 2.8.2 模型信息管理

| 维度 | NeoCode | OpenCode | Aider | Claude Code | Cline | Cursor | Windsurf |
|------|---------|----------|-------|-------------|-------|--------|----------|
| **模型列表来源** | 硬编码 (4个) | 预注册 + 用户自定义 | LiteLLM 内置 + metadata | Claude 全系列 | 预定义列表 | 不暴露 | 不暴露 |
| **模型元数据** | 无 | Token 限制等基础信息 | 完整技术规格 + 成本 | Prompt cache 配置 | 上下文窗口信息 | 不暴露 | 不暴露 |
| **模型选择方式** | 配置文件 | CLI 参数 / 配置文件 | CLI 参数 (模糊匹配) | CLI 参数 | UI 下拉选择 | UI 选择 | UI 选择 |
| **动态模型发现** | 不支持 | 不支持 | LiteLLM 自动发现 | 不适用 | 不支持 | 不暴露 | 不暴露 |

### Aider 的模型元数据管理 (最完善)

```json
// model-metadata.json 示例
{
  "gpt-4": {
    "max_tokens": 8192,
    "input_cost_per_token": 0.00003,
    "output_cost_per_token": 0.00006,
    "supports_function_calling": true,
    "supports_vision": true
  }
}
```

- **技术规格**: 上下文窗口、输出限制、能力标志
- **成本计算**: 输入/输出 token 单价,实时追踪成本
- **推理模型**: 为 o1/o3/R1 配置 `thinking_tokens`、`reasoning_effort`
- **提示缓存**: Anthropic 提示缓存配置

## 2.8.3 API Key 管理机制

| 维度 | NeoCode | OpenCode | Aider | Claude Code | Cline | Cursor | Windsurf |
|------|---------|----------|-------|-------------|-------|--------|----------|
| **存储方式** | 环境变量 | 环境变量 + 配置文件 + auth.json | 环境变量 + .env | 环境变量 + 客户端认证 | VS Code SecretStorage | 云端托管 / 用户 Key | 云端托管 |
| **安全级别** | 低 | 中 (OAuth 加固) | 低 | 极高 (二进制认证) | 中 (OS 密钥环) | 高 | 高 |
| **凭据轮换** | 手动 | 手动 / OAuth 刷新 | 手动 | 自动 (订阅制) | 手动 | 自动 | 自动 |
| **多账户支持** | 不支持 | 支持 (auth.json) | 支持 | 支持 (订阅切换) | 支持 | 支持 | 支持 |

### 环境变量方式 (NeoCode / Aider / OpenCode 部分场景)

```bash
# 标准做法
export ANTHROPIC_API_KEY="sk-ant-xxx"
export OPENAI_API_KEY="sk-xxx"

# NeoCode 配置中引用环境变量名
providers:
  anthropic:
    api_key_env: "ANTHROPIC_API_KEY"  # 存储变量名,不是值
```

- **优点**: 简单通用,符合 CLI 工具惯例
- **缺点**:
  - 进程环境可见 (`/proc/*/environ`)
  - 配置文件泄露环境变量名
  - 无加密保护
  - 难以管理多账户

### 配置文件方式 (OpenCode)

```toml
# config.toml
[providers.openai]
api_key = "sk-xxx"  # 明文存储,不推荐

# 推荐方式: 使用环境变量引用
[providers.openai]
api_key_env = "OPENAI_API_KEY"
```

- **凭据存储**: `~/.local/share/opencode/auth.json` (OAuth 场景)
- **安全建议**: 生产环境优先使用环境变量引用 (`api_key_env`)

### VS Code SecretStorage (Cline)

```typescript
// 使用 VS Code 的 secretStorage API
const secretStorage = context.secrets
await secretStorage.store('apiKey', 'sk-xxx')
const key = await secretStorage.get('apiKey')
```

- **底层实现**: 操作系统密钥环
  - Windows: Windows Credential Manager
  - macOS: Keychain
  - Linux: Secret Service API (如 GNOME Keyring)
- **优点**: 操作系统级加密保护
- **缺点**: 仅适用于 VS Code 生态

### 订阅制认证 (Claude Code / Cursor / Windsurf)

```
Claude Code 认证流程:
1. 用户订阅 Claude Pro/Max
2. 客户端获取 OAuth token
3. 请求中携带原生客户端证明
   - Bun Zig 原生 HTTP 栈替换 cch=00000 占位符
   - 服务端验证二进制签名
```

- **优点**: 最高安全级别,无 API Key 泄露风险
- **缺点**: 完全依赖供应商,无法自托管

### Claude Code 的原生客户端认证

```typescript
// 请求发出前动态计算哈希证明
headers["cch"] = calculateClientAttestationHash(request)
```

- **机制**: 二进制级别的请求证明,证明请求来自真实 Claude Code 客户端
- **防伪造**: 无法用 curl/Postman 等工具伪造请求
- **适用场景**: 商业 CLI 工具的防破解措施

---

## 3. 关键设计维度对比

### 3.1 Provider 抽象策略

| 策略 | 采用者 | 优点 | 缺点 |
|------|--------|------|------|
| **自研统一接口** | NeoCode、OpenCode、Cline | 完全控制行为，性能优化空间大 | 需要为每个 provider 写适配代码 |
| **委托第三方 SDK** | Aider (LiteLLM) | 通信层零适配代码，模型覆盖极广 | 依赖第三方维护通信层，行为不完全可控 |
| **单 provider 生态** | Claude Code | 无抽象开销，深度优化 | 无法切换到其他厂商 |
| **云端路由** | Cursor、Windsurf | 用户无感知，灵活切换 | 闭源，依赖网络，隐私顾虑 |

> **说明**：OpenCode 的"自研接口"不同于 NeoCode 和 Cline——OpenCode 内部使用各厂商的官方 Go SDK（`anthropic.Client`、`openai.Client`、`genai.Client`），而 NeoCode 和 Cline 的 OpenAI 兼容 provider 是直接 HTTP 调用而非 SDK。Aider 虽然通信层依赖 LiteLLM，但在其上构建了大量自研适配逻辑（模型发现、成本追踪、无限续写等），并非简单的 SDK 包装。

**NeoCode 的选择**：自研统一接口 + OpenAI 兼容协议。当前所有 provider 共用 `openai` 驱动，通过 `base_url` 适配。这是 MVP 阶段的合理选择——适配成本最低。但随着需要支持 Anthropic 原生协议（tool_use 格式差异、thinking block 等），纯 OpenAI 兼容会暴露局限性。

### 3.2 流式实现

| 方案 | 事件粒度 | 代表 |
|------|---------|------|
| **Go channel + 类型枚举** | 中等（text/tool/error/done） | NeoCode |
| **Go channel + 细粒度事件** | 高（content/thinking/tool 各有 start/delta/stop） | OpenCode |
| **Python generator (litellm stream)** | 低（token 级别，上层自行解析） | Aider |
| **TypeScript SSE + 自定义渲染** | 高（带游戏引擎级渲染优化） | Claude Code |
| **各 provider 独立解析** | 中等 | Cline |
| **闭源** | 未知 | Cursor、Windsurf |

**NeoCode 的建议**：OpenCode 的事件粒度是值得参考的方向——区分 thinking 和 content 事件对于支持 Claude 的 extended thinking 等特性至关重要。

### 3.3 工具调用

| 方案 | 代表 | 说明 |
|------|------|------|
| **原生 function calling** | Claude Code、Cline、OpenCode | 直接使用 provider 的 tool_use 协议，精度高但依赖模型支持 |
| **提示词工程** | Aider、Cursor | 将工具能力编码到系统提示词，兼容性广但解析复杂 |
| **Schema 透传** | NeoCode | 将 JSON Schema 发给 provider，依赖 provider 的 function calling |

**NeoCode 的现状**：Schema 透传给 OpenAI 兼容协议，实际依赖底层模型的 function calling 能力。如果未来支持非 OpenAI 兼容的 provider（如 Anthropic 原生），需要自行处理 tool_use 格式转换。

### 3.4 认证管理

| 方案 | 代表 | 安全级别 |
|------|------|---------|
| **环境变量** | NeoCode、Aider、Cline | 低（进程可见，配置文件可见变量名） |
| **VS Code SecretStorage** | Cline (VS Code 集成时) | 中（OS 密钥环） |
| **订阅制** | Claude Code、Cursor、Windsurf | 高（服务端管理） |
| **原生客户端认证** | Claude Code | 极高（二进制级别的请求证明） |
| **OAuth** | OpenCode (Copilot) | 高 |

**NeoCode 的现状**：纯环境变量，最低安全级别。文档中已规划 SQLite + AES-GCM 加密存储作为后续演进方向。

### 3.5 上下文裁剪

| 策略 | 代表 | 说明 |
|------|------|------|
| **轮次预算** | NeoCode (maxContextTurns=10) | 简单可靠，不依赖 token 计算 |
| **repo map 摘要** | Aider | 结构化代码摘要（文件树 + 类/函数签名），按需注入 |
| **自动 compaction** | Claude Code | 超出窗口时自动摘要压缩 |
| **向量语义检索** | Cursor、Windsurf | 构建代码向量索引，语义搜索注入相关片段 |
| **prompt cache** | Claude Code | 缓存系统提示词前缀，减少重复 token 消耗 |
| **无显式裁剪** | OpenCode、Cline | 依赖模型自身的上下文窗口限制 |

**NeoCode 的建议**：Claude Code 的"自动 compaction"是最实用的方案——当消息列表过长时，调用模型对历史做摘要压缩，而非简单截断。这比纯轮次预算更智能，比向量检索更轻量。

---

## 4. 对 NeoCode 的启示

### 4.1 可以直接借鉴的设计

| 设计 | 来源 | 适用性 |
|------|------|--------|
| **细粒度流式事件** | OpenCode (EventContentStart/Delta/Stop, EventThinkingDelta, EventToolUse*) | 高——为 Anthropic thinking、tool use 提供更好的 UI 反馈 |
| **TokenUsage 统计** | OpenCode (ProviderResponse.Usage) | 高——成本追踪的基础数据 |
| **自动 compaction** | Claude Code (autoCompact) | 高——替代硬编码轮次裁剪 |
| **请求重试中间件** | Aider (通过 litellm 内置) + Claude Code (指数退避) | 中——Phase 1.2 已规划 |
| **Provider 工厂模式** | OpenCode (NewProvider + options) | 已有类似设计 |

### 4.2 不建议采纳的设计

| 设计 | 来源 | 原因 |
|------|------|------|
| **原生客户端认证** | Claude Code | 过度设计，CLI 工具不需要 DRM 级别的认证 |
| **全向量索引** | Cursor / Windsurf | 依赖嵌入模型，引入外部依赖，MVP 不需要 |
| **提示词工程替代 tool calling** | Aider | 精度低、解析复杂，原生 function calling 是更好的选择。但此策略使 Aider 能兼容不支持 function calling 的模型，是其 200+ 模型覆盖的关键 |
| **云端路由** | Cursor | 与"本地优先"的设计理念矛盾 |

### 4.3 NeoCode 的独特优势

1. **纯 Go 实现**：与 OpenCode 同为 Go 生态，但 NeoCode 的架构分层更清晰（Runtime → ProviderFactory → Registry，各层职责边界明确）。
2. **零外部 LLM 依赖**：不像 Aider 依赖 LiteLLM，NeoCode 的 provider 层完全自研，行为完全可控（但代价是需要自行适配每个新 provider）。
3. **AGENTS.md 规范**：有完善的 AI 协作规则文档，这在其他开源项目中很少见。

---

## 5. 参考来源

- [OpenCode 官方文档 - 提供商配置](https://opencode.ai/docs/zh-cn/providers/) — provider 配置方式、自定义 provider 支持、环境变量/配置文件/opencode.json 三种配置方式
- [OpenCode 源码](https://github.com/opencode-ai/opencode) — `internal/llm/provider/`
- [DeepWiki - OpenCode LLM Providers](https://deepwiki.com/opencode-ai/opencode/3.2-llm-providers) — 两层架构、Go SDK 集成、工厂模式
- [Aider 源码](https://github.com/Aider-AI/aider) — `aider/models.py`
- [DeepWiki - Aider Multi-Provider LLM Integration](https://deepwiki.com/Aider-AI/aider/7.3-multi-provider-llm-integration) — LiteLLM + 自研适配层的完整分析
- [Aider 高级模型设置](https://aider.chat/docs/config/adv-model-settings.html) — 自定义模型元数据配置
- [Claude Code 源码分析](https://alex000kim.com/posts/2026-03-31-claude-code-source-leak/)
- [Claude Code 源码文档](https://github.com/ComeOnOliver/claude-code-analysis)
- [Cline 源码](https://github.com/cline/cline)
- [Cline - VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=saoudrizwan.claude-dev) — 支持的 provider 列表
- [DeepWiki - Cline API Configuration](https://deepwiki.com/char8x/cline/3.1-api-configuration) — 20+ provider、自定义端点支持
- [Agent 系统架构对比](https://cuckoo.network/blog/2025/06/03/coding-agent)

---

