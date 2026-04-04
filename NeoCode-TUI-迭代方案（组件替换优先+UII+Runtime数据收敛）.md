# NeoCode TUI 迭代方案（组件替换优先 + UI 分层 + Runtime 数据收敛）

## 1. 目标

- 先完成 Bubble 组件替换与交互统一，再推进既定功能迭代。
- 增加 UII 分层改造，明确 UI 各层职责并沉淀可复用组件。
- 新增 Runtime 数据收敛：`tool 状态`、`背景信息窗口`、`token 计数` 必须由 runtime 接口读取。
- 全部任务采用增量演进，不推倒重做。

## 2. 当前组件基线

已使用组件：

- `list`
- `textarea`
- `viewport`
- `spinner`
- `help`
- `key`

待引入组件：

- `filepicker`
- `progress`

待组件化区域：

- 命令建议菜单（当前为手写渲染）
- Activity 预览区（当前为固定条目）

## 3. UI 架构调整任务

### UI-1：定义分层与职责边界

层次定义：

- `L1 Shell`：程序生命周期、窗口尺寸、全局消息路由
- `L2 Screen Orchestrator`：页面编排、焦点流转、面板切换
- `L3 ViewModel State`：纯状态对象与状态转换
- `L4 Component Adapter`：bubbles 组件封装
- `L5 Renderer Theme`：样式、布局、断点、渲染规则
- `L6 Domain Bridge`：runtime/tools/security 到 UI 事件桥接

任务：

- 在 `internal/tui` 建立分层目录并迁移代码。
- 定义层间接口，禁止跨层直接访问。

### UI-2：统一 UI 消息协议

任务：

- 建立统一消息类型（例如 `UIEvent` + `Kind` + `Payload`）。
- 将 `RuntimeMsg`、本地命令消息、picker 消息统一映射到桥接层。

### UI-3：抽离可复用组件

组件清单：

- `CommandPalette`
- `ActivityPane`
- `InspectorTabs`
- `SessionListPane`
- `ComposerPane`
- `StatusHeader`

任务：

- 每个组件提供 `Model + Update + View + SetSize` 标准接口。
- 组件不直接依赖 runtime 实例。

### UI-4：主题与布局平台化

任务：

- 将样式映射整理为 token 体系。
- 将断点布局提取为纯函数（输入宽高，输出布局结构）。

### UI-5：回归守护

任务：

- 增加焦点流转、渲染顺序、滚动行为、输入流程测试。
- 为组件层补充最小渲染断言测试。

## 4. Runtime 数据接口收敛任务（新增）

### R1：扩展 RuntimeEvent 协议

目标：让 TUI 所需的状态都可由 runtime 事件流获得。

任务：

- 新增 `EventToolStatus`（tool 生命周期状态）。
- 新增 `EventRunContext`（背景信息快照）。
- 新增 `EventUsage`（token 使用统计，支持增量或最终值）。
- 为以上事件定义强类型 payload，避免 `any` 无约束扩散。

验收：

- TUI 无需本地猜测 tool/token/context，即可渲染完整状态。

### R2：扩展 Runtime 查询接口

目标：TUI 在重连、切会话、恢复时可补齐状态。

任务：

- 新增查询接口（示例）：
  - `GetRunSnapshot(runID)`
  - `GetSessionContext(sessionID)`
  - `GetSessionUsage(sessionID)` 或 `GetRunUsage(runID)`
- 明确接口返回结构与空值语义。

验收：

- 启动后仅依赖 runtime 查询 + runtime 事件即可恢复 UI 状态。

### R3：Runtime -> UII Bridge 映射

任务：

- 在 bridge 层统一映射：
  - `EventToolStatus -> ToolStateVM`
  - `EventRunContext -> ContextWindowVM`
  - `EventUsage -> TokenUsageVM`
- 为映射层补充单测（事件乱序、重复、缺失场景）。

验收：

- TUI 层不再直接拼接 runtime payload 字段。

### R4：数据源约束规则

规则：

- `Tool 状态` 仅能来自 runtime 事件/查询。
- `背景信息窗口` 仅能来自 runtime 事件/查询。
- `Token 计数` 仅能来自 runtime 事件/查询。

任务：

- 在代码评审与测试中加约束：禁止从 configManager 或 UI 本地临时变量推导这三类数据。

## 5. 组件替换优先任务

### C1：Command Menu 组件化（`list`）

任务：

- 用 `list.Model` 重做命令建议菜单。
- 统一 `/`、`@`、`&` 的建议呈现与交互。

### C2：Activity 组件化（`viewport`）

任务：

- Activity 改为可滚动面板。
- 将 `panelActivity` 纳入焦点循环。

### C3：`@file` 浏览模式（`filepicker`）

任务：

- 在文本联想之外新增文件浏览选择模式。
- 支持目录跳转、过滤、回填。

### C4：进度反馈增强（`spinner + progress`）

任务：

- 增加 `progress` 展示可量化阶段。
- 无进度时自动回退 spinner。

## 6. 既定功能迭代任务（组件替换后）

### F1：Run 隔离与事件去串线

任务：

- 发起 run 生成并绑定 `run_id`。
- TUI 仅消费当前 run 事件流。

### F2：Ask/Plan/Act 显式模式

任务：

- 新增 `/mode` 与状态栏 mode badge。
- 模式可查询、可持久化。

### F3：模式驱动权限策略

任务：

- Ask：禁止写工具与 `bash`
- Plan：允许读工具，阻断写工具
- Act：维持完整执行能力

### F4：三栏信息架构（Sessions / Transcript / Inspector）

任务：

- Inspector 支持 `Activity / Tool Output / Changes`。
- 保持三档响应式布局。

### F5：Changes 面板最小闭环

任务：

- 记录 `tool call -> files -> timestamp`。
- 在 Inspector 中回看最近变更。

### F6：键位迁移预设 + 命令面板

任务：

- 提供 `default / opencode / claude` 预设。
- 增加 `Ctrl+K` 命令面板入口。

### F7：Tool 状态面板（Runtime Source）

任务：

- 新增 Tool 状态面板（待执行/运行中/成功/失败/耗时）。
- 状态数据仅来自 runtime（`EventToolStatus` 与查询接口）。

### F8：背景信息窗口（Runtime Source）

任务：

- 新增 Background Info 窗口（session、workdir、provider、model、mode、权限摘要、run 元信息）。
- 数据仅来自 runtime（`EventRunContext` 与查询接口）。

### F9：Token 计数面板（Runtime Source）

任务：

- 新增 Token 计数区域（input/output/total，可含本轮与会话累计）。
- 数据仅来自 runtime（`EventUsage` 与查询接口）。

## 7. 统一完成标准（DoD）

- 测试通过：

```bash
go test ./internal/tui ./internal/runtime ./internal/tools ./internal/security ./internal/app
```

- 新增测试覆盖：
  - runtime 新事件与查询接口测试
  - bridge 映射测试
  - tool/context/token 三类 runtime-source 约束测试

- 文档同步：
  - `docs/tools-and-tui-integration.md`
  - `README.md`

- 每项任务可独立回滚，不引入跨任务强耦合发布依赖。

## 8. Issue 拆分建议

- `Epic: TUI Iteration (Component-first + UII + Runtime-source)`
- `UII-1` 到 `UII-5`
- `R1` 到 `R4`
- `C1` 到 `C4`
- `F1` 到 `F9`
