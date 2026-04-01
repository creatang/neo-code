# NeoCode 上下文模块下一阶段规划案

日期：2026-04-01
项目：NeoCode Coding Agent
阶段判断：MVP 主链路已基本跑通，适合进入“上下文能力提取与演进”阶段

---

## 1. 结论先行

就 NeoCode 当前状态来看，**下一阶段最合理的方向不是直接做“复杂记忆系统”，而是先把“上下文构建”从 runtime 中正式提取为独立模块**，然后按以下顺序推进：

1. 先做 **上下文模块抽离**，保证现有行为不变。
2. 再做 **显式上下文注入**，补齐项目规则、系统状态、工作目录上下文。
3. 然后做 **会话压缩 / compact**，解决上下文窗口增长问题。
4. 最后再做 **长期记忆 / memory**，先手动、后自动，先规则检索、后相似度检索。

这是最符合当前仓库边界、风险最低、收益最高的路线。

---

## 2. 规划依据

结合你提供的 4 份文档，可以提炼出几个对 NeoCode 很关键的原则：

### 2.1 Context Engineering 不是写一句 prompt，而是管理整套输入上下文

它至少包含三层：

- 内容层：给模型哪些信息
- 结构层：这些信息以什么顺序和形式进入模型
- 管理层：如何在有限上下文窗口内做筛选、压缩和动态注入

### 2.2 显式项目上下文和长期记忆要分层

- `CLAUDE.md / 项目规则文件` 这类内容，适合承载稳定的项目背景、规范和约束
- `Memory` 适合承载不可从代码直接推导的长期信息，例如历史决策、团队偏好、用户反馈

### 2.3 Compact 的目标不是截断，而是保留“继续工作所需的最少关键信息”

压缩必须保留：

- 已完成任务与结果
- 关键决策及原因
- 当前进行中的任务状态
- 关键代码改动摘要
- 用户偏好与约束

### 2.4 NeoCode 不能照搬 Claude Code 的实现细节

Claude Code 会把工具定义也视为 system prompt 的一部分，但 **NeoCode 当前架构已经把工具 schema 放在 `provider.ChatRequest.Tools` 中单独传递**。这套设计更符合你当前仓库的边界要求，所以这里应当借鉴“思想”，而不是机械照搬“工具写进 prompt”。

---

## 3. 当前项目状态评估

基于当前仓库实现，我对现状的判断如下。

### 3.1 已具备的基础

- 主链路已经可用：`TUI -> Runtime -> Provider -> Tools -> Runtime -> TUI`
- Provider 抽象已经稳定，OpenAI 兼容实现可工作
- Tools Registry 已具备统一 schema 和执行入口
- Session 已经通过 JSON 做了本地持久化
- Runtime 已有基本 ReAct loop、事件派发、工具结果回灌
- 2026-04-01 本地执行 `go test ./...` 通过

这说明：**现在有条件进入“能力提取与结构升级”阶段，而不是继续堆功能在 runtime 里。**

### 3.2 当前上下文能力的真实水平

目前的上下文能力仍然非常初级，主要表现为：

- `runtime.systemPrompt()` 仍是固定字符串
- 上下文裁剪依赖 `maxContextTurns = 10` 的固定消息轮次裁剪
- 只做了“最近消息保留”，没有做“相关性注入”
- 没有独立的 prompt/context builder
- 没有项目规则文件发现机制
- 没有 git 状态、workdir、shell 等系统上下文注入
- 没有 token 预算管理
- 没有 compact / summary 机制
- 没有长期 memory 存储和检索

### 3.3 当前最明显的架构机会点

当前 runtime 实际上承担了两种职责：

- ReAct 执行编排
- 上下文构建与裁剪

这两者应该拆开。  
**Runtime 应只负责“循环与编排”，Context 模块负责“构建这次请求该带什么上下文”。**

---

## 4. 下一阶段总目标

目标不是一次性做完整 memory 平台，而是先把 NeoCode 升级到下面这个层级：

> 给 Runtime 一个独立、可测试、可扩展的 Context Builder，能够稳定地构建：
> `核心指令 + 项目规则 + 系统状态 + 会话历史 + 压缩摘要 + 后续可扩展 memory`

建议把下一阶段定义为：

**Phase Context-1：提取上下文模块，建立显式上下文体系，为 compact 和 memory 打基础。**

---

## 5. 推荐落地路线

## 5.1 第一阶段：只做“上下文模块抽离”，不改行为

### 目标

把现在分散在 `runtime` 里的 system prompt 和 history trimming 提取到独立模块中，但先保持行为基本等价。

### 建议新增目录

```text
internal/context/
  builder.go
  types.go
  policy.go
  trim.go
  prompt.go
```

### 建议职责

