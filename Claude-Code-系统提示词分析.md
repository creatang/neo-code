#  Claude Code 系统提示词（System Prompt）与上下文架构深度剖析

## 0. 概述

本文档基于对 `claude-code-haha`（Claude Code 核心逻辑）的逆向工程分析，提炼了其在**系统提示词生成、优先级覆盖、动态组装与上下文解耦**方面的底层设计思想。

Claude Code 并没有将所有规则塞进一个巨大的系统提示词字符串，而是采用了一套**“有序区块数组（Ordered `string[]` sections） + 动态缓存边界（Dynamic Boundary） + 块转换（Block Conversion）”**的精细化分层管理架构。

------

## 一、 核心输入通道：多维度的上下文隔离

从提示词与上下文管理视角看，Claude Code 至少有四类核心注入通道（参考 `queryContext.ts`、`context.ts` 与 `attachments.ts`）：

### 1. `defaultSystemPrompt`（系统默认规则区块）

- **定位：** Claude Code 的基础行为准则。
- **形态：** 一个有序的字符串数组（`string[]`）。
- **内容：** `defaultSystemPrompt` 是默认提示词区块集合，既包含基于已启用工具（Enabled Tools）生成的工具使用规则，也包含角色介绍、系统规则、任务执行方式、行动边界、输出效率与风格约束，以及一组按 section 管理的动态增强区块（如 memory、language、output style、env info、MCP instructions 等）。**注意：** 真正的具体技能列表（Skill Listing）在很多场景下是通过后文的 Attachment 动态注入的，而不是硬编码在这里。

### 2. `userContext`（用户与项目侧事实）

- **定位：** 客观存在的项目环境信息（如项目目录下的 `CLAUDE.md` 规约、当前日期）。
- **注入方式：** 它并不属于 System Prompt 本体。在构建请求时，它会被预处理，并作为**构造好的上下文消息**，前置（Prepend）到整个请求消息流（`messagesForQuery`）的最前端。

### 3. `systemContext`（系统侧辅助事实）

- **定位：** 独立的系统级状态对象（如 `git status`、`cacheBreaker`）。
- **生命周期细节：** 当调用方传入了自定义提示词（`customSystemPrompt`）时，`getSystemContext()` 会在组装默认提示词的环节被跳过。但这并不意味着它被系统彻底丢弃，`systemContext` 依然是系统里的独立上下文对象，在后续某些流程中会继续传递。

### 4. `attachments` / `system-reminder`（轮次附件与动态提醒）

- **定位：** 伴随每一轮（Turn）对话动态注入的强关联上下文。
- **内容：** 例如 `skill_listing`（当前可用的技能列表）、`mcp_instructions_delta`、或者由于工具执行引发的内存状态提醒。

------

## 二、 提示词组装与决策链路

系统提示词相关逻辑可分为默认提示词生成、优先级仲裁、主干组装、API 发送前包装四类组件；它们彼此关联，但并不要求每次请求都完整经过同一条串行流水线：

### 1. 制造者：`getSystemPrompt()`

- 负责**生成默认的 Prompt Sections**。它关心的是默认底座由哪些有序区块组成。

### 2. 仲裁者：`buildEffectiveSystemPrompt()`

- 作为一个优先级仲裁工具，负责在多个提示词来源中做**选择**。它的仲裁树从高到低依次为：
  1. **override prompt：** 最高优先级，绝对接管。
  2. **coordinator prompt：** 协调者模式专用人格。
  3. **agent prompt：** 主线程 Agent 的专属提示词（替换或追加）。
  4. **custom system prompt：** 用户通过 CLI 传入的自定义规则。
  5. **default system prompt：** 上述皆无时，采用的默认底座。
- 在大多数非 override 情况下，它会在末尾追加 `appendSystemPrompt`（尾部临时补丁）。

### 3. 主干执行流：`QueryEngine.submitMessage()`

这是实际发起请求时的核心装配车间。它的主路径常常是直接基于 `fetchSystemPromptParts()` 做本地组装，其核心动作包括：

- 提取 `custom` 或 `default` 提示词。
- **关键补丁 1 (memoryMechanicsPrompt)：** 当传入了 `customSystemPrompt` 且启用了特定的 Memory Override 时，系统会在此处额外把内存使用说明补回去。
- 追加 `appendSystemPrompt`。

### 4. Provider 发送前包装（API Layer）

在数据真正发往 Anthropic API 之前（如 `claude.ts`），代码还会进行最后一次包装：为其包裹上 Provider 侧的前缀（例如 Attribution、CLI Prefix 或是某些厂商特定的额外指令）。这意味着“最终发给模型的 System Prompt”比系统内部流转的结构还要多一层外壳。

------

## 三、 缓存边界（Cache Boundary）与区块化

Claude Code 在 `src/constants/prompts.ts` 中明确定义了 `SYSTEM_PROMPT_DYNAMIC_BOUNDARY`。

- **物理分离：** 它将稳定的绝对规则（如核心人设、工具调用契约）放置在边界上方，将易变信息（如输出风格、当前环境变量、MCP 动态指令）放置在边界下方。
- **Block 转换：** 系统会将这些字符串 Section 转换为底层 API 认可的 Text Blocks。
- **核心价值：** 这种设计不仅是为了**减少 Prompt Cache 的失效（提升缓存命中率这一重要工程目标）**，同时也是为了保证核心行为的稳定性、实现角色的无缝切换，以及将动态指令进行物理隔离。

------

## 四、 核心架构设计结论

总结 Claude Code 在上下文与提示词管理上的核心范式：

1. Claude Code **不是**把所有规则塞进一个 System Prompt 字符串，而是把“默认 prompt sections、覆盖优先级、用户上下文、系统上下文、每轮附件提醒”进行分层管理。
2. `getSystemPrompt()` 负责生成默认 prompt sections；`buildEffectiveSystemPrompt()` 负责在 override / coordinator / agent / custom / default 之间做优先级选择。
3. `userContext` 与 `systemContext` 不等于 System Prompt 本体，它们是平行的上下文来源。
4. 真正发给大模型的输入，绝不仅限于 System Prompt，还包括 Prepend 到消息流前方的普通历史消息、系统构造的上下文消息，以及按 Turn 注入的 Attachments。
5. 引入动态缓存边界的核心价值，在于将稳定规则和易变信息进行物理分离，既保障了 Prompt Cache 的高命中率，又维持了系统行为的鲁棒性。