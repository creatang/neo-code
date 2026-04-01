# NeoCode 上下文模块阶段规划

日期：2026-04-01  
项目：NeoCode Coding Agent  
阶段：MVP 主链路已跑通，进入上下文能力建设阶段

---

## 1. 规划结论

NeoCode 下一阶段不应直接进入复杂记忆系统建设，而应先将上下文构建能力从 `runtime` 中抽离，形成独立的 `Context Builder` 模块。

推荐推进顺序如下：

1. 抽离上下文模块，保持现有行为基本不变。
2. 接入显式上下文源，补齐规则文件和系统状态。
3. 引入 `compact`，解决长会话上下文膨胀问题。
4. 最后建设长期 `memory`，先手动保存，再逐步自动化。

这条路线风险最低，也最符合当前仓库的成熟度与边界设计。

---

## 2. 当前问题

当前 `runtime` 同时承担了两类职责：

- ReAct 循环与工具编排
- system prompt 生成与历史裁剪

这会带来几个直接问题：

- 上下文能力缺少独立承载点，后续扩展会持续挤压 `runtime`
- 项目规则、环境状态、压缩摘要、长期记忆都没有统一接入位置
- 上下文策略难以单测，也不利于后续演进

当前能力短板主要包括：

- `systemPrompt()` 仍是固定字符串
- 仅按固定轮次裁剪消息历史
- 缺少规则文件发现与注入
- 缺少 git、workdir、shell 等系统上下文
- 缺少 `compact` 和长期 `memory`

---

## 3. 目标与范围

### 目标

- 建立独立的 `internal/context` 模块
- 统一输出本轮请求所需的 `SystemPrompt`、`Messages`、`Metadata`
- 保持 `runtime` 聚焦循环、provider 调用、工具执行和事件派发
- 为规则注入、`compact`、`memory` 提供稳定扩展点

### 非目标

当前阶段不做以下内容：

- 向量数据库
- embedding 检索
- 自动记忆抽取
- 多策略复杂压缩
- 将 tool schema 合并进 system prompt

原则是先把基础设施做稳，再扩展能力。

---

## 4. 分阶段路线

| 阶段 | 目标 | 关键产出 | 验收标准 |
| --- | --- | --- | --- |
| Phase 1 | 抽离上下文模块 | `internal/context`、统一 `Builder` 接口、迁移 `systemPrompt()` 与历史裁剪 | 行为与当前基本一致，`runtime` 职责变纯，单测补齐 |
| Phase 2 | 引入显式上下文源 | 规则文件发现、系统状态摘要注入 | 支持 `AGENTS.md` 等规则文件，能注入 workdir、shell、provider、model、git 摘要 |
| Phase 3 | 引入 `compact` | 手动 `/compact`、历史摘要替换旧消息 | 能保留最近消息与工具调用配对，长会话可稳定继续 |
| Phase 4 | 建设长期 `memory` | 独立存储目录、手动记忆写入、基础检索 | 能保存高价值长期信息，不与会话历史混用 |

---

## 5. 模块设计建议

建议新增目录：

```text
internal/context/
  builder.go
  types.go
  prompt.go
  trim.go
  policy.go
```

建议核心接口：

```go
type Builder interface {
	Build(ctx context.Context, in BuildInput) (BuildResult, error)
}

type BuildInput struct {
	Session   SessionView
	UserInput string
	Config    config.Config
	ToolSpecs []provider.ToolSpec
}

type BuildResult struct {
	SystemPrompt string
	Messages     []provider.Message
	Metadata     BuildMetadata
}
```

职责边界建议如下：

- `internal/context`：负责上下文收集、拼装、裁剪、压缩与后续 memory 注入
- `internal/runtime`：负责会话状态、ReAct loop、provider 调用、工具回灌
- `internal/config`：负责 context 配置项定义、默认值与校验
- `internal/tui`：只提供 `/compact`、`/memory` 等入口与展示，不直接读取上下文文件

---

## 6. 显式上下文策略

### 规则文件

推荐支持以下规则文件，按优先级读取：

1. `AGENTS.md`
2. `NEOCODE.md`
3. `CLAUDE.md`

查找策略采用“当前工作目录向上查找”，并允许多级合并。越接近当前 `workdir` 的文件，优先级越高。

### 系统状态

建议注入以下高价值环境信息：

- 当前 `workdir`
- 当前 `shell`
- 当前 `provider`
- 当前 `model`
- git branch 与 dirty 状态

说明：

- 工具 schema 仍通过 `provider.ChatRequest.Tools` 传递
- `context` 只负责文本上下文，不接管工具定义

---

## 7. compact 与 memory 策略

### compact

`compact` 应先于长期 `memory` 落地，因为它解决的是长会话稳定性问题。

最小方案：

- 保留最近 N 轮消息
- 保持 `assistant tool_call -> tool result` 配对不被切断
- 将更早历史摘要为单条消息

推荐顺序：

1. 手动 `/compact`
2. 运行时摘要存储
3. 自动触发 `compact`

### memory

长期 `memory` 只承载无法从代码直接推导、且在后续任务中仍有价值的信息，例如：

- 用户偏好
- 项目决策
- 行为纠偏
- 长期有效的参考知识

第一版建议仅支持显式保存，例如 `/remember` 或 `/memory add`，不直接做自动抽取。

---

## 8. 配置与测试

建议新增配置：

```yaml
context:
  max_history_turns: 10
  rules_files:
    - AGENTS.md
    - NEOCODE.md
    - CLAUDE.md
  compact:
    enabled: false
  memory:
    enabled: false
```

测试重点：

- `context` 单测：规则文件发现、prompt 拼装顺序、历史裁剪边界、工具调用配对保留
- `runtime` 集成测试：正确调用 builder、builder 失败时的错误处理、压缩后仍可继续工具回合
- `config` 测试：默认值、阈值校验、重复配置处理

---

## 9. 近期开发建议

建议按以下顺序启动实现：

1. 新建 `internal/context`，迁移 `systemPrompt()` 与 `trimMessages()`，完成行为等价抽离。
2. 接入 `AGENTS.md` 向上查找与系统状态摘要注入。
3. 增加最小可用的手动 `/compact`。

---

## 10. 最终建议

NeoCode 当前阶段的正确路线应是：

> 先做 `Context Builder`，再做显式规则注入，然后做 `compact`，最后做长期 `memory`。

这样可以同时满足四个要求：

- 主链路稳定
- 模块边界清晰
- 变更易于测试
- 后续扩展有明确承载点