- `builder.go`：统一构建入口
- `types.go`：定义 `BuildInput / BuildResult / Fragment`
- `prompt.go`：核心 system prompt 生成
- `trim.go`：历史裁剪逻辑
- `policy.go`：上下文预算和策略

### 推荐接口

```go
type Builder interface {
	Build(ctx context.Context, in BuildInput) (BuildResult, error)
}

type BuildInput struct {
	Session     runtime.Session
	UserInput   string
	Config      config.Config
	ToolSpecs   []provider.ToolSpec
}

type BuildResult struct {
	SystemPrompt string
	Messages     []provider.Message
	Metadata     BuildMetadata
}
```

### runtime 中的变化

Runtime 不再自己做：

- `systemPrompt()`
- `trimMessages()`

而是改为：

- 请求 `context.Builder.Build(...)`
- 使用返回的 `SystemPrompt + Messages`

### 这一阶段的验收标准

- 行为与当前基本一致
- 不引入 provider 特定字段到 context 模块
- runtime 代码更短，职责更纯
- 相关单测补齐

---

## 5.2 第二阶段：补齐“显式上下文源”

这是最值得做、也最容易立刻提升回答质量的一阶段。

### 建议先接入的上下文源

1. 核心系统指令
2. 项目规则文件
3. 系统环境摘要
4. 会话历史

### 2.1 项目规则文件发现

建议 NeoCode 支持多种显式规则文件，但要有主次：

- 首选：`AGENTS.md`
- 兼容：`NEOCODE.md`
- 可选兼容：`CLAUDE.md`

原因：

- 你当前仓库已经有 `AGENTS.md`
- 许多 AI 协作项目也已经在使用 `AGENTS.md`
- 这能让 NeoCode 更容易接入现有工程生态

### 2.2 文件发现策略

建议采用“当前工作目录向上查找”的多级合并机制：

```text
<workdir>/AGENTS.md
<workdir 的父目录>/AGENTS.md
...
用户级全局规则文件（后续再支持）
```

建议规则：

- 越靠近当前工作目录，优先级越高
- 多份文件可以合并，但要带来源标记
- 初期不做复杂 `@include`，先把主链路跑通

### 2.3 系统状态上下文

建议补充：

- 当前 workdir
- 当前 shell
- 当前 provider
- 当前 model
- git branch / dirty 状态 / 最近提交摘要

但要做长度控制，避免把噪声塞进 prompt。

### 2.4 这一阶段的关键提醒

不要把“工具 schema”并入 system prompt。  
继续保持：

- `context` 负责文本上下文
- `tools` 继续通过 `provider.ChatRequest.Tools` 传递

这符合你项目现有分层。

---

## 5.3 第三阶段：引入 compact，先手动再自动

这一步是从“能注入上下文”升级到“能管理上下文”。

### 为什么这里要排在 memory 前面

因为当前 NeoCode 还没有：

- token 预算感知
- 历史摘要
- 长会话稳定性控制

如果直接做 memory，会很容易把“历史消息”和“长期记忆”混在一起，最后上下文越来越乱。

### 建议实现顺序

1. 先做手动 `/compact`
2. 再做运行时 summary 存储
3. 最后再做自动 compact

### compact 的最小方案

- 保留最近 N 轮消息
- 保持 `assistant tool_call -> tool result` 配对不被切断
- 对更早历史生成摘要
- 用“摘要消息”替代旧消息片段

### 建议新增目录

```text
internal/context/compact/
  service.go
  summary.go
  boundary.go
```

### 建议不要一开始就做的东西

- 不要先做复杂 token 精算
- 不要先做多种 feature flag 压缩策略
- 不要先做 snippet 级裁剪

NeoCode 当前阶段先做：

- 简单可靠
- 行为可预测
- 测试容易写

就够了。

---

## 5.4 第四阶段：再进入长期 memory

当显式上下文和 compact 稳定后，再做长期 memory 才合理。

### 推荐的 memory 分层

建议借鉴文档里的分类，但做 NeoCode 版本收敛：

- `user`：用户偏好、沟通方式、常见要求
- `project`：项目决策、架构约束、非显式规则
- `feedback`：用户对 Agent 行为的纠偏
- `reference`：外部文档引用或长期有效知识

### 存储建议

建议独立于 session 存储：

```text
~/.neocode/projects/<project-hash>/memory/
```

而不要混到 session JSON 里。

### 第一版推荐只做“显式保存”

例如：

- `/remember`
- `/memory add`
- 或明确语言触发：“记住这条规则”

### 第一版不建议直接做“自动抽取”

因为自动抽取如果没有好的筛选机制，很容易把：

- 可从代码推导的信息
- 临时过程信息
- 一次性讨论内容

错误地写进长期 memory。

建议等显式保存稳定后，再加自动抽取。

---

## 6. 模块边界建议

结合当前仓库边界，我建议这样放职责。

