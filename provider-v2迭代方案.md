## 1. 文档目标

本文定义 NeoCode Provider V2 的演进方向。

Provider V2 的目标不是替换现有 `Provider.Chat` 主链路，而是在保持当前闭环可用的前提下，把 provider 层从“最小聊天适配层”升级为“模型执行边界”，为后续 runtime、tools、context、tui 的持续迭代提供稳定契约。

Provider V2 落地后，仓库内应同时存在两套能力：

- V1：继续服务当前 `Runtime -> Provider.Chat -> ToolCall -> ToolResult` 主链路
- V2：承载结构化输出、细粒度流式事件、原生协议差异抹平、response continuation、typed capabilities 等未来能力

## 2. 当前状态

当前 provider 层已经完成以下基础治理：

- Provider 错误类型与可重试语义
- HTTP 传输层重试
- 模型动态发现与本地缓存
- 模型描述合并与能力字段透传

当前 V1 契约仍然偏薄，主要表现为：

- `Message` 仍以单一 `Content string` 为中心，无法稳定承载 reasoning summary、artifact 引用、结构化 tool result 等输出
- `ChatResponse` 只返回单条 assistant message，缺少 response handle、provider stop reason、usage 明细、warning 等元数据
- `StreamEvent` 仅覆盖 `text_delta` 和 `tool_call_start`，不足以支撑 reasoning、tool 参数增量、usage 增量和完成事件
- `ToolResult.Metadata` 在运行链路中没有被 provider 契约承接，runtime 与 tui 当前只消费纯文本结果
- `ModelDescriptor.Capabilities` 仍为 `map[string]bool`，尚未形成 typed capabilities 契约

当前实现因此适合 MVP 闭环，但尚不足以承接以下能力：

- OpenAI Responses API / GPT-5 类模型的结构化 response item
- Anthropic Messages API 的 content block、tool_use、thinking block、prompt cache 语义
- runtime 基于模型能力做工具暴露、上下文压缩、并行工具调用和 continuation 策略分支
- tui 展示更细粒度的推理、工具调用和警告状态

## 3. 设计目标

Provider V2 采用以下目标：

1. Provider 层对上暴露统一的“单回合执行”契约，而不是继续围绕厂商消息格式建模。
2. Provider 层对下收敛 OpenAI Chat Completions、OpenAI Responses、Anthropic Messages 等原生协议差异。
3. runtime 不直接消费厂商 JSON，只消费结构化 turn request / turn response / stream event。
4. tools 层输出进入 provider 前先规范为模型可见 payload，再由 provider 负责映射到各协议的 tool result 结构。
5. typed capabilities、typed stop reason、usage 明细、response handle 等策略信息在 provider 层统一沉淀。
6. V2 与 V1 并存；V1 在迁移完成前继续保持可用。

## 4. 非目标

本设计不覆盖以下事项：

- runtime 层的多模型调度、fallback 编排和任务级降级
- 向量检索、仓库索引、repo map 等上下文召回方案
- 云端路由、代理服务或 SaaS 网关
- 替代现有 tool manager 与 security pipeline

## 5. Provider V2 职责边界

Provider V2 的职责固定为以下四类：

### 5.1 协议适配

- 将统一请求映射到目标厂商协议
- 解析原生响应与流式事件
- 抹平 tool use / function calling / response item 等协议差异

### 5.2 执行元数据归一化

- 暴露 typed stop reason
- 暴露 usage 明细与重试次数
- 保留 provider 原始 stop reason 与 response handle
- 透出 warning 与能力缺失信息

### 5.3 能力协商

- 提供 typed capabilities，供 runtime 和 tools 层做编排决策
- 对模型不支持的能力返回明确错误或降级信息

### 5.4 流式边界

- 统一输出 content delta、reasoning delta、tool call delta、usage、warning、completed 事件
- 不让 runtime 和 tui 直接感知厂商 SSE/stream chunk 格式

Provider V2 不负责：

- 工具执行
- 会话存储
- 权限审批
- 上下文裁剪策略本身

这些逻辑仍归 runtime、tools、context、security。

## 6. 核心数据模型

Provider V2 采用 turn-based 契约。

### 6.1 请求模型

- `TurnRequest`
  - 描述一次模型执行回合
  - 由 `instructions`、`input`、`tools`、`options` 和 `continue_from` 组成
- `TurnItem`
  - 表示单条输入或输出项
  - 使用 `role + parts` 组合承载结构化内容
- `ContentPart`
  - 作为 item 的最小内容单元
  - 当前草案覆盖 `text`、`reasoning_summary`、`tool_call`、`tool_result`、`artifact`

### 6.2 响应模型

- `TurnResponse`
  - 返回结构化输出项列表和统一元数据
- `ResponseMeta`
  - 记录 response handle、typed stop reason、provider stop reason、usage、warning、延迟和重试次数
- `ResponseHandle`
  - 承载 provider 后续 continuation 所需的句柄信息

### 6.3 能力模型