### `internal/context`

负责：

- 收集上下文源
- 合并上下文
- 控制上下文预算
- 裁剪与压缩
- 未来的 memory 检索注入

不负责：

- 调用 provider
- 执行工具
- 直接驱动 UI

### `internal/runtime`

负责：

- 会话状态
- ReAct loop
- provider 调用
- tool 调用与结果回灌
- 事件派发

不负责：

- 拼接复杂 prompt
- 文件规则发现
- compact 策略细节

### `internal/config`

负责：

- context 相关配置项的加载、默认值、校验

### `internal/tui`

只负责：

- 展示 compact 提示
- 提供 `/compact`、后续 `/memory` 等命令入口

不要在 TUI 内直接读取上下文文件或 memory 文件。

---

## 7. 配置设计建议

建议在现有配置上新增 `context` 配置块，例如：

```yaml
context:
  max_history_turns: 10
  rules_files:
    - AGENTS.md
    - NEOCODE.md
    - CLAUDE.md
  git:
    enabled: true
    max_chars: 2000
  compact:
    enabled: false
    keep_recent_turns: 8
    warning_ratio: 0.85
    critical_ratio: 0.95
  memory:
    enabled: false
    max_injected_items: 5
```

建议策略：

- 第一阶段先只加 `max_history_turns` 和 `rules_files`
- 第二阶段再加 `compact`
- 第三阶段再加 `memory`

避免一次把配置做得过大。

---

## 8. 建议的开发顺序

### Milestone A：上下文模块抽离

目标：

- 新建 `internal/context`
- runtime 改为依赖 context builder
- 保持当前功能不变

### Milestone B：显式规则文件与系统状态注入

目标：

- 支持 `AGENTS.md` 等规则文件向上查找
- 支持 git/workdir/provider/model 注入
- 加长度截断和来源标识

### Milestone C：手动 compact

目标：

- 新增 `/compact`
- 生成历史摘要并替换老消息
- 保留最近消息和工具配对

### Milestone D：自动 compact

目标：

- 基于预算阈值自动触发
- UI 给出压缩反馈
- Runtime 对压缩结果可追踪

### Milestone E：显式 memory

目标：

- 增加 memory 存储目录
- 支持手动记忆保存
- 支持按任务关键词做基础检索

### Milestone F：自动 memory 提取

目标：

- 仅抽取高价值、不可推导、长期有效的信息
- 加过期检查和去重策略

---

## 9. 测试规划

按你的仓库规则，这部分必须提前设计，不要后补。

### `internal/context` 单测重点

- 多级规则文件发现顺序
- 缺失文件时的降级行为
- prompt 片段合并顺序
- 历史裁剪边界
- tool call / tool result 配对保留
- 上下文长度截断

### `runtime` 集成测试重点

- runtime 能正确调用 context builder
- context 构建失败时正确派发 error event
- compact 后仍能继续工具调用回合
- compact 后最终响应仍可正常产出

### `config` 测试重点

- 新增 context 配置默认值
- 非法阈值校验
- rules_files 空值或重复值处理

### 后续 `memory` 测试重点

- memory 分类写入
- 检索相关性排序
- 过期与去重
- 禁止保存可推导信息

---

## 10. 明确不建议现在做的事

为了符合当前项目阶段，我明确不建议一开始就做下面这些：

- 直接上向量数据库
- 直接上 embedding 检索
- 直接做自动记忆抽取
- 直接做复杂多策略 compact
- 把上下文模块做成跨 provider 的“大一统万能中心”
- 把所有历史消息都当成 memory

原因很简单：**当前最稀缺的不是“高级算法”，而是“边界清晰、行为稳定、易于测试的基础设施”。**

---

## 11. 我建议你现在就这样启动

如果按投入产出比排序，我建议你下一步实际开发就按下面 3 个小目标开工：

### 第一小步

先新增 `internal/context`，把下面两件事迁进去：

- `systemPrompt()`
- `trimMessages()`

做到“行为等价迁移”。

### 第二小步

让 context builder 开始读取工作目录向上的 `AGENTS.md`。

这是当前项目最容易立刻产生质量提升的一步，因为你仓库已经在用这套规则文件。

### 第三小步

补一个手动 `/compact`，哪怕第一版只是：

- 保留最近 8 轮
- 把更老内容总结成一条摘要消息

只要这一步能跑通，NeoCode 的上下文能力就会从“仅能对话”升级为“能管理长任务”。

---

## 12. 最终建议

对于 NeoCode 当前阶段，我的最终建议是：

> **先做 Context Builder，后做显式规则注入，再做 Compact，最后做长期 Memory。**

这条路线最符合你当前仓库的真实成熟度，也最符合仓库里强调的原则：

- 主链路优先
- 边界清晰
- 不跨层直连
- 不过度设计
- 变更可测试