- `CapabilitySet`
  - 代替零散的 `map[string]bool`
  - 当前草案覆盖：
    - `ToolCall`
    - `ParallelToolCalls`
    - `Reasoning`
    - `ReasoningSummary`
    - `StructuredOutput`
    - `Vision`
    - `PromptCache`
    - `ResponseContinuation`

### 6.4 停止原因

`StopReason` 作为 provider 层统一停止语义，屏蔽厂商差异。

当前草案覆盖：

- `completed`
- `tool_call`
- `max_output_tokens`
- `context_window_exceeded`
- `content_filtered`
- `canceled`
- `error`

同时保留 `ProviderStopReason` 字段，用于调试与兼容未归一化的厂商语义。

### 6.5 流式事件

`StreamEventV2` 负责将 provider 流式输出桥接为稳定事件流。

当前草案覆盖：

- `content_delta`
- `reasoning_delta`
- `tool_call_delta`
- `tool_call_finished`
- `usage`
- `warning`
- `completed`

## 7. Driver 分层

V2 驱动采用“原生协议优先，兼容协议兜底”的分层策略。

### 7.1 `openai_chat_compat`

职责：

- 继续承接长尾 OpenAI-compatible 端点
- 兼容自定义 `base_url`
- 作为本地模型、自建代理和第三方平台的通用接入层

适用场景：

- 长尾 OpenAI-compatible 模型
- 仍以 chat completions 为主的供应商

### 7.2 `openai_responses`

职责：

- 承接 Responses API 语义
- 对齐 GPT-5 / Codex 类模型的 response item、continuation、reasoning summary、structured output 能力

适用场景：

- 需要 response continuation
- 需要更丰富的 usage / stop reason / structured output 语义

### 7.3 `anthropic_messages`

职责：

- 承接 Anthropic Messages API 原生 block 语义
- 对齐 tool_use、thinking block、prompt cache 相关能力

适用场景：

- Claude 原生模型
- 需要原生 thinking、tool_use、cache 语义

### 7.4 `gemini_native`

职责：

- 后续承接 Gemini 原生协议
- 对齐多模态输入、结构化输出和 Gemini 特有安全/返回结构

该驱动不在本阶段实现范围内，但 V2 契约需要为其预留空间。

## 8. 对上层模块的影响

### 8.1 Runtime

runtime 在 V2 里承担以下变化：

- 从 `ChatRequest` 过渡到 `TurnRequest`
- 将历史消息转换为 `TurnItem`
- 基于 `CapabilitySet` 决定是否启用并行工具调用、reasoning、structured output、continuation
- 使用 `StopReason`、`UsageDetails` 和 response handle 驱动上下文压缩与续写策略
- 在迁移阶段同时支持 V1 与 V2 provider

runtime 仍然不直接理解厂商协议。

### 8.2 Tools

tools 层在配套改造中需要补齐两类信息：

- 模型可见 payload
  - 提供给 provider 继续回灌模型
  - 包含文本结果、结构化结果、错误标志
- runtime 可见 metadata
  - 仅供 runtime、tui、安全策略消费
  - 包含 `truncated`、权限决策、artifact/path/url、执行摘要等信息

V2 的 provider 类型草案先定义模型侧的 `ToolDefinition` 与 `ToolResultPart`。tools 层的结果拆分在实现阶段同步引入，不在本次类型草案中落地。

### 8.3 Context

context 层在 V2 里需要使用以下信息：

- `ModelDescriptor` 的 typed capabilities
- `UsageDetails`
- `StopReason`

上下文裁剪策略将从纯轮次预算逐步过渡到 token budget + 轮次上限双重约束。

### 8.4 TUI

tui 在 V2 里直接受益于细粒度流式事件：

- reasoning 文本可以独立展示
- tool call 参数增量可以独立展示
- usage 与 warning 可以在状态栏或活动流中展示
- completed 事件可以更准确地驱动收尾状态

## 9. 兼容策略

Provider V2 采用双栈兼容：

1. 现有 `Provider` 接口与 `ChatRequest/ChatResponse/StreamEvent` 保持不变。
2. 新增 `ProviderV2` 接口与 V2 类型，不修改现有调用方。
3. runtime 在迁移阶段优先探测 provider 是否实现 `ProviderV2`。
4. 未实现 `ProviderV2` 的驱动继续走 V1。
5. V1 到 V2 的适配器在 runtime 层实现，不在 provider 层做反向污染。

## 10. 迁移阶段

### Phase 0：类型落盘

- 新增 `docs/provider-v2-design.md`
- 不改现有运行逻辑

### Phase 1：双栈接入

- runtime 增加 V2 provider 探测与调度
- 新增 V1/V2 适配器
- 保持当前 OpenAI-compatible 驱动继续走 V1

### Phase 2：原生驱动接入

- 落地 `openai_responses`
- 落地 `anthropic_messages`
- 将与厂商强相关的语义收敛到对应驱动内部

### Phase 3：上层能力升级

- runtime 接入 continuation
- runtime 接入基于 capabilities 的工具过滤与编排
- tools 接入模型 payload 与 runtime metadata 拆分
- tui 接入细粒度流式事件

### Phase 4：V1 收缩

- 在主要驱动完成 V2 接入后，逐步收缩 V1 的演进范围
