> **版本**: v1.0  
> **日期**: 2026-04-03  
> **Go 版本**: Go 1.25.0  
> **模块路径**: `neo-code`  

---

## 一、项目概览

NeoCode 是一个基于 **Go 语言** 构建的**终端 AI 编码助手 (Coding Agent MVP)**，采用 **Bubble Tea** (ELM 架构) TUI 框架实现终端交互界面。项目实现了 **ReAct (Reasoning + Acting) 循环** 模式：用户输入 -> LLM 推理 -> 工具调用 -> 结果观察 -> 再次推理，直至任务完成。

### 1.1 核心技术栈

| 类别 | 技术 | 用途 |
|------|------|------|
| UI 框架 | `github.com/charmbracelet/bubbletea` | ELM 架构 TUI |
| UI 组件 | `github.com/charmbracelet/bubbles` | list, textarea, viewport, spinner, help |
| 终端样式 | `github.com/charmbracelet/lipgloss` | 终端色彩与布局 |
| 配置格式 | `gopkg.in/yaml.v3` | YAML 配置解析 |

### 1.2 主链路闭环

```
用户输入 -> Agent 推理 (Runtime) -> 调用工具 (Tools) -> 获取结果 -> 继续推理 -> UI 展示 (TUI)
```

---

## 二、整体目录结构

```
neocode/
├── cmd/neocode/main.go                 # 主入口
├── internal/
│   ├── app/
│   │   └── bootstrap.go                # 应用引导 & 依赖组装 (Composition Root)
│   ├── config/
│   │   ├── model.go                    # 配置类型定义
│   │   ├── loader.go                   # 配置文件加载/保存 (YAML)
│   │   ├── manager.go                  # 配置管理器（线程安全）
│   │   ├── default_config.go           # 默认配置值
│   │   ├── builtin_providers.go        # 内置 Provider 定义
│   │   ├── provider_identity.go        # Provider 身份标识
│   │   └── *_test.go                   # 测试文件
│   ├── context/                        # Agent 上下文构建
│   │   ├── types.go                    # Builder 接口与 I/O 类型
│   │   ├── builder.go                  # DefaultBuilder 实现
│   │   ├── prompt.go                   # System Prompt 组装（6 大段）
│   │   ├── metadata.go                 # 运行时元数据定义
│   │   ├── source_rules.go            # AGENTS.md 规则加载
│   │   ├── source_system.go           # 系统 state 收集
│   │   └── trim.go                     # 消息裁剪
│   ├── provider/                       # Provider 抽象层
│   │   ├── provider.go                # V1 接口（仅 Chat）
│   │   ├── types.go                   # V1 类型体系
│   │   ├── v2_types.go                # V2 类型体系（预留）
│   │   ├── registry.go                # 驱动注册表
│   │   ├── errors.go                  # 领域错误类型
│   │   ├── metadata.go                # ModelDescriptor 辅助方法
│   │   ├── openai/openai.go           # OpenAI 协议驱动实现
│   │   ├── builtin/builtin.go         # 内置驱动注册入口
│   │   ├── catalog/                   # 模型目录缓存服务
│   │   │   ├── service.go             # 目录查询逻辑
│   │   │   └── store.go              # JSON 持久化
│   │   ├── selection/                 # Provider/Model 选择服务
│   │   │   └── service.go            # 选择逻辑 + 配置持久化
│   │   ├── discovery/                 # 模型发现
│   │   │   └── discovery.go          # /v1/models 端点调用
│   │   └── transport/                 # HTTP 传输层
│   │       └── retry.go              # 重试 Transport 包装器
│   ├── runtime/                         # Agent 运行时核心
│   │   ├── runtime.go                  # Service 核心 (ReAct Loop)
│   │   ├── events.go                   # 事件类型定义
│   │   ├── session.go                  # 会话持久化 (JSON)
│   │   ├── id.go                       # ID 生成器
│   │   └── *_test.go                   # 测试文件
│   ├── tools/                          # 工具系统
│   │   ├── types.go                   # Tool 接口与数据类型
│   │   ├── registry.go                 # 工具注册表
│   │   ├── manager.go                  # 工具管理器（权限+沙箱）
│   │   ├── permission_mapper.go       # 权限 Action 映射
│   │   ├── format.go                   # 格式化工具
│   │   ├── bash/tool.go               # Bash 命令执行工具
│   │   ├── filesystem/                 # 文件系统工具集
│   │   │   ├── read_file.go          # 读文件
│   │   │   ├── write.go              # 写文件
│   │   │   ├── edit.go               # 编辑文件
│   │   │   ├── grep.go               # 内容搜索
│   │   │   └── glob.go               # 文件名匹配
│   │   └── webfetch/                   # 网页抓取工具
│   │       ├── tool.go               # 工具主体
│   │       └── html.go               # HTML 内容提取
│   ├── security/                        # 安全子系统
│   │   ├── types.go                    # 安全类型（Action, Rule, Decision）
│   │   ├── gateway.go                  # 权限引擎（StaticGateway）
│   │   └── workspace.go               # 工作区沙箱
│   └── tui/                            # 终端用户界面
│       ├── app.go                      # App 结构体与构造函数
│       ├── state.go                    # UIState 与辅助类型
│       ├── update.go                   # Update() 消息分发核心
│       ├── view.go                     # View() 渲染布局
│       ├── commands.go                 # 斜杠命令定义与处理
│       ├── keymap.go                   # 键盘快捷键映射
│       ├── styles.go                   # lipgloss 样式定义
│       ├── input_features.go           # 输入特性检测（粘贴等）
│       ├── copy_code.go               # 代码块复制功能
│       ├── markdown_renderer.go        # Markdown 渲染器
│       ├── provider_service.go         # ProviderController 接口
│       └── *_test.go                   # 测试文件
├── go.mod / go.sum
├── agents.md                            # AI 协作规则
├── README.md
└── docs/
    └── *.md                             # 各类设计文档
```

---

## 三、各子模块详细说明

### 3.1 `internal/app` — 应用引导层 (Composition Root)

**职责**: 负责整个应用的依赖组装，创建所有核心组件并注入依赖关系。

**核心函数**: `NewProgram(ctx) (*tea.Program, error)` — 唯一入口函数

**启动流程**:

```
1. ensureConsoleUTF8()           -- Windows UTF-8 兼容
2. config.NewLoader()             -- 创建配置加载器 (~/.neocode/config.yaml)
3. config.NewManager(loader)      -- 创建配置管理器（线程安全）
4. manager.Load(ctx)              -- 加载配置（首次自动创建默认配置）
5. builtin.NewRegistry()          -- 创建并注册 OpenAI 驱动
6. catalog.NewService(...)        -- 模型目录缓存服务
7. selection.NewService(...)      -- Provider/Model 选择服务
8. selection.EnsureSelection()    -- 确保 Provider/Model 已选定
9. buildToolRegistry(cfg)         -- 注册 7 个工具
10. buildToolManager(registry)     -- 包装权限引擎 + 工作区沙箱
11. runtime.NewSessionStore()     -- 会话存储
12. runtime.NewWithFactory(...)   -- Runtime Service (ReAct 引擎)
13. tui.New(...)                   -- TUI App
14. tea.NewProgram(app, ...)       -- Bubble Tea Program
```

**关键设计点**: 所有依赖在单一入口点显式组装，无 DI 容器框架，纯手动注入。这使得依赖关系完全透明、易于测试。

---

### 3.2 `internal/config` — 配置管理

**职责**: 配置的加载、验证、默认值填充、持久化。

#### 3.2.1 核心类型

| 类型 | 关键字段 | 说明 |
|------|----------|------|
| `Config` | `Providers[]`, `SelectedProvider`, `CurrentModel`, `Workdir`, `Shell`, `MaxLoops`, `ToolTimeoutSec`, `Tools` | 应用全量配置 |
| `ProviderConfig` | `Name`, `Driver`, `BaseURL`, `Model`, `APIKeyEnv` | 单个 Provider 定义 |
| `ResolvedProviderConfig` | 继承 `ProviderConfig` + `APIKey` | 解析后（含 API Key） |
| `ToolsConfig` | `WebFetch(WebFetchConfig)` | 工具级配置 |
| `Loader` | `baseDir`, `defaults` | 配置文件读写（YAML），默认路径: `~/.neocode/config.yaml` |
| `Manager` | `mu sync.RWMutex`, `loader`, `config *Config` | 线程安全的配置管理器 |

#### 3.2.2 Manager 方法

| 方法 | 说明 |
|------|------|
| `Load/Reload(ctx)` | 从磁盘加载 |
| `Get()` | 读取当前快照（线程安全深拷贝） |
| `Save(ctx)` | 持久化到磁盘 |
| `Update(ctx, mutate)` | 事务性修改: 加锁 -> mutate -> Validate -> Save -> 更新内存 |
| `SelectedProvider()` / `ResolvedSelectedProvider()` | 快捷访问 |

#### 3.2.3 设计要点

- 使用 `persistedConfig` 中间结构做序列化（**不包含 Providers 列表**，Providers 由内置代码固定）
- 配置变更时自动回写规范化格式
- 内置 **4 个 Provider**: OpenAI (`gpt-4.1`), Gemini (`gemini-2.5-flash`), OpenLL (`gpt-5.4`), Qiniu (`openai/gpt-5`)
- **API Key 通过环境变量注入，绝不落盘存储**
- 默认值: MaxLoops=8, ToolTimeoutSec=20, WebFetch MaxBytes=256KB

---

### 3.3 `internal/provider` — Provider 抽象层

**职责**: 定义 LLM Provider 的统一接口和类型系统，支持多协议驱动扩展。

#### 3.3.1 V1 接口

```go
type Provider interface {
    Chat(ctx context.Context, req ChatRequest, events chan<- StreamEvent) (ChatResponse, error)
}
```

#### 3.3.2 V1 核心类型

| 类型 | 说明 |
|------|------|
| `Message` | 聊天消息 {Role, Content, ToolCalls[], ToolCallID, IsError} |
| `ToolCall` | 工具调用 {ID, Name, Arguments(JSON)} |
| `ToolSpec` | 工具描述 {Name, Description, Schema} |
| `ChatRequest` | 请求 {Model, SystemPrompt, Messages[], Tools[]} |
| `ChatResponse` | 响应 {Message, FinishReason, Usage} |
| `StreamEvent` | 流式事件: TextDelta / ToolCallStart / ToolCallDelta / MessageDone |
| `ModelDescriptor` | 模型描述符 {ID, Name, ContextWindow...} |
| `ProviderCatalogItem` | Provider 目录项 {ID, Name, Models[]} |
| `ProviderSelection` | 选择结果 {ProviderID, ModelID} |

#### 3.3.3 V2 类型系统 (v2_types.go — 预留)

```go
type ProviderV2 interface {
    CompleteTurn(ctx context.Context, req TurnRequest, events chan<- StreamEventV2) (TurnResponse, error)
}
```

V2 引入了更丰富的类型:
- `TurnItem` + `ContentPart` (text/reasoning_summary/tool_call/tool_result/artifact)
- `ArtifactRef` (file/image/url)
- `StopReason` (10 种细粒度停止原因)
- `CapabilitySet` (9 种能力标记: tool_call, parallel_tool_calls, reasoning, vision...)
- `StreamEventV2` (7 种细粒度流式事件)
- `ResponseHandle` 支持 continuation

#### 3.3.4 Registry — 驱动注册表

```go
type Builder func(ctx context.Context, cfg config.ResolvedProviderConfig) (Provider, error)
type DiscoveryFunc func(ctx context.Context, cfg config.ResolvedProviderConfig) ([]ModelDescriptor, error)

type DriverDefinition struct {
    Name string; Build Builder; Discover DiscoveryFunc
}

type Registry struct { drivers map[string]DriverDefinition }
// Register(driver) -- 注册驱动
// Build(ctx, cfg) -- 根据配置构建 Provider 实例
// DiscoverModels(ctx, cfg) -- 发现可用模型列表
```

**好处**: 新增协议只需写一个新的 `DriverDefinition` 并 `Register`，零修改核心代码。

#### 3.3.5 错误体系

```go
type ProviderError struct {
    StatusCode int               // HTTP 状态码
    Code       ProviderErrorCode // auth_failed/forbidden/not_found/rate_limited/server_error/timeout/network_error/unknown
    Message    string
    Retryable  bool              // 429 和 5xx 自动标记为可重试
}
```

#### 3.3.6 OpenAI 驱动

- 实现完整的 SSE 流式消费 (`consumeStream`)
- 发送 4 种流式事件: TextDelta, ToolCallStart, ToolCallDelta, MessageDone
- 内置 `RetryTransport`（HTTP 层重试）
- 支持 `DiscoverModels` 通过 `/v1/models` 端点

---

### 3.4 `internal/runtime` — Agent 运行时 (ReAct Loop 核心)

**职责**: 实现完整的 ReAct 循环——用户输入 -> 构建 Context -> 调用 LLM -> 处理工具调用 -> 循环/结束。

#### 3.4.1 Runtime 接口

```go
type Runtime interface {
    Run(ctx context.Context, input UserInput) error
    CancelActiveRun() bool
    Events() <-chan RuntimeEvent
    ListSessions(ctx context.Context) ([]SessionSummary, error)
    LoadSession(ctx context.Context, id string) (Session, error)
    SetSessionWorkdir(ctx context.Context, sessionID string, workdir string) (Session, error)
}
```

#### 3.4.2 Service 结构体字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `configManager` | `*config.Manager` | 配置读取 |
| `sessionStore` | `Store` | 会话 CRUD |
| `toolManager` | `tools.Manager` | 工具执行 |
| `providerFactory` | `ProviderFactory` | Provider 创建工厂 |
| `contextBuilder` | `agentcontext.Builder` | System Prompt 构建 |
| `events` | `chan RuntimeEvent` (缓冲128) | 事件广播通道 |
| `runMu` | `sync.Mutex` | 运行互斥锁（保证单次 Run） |
| `activeRunToken` | `uint64` | 当前运行令牌（过期事件过滤） |
| `nextRunToken` | `uint64` | 下一个运行令牌的递增计数器（用于区分不同 Run 的生命周期） |
| `activeRunCancel` | `context.CancelFunc` | 当前活跃 Run 的取消函数 |

#### 3.4.3 ReAct 核心流程 (Run())

```
用户输入 UserInput
  │
  ├─ 1. loadOrCreateSession() -- 加载或新建会话
  ├─ 2. 追加 user Message → 保存会话 → emit EventUserMessage
  │
  ├─ FOR attempt = 0; attempt < MaxLoops(8); attempt++
  │     │
  │     ├─ 3. contextBuilder.Build() -- 构建 SystemPrompt + Messages + 规则 + Git State
  │     ├─ 4. toolManager.ListAvailableSpecs() -- 获取工具 Schema 列表
  │     │
  │     ├─ 5. callProviderWithRetry() -- 调用 LLM（最多重试 2 次，指数退避+抖动）
  │     │     ├─→ 创建 Provider 实例
  │     │     ├─→ 启动 goroutine 转发流式事件 (forwardProviderEvents)
  │     │     ├─→ provider.Chat(req, streamEvents) -- SSE 流式调用
  │     │     └─← ChatResponse
  │     │
  │     ├─ 6. 追加 assistant Message → 保存会话
  │     │
  │     ├─ 7. IF 无 ToolCalls → emit EventAgentDone → RETURN（正常完成）
  │     │
  │     └─ 8. FOR EACH ToolCall:
  │           ├─ emit EventToolStart
  │           ├─ tools.Manager.Execute()
  │           │   ├─ [第1层] PermissionEngine.Check() -- 权限决策
  │           │   ├─ [第2层] WorkspaceSandbox.Check() -- 路径沙箱
  │           │   └─ [第3层] Registry.Execute() -- 实际执行
  │           ├─ 构造 tool Message → 追加到 Session → 保存
  │           └─ emit EventToolResult
  │
  └─ 达到 MaxLoops → emit EventError → RETURN ERROR
```

#### 3.4.4 事件类型

| EventType | Payload | 说明 |
|-----------|---------|------|
| `user_message` | Message | 用户输入已接受 |
| `agent_chunk` | string | 助手文本增量（流式） |
| `agent_done` | Message | 助手完成 |
| `tool_start` | ToolCall | 工具开始执行 |
| `tool_result` | ToolResult | 工具执行完成 |
| `tool_chunk` | string | 工具输出增量 |
| `run_canceled` | nil | 运行被取消 |
| `error` | string | 错误信息 |
| `tool_call_thinking` | string | 模型决定调工具（过渡提示） |
| `provider_retry` | string | 正在重试 Provider 调用 |

#### 3.4.5 Session 持久化

- 存储路径: `~/.neocode/sessions/{session_id}.json`
- JSON 格式，**原子写入**（先写 `.tmp` 再 `rename`）
- `Session`: `{ID, Title, CreatedAt, UpdatedAt, Workdir, Messages[]}`
- 支持按 UpdatedAt 降序列出摘要

---

### 3.5 `internal/context` — 上下文构建

**职责**: 为每一轮 LLM 调用构建 System Prompt 和消息上下文。

#### 3.5.1 Builder 接口

```go
type Builder interface {
    Build(ctx context.Context, input BuildInput) (BuildResult, error)
}
```

#### 3.5.2 DefaultBuilder.Build() 流程

```
1. loadProjectRules(workdir)     -- 从工作区向上查找 AGENTS.md 文件
   └→ 最多加载 12000 runes 总量，单个文件 4000 runes 上限
2. collectSystemState(metadata)  -- 收集 Git 信息（branch, dirty状态）
3. 组装 System Prompt Sections:
   ├─ ## Agent Identity      -- "You are NeoCode..."
   ├─ ## Tool Usage          -- 工具使用指导
   ├─ ## Workspace Safety    -- 安全约束
   ├─ ## Code Changes        -- 代码变更规范
   ├─ ## Failure Recovery    -- 错误恢复策略
   ├─ ## Response Style      -- 回复风格
   ├─ ## Project Rules       -- AGENTS.md 内容（如果有）
   └─ ## System State        -- Workdir/Shell/Provider/Model/Git Branch/Git Dirty
4. trimMessages(messages)    -- 裁剪到最近 10 条消息（maxContextTurns=10）
5. 返回 {SystemPrompt, Messages}
```

---

### 3.6 `internal/tools` — 工具系统

**职责**: 定义工具接口、注册表、执行管理层和安全包装。

#### 3.6.1 Tool 接口

```go
type Tool interface {
    Name() string
    Description() string
    Schema() map[string]any          // JSON Schema
    Execute(ctx context.Context, call ToolCallInput) (ToolResult, error)
}
```

#### 3.6.2 三层执行管道

```
Execute(input)
  │
  ├─ [第1层] PermissionEngine.Check(action)  -- 权限决策 (allow/deny/ask)
  │     └─ StaticGateway: 有序规则列表匹配，默认 DecisionAllow
  │
  ├─ [通过] WorkspaceSandbox.Check(action)  -- 工作区边界校验
  │     ├─ 路径逃逸检测 (filepath.Rel "..")
  │     ├─ 符号链接逃逸检测 (EvalSymlinks)
  │     └─ Windows 卷名校验 (防止跨盘符访问)
  │
  └─ [通过] Executor.Execute(input)        -- 实际执行 (Registry)
        └─ Registry: 按 name (小写索引) 查找并执行
```

#### 3.6.3 已注册的 7 个工具

| 工具名 | 包位置 | 功能 |
|--------|--------|------|
| `read_file` | `tools/filesystem/read_file.go` | 读取文件内容 |
| `write` | `tools/filesystem/write.go` | 写入/创建文件 |
| `edit` | `tools/filesystem/edit.go` | 编辑文件（搜索替换） |
| `grep` | `tools/filesystem/grep.go` | 正则内容搜索 |
| `glob` | `tools/filesystem/glob.go` | 文件名模式匹配 |
| `bash` | `tools/bash/tool.go` | Shell 命令执行（支持 bash/powershell/sh） |
| `webfetch` | `tools/webfetch/` | 网页内容抓取（限制 MIME 类型和大小） |

---

### 3.7 `internal/security` — 安全子系统

**职责**: 提供权限控制和工作区沙箱两层安全机制。

#### 3.7.1 权限引擎 (StaticGateway)

- **规则匹配**: Type 精确匹配 + Resource 忽略大小写 + TargetPrefix 前缀匹配
- **返回第一个命中规则的 Decision**，无匹配则返回 defaultDecision
- 当前使用 **DecisionAllow 全放行**（生产环境可接入更严格的策略）
- **三种 Decision**: `Allow` / `Deny` / `Ask`

#### 3.7.2 工作区沙箱 (WorkspaceSandbox)

- 验证目标路径在 **Workspace Root** 内部（使用 `filepath.Rel` 判断 `..` 逃逸）
- 检测**符号链接逃逸**（解析最近存在的祖先路径上的 symlink）
- **Windows 卷名校验**（防止跨盘符访问如 `D:\` 从 `C:\` workspace）
- 缓存规范化的 workspace root 路径（`sync.Map`）

#### 3.7.3 Action 类型体系

```go
type Action struct {
    Type    ActionType   // "bash" | "read" | "write" | "mcp"
    Payload ActionPayload // {ToolName, Resource, Operation, SessionID, Workdir, Target...}
}
```

---

### 3.8 `internal/tui` — 终端用户界面

**职责**: 基于 Bubble Tea 的 TUI 应用，处理用户交互、渲染界面、消费 Runtime 事件。

#### 3.8.1 App 核心组件

| 组件 | 类型 | 用途 |
|------|------|------|
| `state` | `UIState` | 全部 UI 状态 |
| `configManager` | `*config.Manager` | 配置管理器引用 |
| `providerSvc` | `ProviderController` | Provider 选择服务接口 |
| `runtime` | `Runtime` (接口) | Agent 运行时接口引用 |
| `sessions` | `list.Model` | 会话列表面板 |
| `providerPicker` | `list.Model` | Provider 选择器 |
| `modelPicker` | `list.Model` | Model 选择器 |
| `transcript` | `viewport.Model` | 对话记录视图 |
| `input` | `textarea.Model` | 输入框 |
| `markdownRenderer` | `markdownContentRenderer` | Markdown 渲染器 |
| `spinner` | `spinner.Model` | 加载动画 |
| `help` | `help.Model` | 帮助面板 |
| `keys` | `keyMap` | 键盘快捷键绑定定义 |
| `codeCopyBlocks` | `map[int]string` | 代码块复制映射（行号→内容） |
| `pendingCopyID` | `int` | 待复制的代码块 ID |
| `activeMessages` | `[]provider.Message` | 当前活跃消息列表（用于 transcript 渲染） |
| `activities` | `[]activityEntry` | 活动日志条目列表 |
| `fileCandidates` | `[]string` | 文件补全候选列表 |
| `modelRefreshID` | `string` | 模型目录刷新标识（关联当前 Provider） |
| `focus` | `panel` | 当前焦点面板 |
| `width/height` | `int` | 终端窗口尺寸 |
| `styles` | `styles` | lipgloss 样式集合 |
| `nowFn` | `func() time.Time` | 时间函数注入（用于可测试性） |
| `lastInputEditAt` | `time.Time` | 上次输入编辑时间戳（粘贴检测用） |
| `lastPasteLikeAt` | `time.Time` | 上次粘贴事件时间戳（粘贴检测用） |
| `inputBurstStart` | `time.Time` | 输入突发开始时间 |
| `inputBurstCount` | `int` | 输入突发计数 |
| `pasteMode` | `bool` | 粘贴模式标记 |

#### 3.8.2 四种面板 (panel)

| 面板 | 常量 | 用途 |
|------|------|------|
| 会话面板 | `panelSessions` | 左侧会话列表 |
| 对话面板 | `panelTranscript` | 中间对话记录 |
| 活动面板 | `panelActivity` | 活动日志 |
| 输入面板 | `panelInput` | 底部输入框 |

#### 3.8.3 布局算法

- **宽度 >= 110**: 左右分栏模式（sidebar 22px + body gap 1px）
- **宽度 < 110**: 上下堆叠模式
- 最小窗口尺寸: **84x24**

#### 3.8.4 消息分发 (Update)

```
tea.Msg 分发:
  ├── WindowSizeMsg          → resizeComponents()
  ├── RuntimeMsg             → handleRuntimeEvent() → rebuildTranscript()
  ├── RuntimeClosedMsg       → IsAgentRunning = false
  ├── runFinishedMsg         → 处理运行结果/错误
  ├── modelCatalogRefreshMsg → 刷新模型选择器
  ├── localCommandResultMsg  → 处理斜杠命令结果
  ├── MouseMsg               → handleTranscriptMouse / handleInputMouse
  └── KeyMsg                 →
      ├── Ctrl+U/Quit        → tea.Quit (退出应用)
      ├── Ctrl+Q             → ToggleHelp (显示帮助)
      ├── Ctrl+W             → CancelActiveRun() (取消运行中任务)
      ├── Esc                → FocusInput (聚焦输入框)
      ├── Enter              → 发送输入 / 执行命令 / 打开会话
      ├── Ctrl+J             → 输入框换行
      ├── Tab/Shift+Tab      → 面板切换 (NextPanel/PrevPanel)
      ├── Ctrl+N             → 新建会话
      ├── Up/K / Down/J      → 滚动列表或视图
      ├── PgUp/B / PgDn/F    → 翻页
      ├── G/Home             → 滚动到顶部
      ├── Shift+G/End        → 滚动到底部
      ├── /help /exit ...    → 斜杠命令
      ├── /provider          → 打开 Provider 选择器
      └── /model             → 打开 Model 选择器
```

#### 3.8.5 自定义消息类型

| 消息类型 | 用途 |
|----------|------|
| `RuntimeMsg` | Runtime 事件桥接 |
| `RuntimeClosedMsg` | Runtime 通道关闭通知 |
| `runFinishedMsg` | Agent 运行完成（含 error） |
| `modelCatalogRefreshMsg` | 模型目录刷新完成 |
| `localCommandResultMsg` | 本地命令执行结果 |
| `sessionWorkdirResultMsg` | 工作目录切换结果 |

#### 3.8.6 斜杠命令

| 命令 | 功能 |
|------|------|
| `/help` | 显示帮助 |
| `/exit` | 退出应用 |
| `/clear` | 清空草稿 |
| `/status` | 显示状态摘要 |
| `/provider` | 打开 Provider 选择器 |
| `/model` | 打开 Model 选择器 |
| `/cwd [path]` | 查看/切换工作目录 |

#### 3.8.7 ProviderController 接口

```go
type ProviderController interface {
    ListProviders(ctx) ([]ProviderCatalogItem, error)
    SelectProvider(ctx, name) (ProviderSelection, error)
    ListModels(ctx) ([]ModelDescriptor, error)
    ListModelsSnapshot(ctx) ([]ModelDescriptor, error)
    SetCurrentModel(ctx, modelID) (ProviderSelection, error)
}
```

由 `selection.Service` 实现。

---

## 四、模块间依赖关系图

```
┌──────────────────────────────────────────────────────────────────────┐
│                           cmd/neocode/main.go                         │
│                                  │                                    │
│                                  ▼                                   │
│                      ┌─────────────────────┐                          │
│                      │   internal/app      │  ← Composition Root     │
│                      │    bootstrap.go     │  (依赖组装中心)           │
│                      └────────┬────────────┘                          │
├───────────────────────────────┼───────────────────────────────────────┤
│                               │                                       │
│          ┌────────────────────┼────────────────────┐                 │
│          ▼                    ▼                     ▼                 │
│  ┌────────────────┐  ┌────────────────┐  ┌──────────────────┐        │
│  │  config.Manager│  │ provider.Registry│  │  tools.Registry   │        │
│  │  (Loader)      │◄─┤  (builtin.OpenAI)││  (7 个 Tool)      │        │
│  └───────┬────────┘  └───────┬────────┘  └────────┬─────────┘        │
│          │                   │                     │                  │
│          ▼                   ▼                     ▼                  │
│  ┌──────────────┐  ┌─────────────────┐  ┌────────────────────┐       │
│  │catalog.Service│  │selection.Service│  │  tools.Manager     │       │
│  │ (模型缓存)    │  │ (Provider/Model)│  │ (Executor + Engine │       │
│  └──────┬────────┘  └──────┬────────┘  │  + Sandbox)        │       │
│         │                    │           └───────┬────────────┘       │
│         └────────┬───────────┘                   │                    │
│                  ▼                               ▼                    │
│         ┌──────────────────────────────────────────────────┐         │
│         │              runtime.Service (ReAct Loop)         │         │
│         │  ┌─────────────┐  ┌──────────┐  ┌────────────┐   │         │
│         │  │context.Builder│  │sessionStore│  │ events chan│   │         │
│         │  └─────────────┘  └──────────┘  └────────────┘   │         │
│         └───────────────────────┬──────────────────────────┘         │
│                                 │                                     │
│                                 ▼                                     │
│                  ┌────────────────────────┐                          │
│                  │    tui.App (BubbleTea)  │  ← View / Update        │
│                  │  ┌──────────────────┐   │                          │
│                  │  │ ProviderController│   │                          │
│                  │  └──────────────────┘   │                          │
│                  └────────────────────────┘                          │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────┐     │
│  │                     security 子系统 (横切关注点)                │     │
│  │  ┌────────────────┐    ┌─────────────────┐                    │     │
│  │  │ StaticGateway  │◄───│  tools.Manager   │                    │     │
│  │  │ (权限决策)     │    │  (第1层: 权限)   │                    │     │
│  │  └────────────────┘    ├─────────────────┤                    │     │
│  │                         │ WorkspaceSandbox │                    │     │
│  │                         │  (第2层: 路径沙箱)│                    │     │
│  │                         └─────────────────┘                    │     │
│  └──────────────────────────────────────────────────────────────┘     │
└──────────────────────────────────────────────────────────────────────┘
```

### 依赖方向总结

| 模块 | 依赖 |
|------|------|
| **app** | 依赖所有其他内部包（作为组装根） |
| **runtime** | config, provider, tools, context, security（间接） |
| **tui** | config, provider, runtime (Runtime 接口), tools |
| **tools/manager** | security, provider (ToolSpec 类型) |
| **tools/bash**, **tools/filesystem**, **tools/webfetch** | tools (接口) |
| **provider/openai** | provider (接口/类型), config, transport |
| **provider/catalog** | provider, config |
| **provider/selection** | config, provider, catalog |
| **context** | 仅依赖 provider (Message 类型) |
| **security** 无内部依赖（纯领域包） |

**关键约束**:
- 不跨层直连；新功能默认沿 `TUI -> Runtime -> Provider / Tool Manager` 设计
- 不把模型厂商差异泄漏到 runtime、tui 或上层调用方
- 不在 runtime 或 tui 里直接写工具执行逻辑；所有可被模型调用的能力进入 internal/tools

---

## 五、核心数据流

### 5.1 完整数据流

```
用户键盘输入
    │
    ▼
TUI App.Update(KeyMsg)
    │
    ├─ 斜杠命令 (/help, /exit, /provider, /model, /cwd, ...)
    │   └─ localCommandResultMsg (同步/异步)
    │       ├─ /provider → selection.SelectProvider() → config.Manager.Update()
    │       └─ /model    → selection.SetCurrentModel() → config.Manager.Update()
    │
    └─ 普通文本 → runAgent() [tea.Cmd, 异步 goroutine]
         │
         ▼
    runtime.Run(UserInput)
         │
         ├─ [EventUserMessage] ─────────► TUI: 显示用户消息
         │
         ├─ contextBuilder.Build() → SystemPrompt + Messages
         ├─ toolManager.ListAvailableSpecs() → ToolSpec[]
         │
         ├─ provider.Chat(request, streamEvents)
         │   │
         │   ├─ [TextDelta] ──→ forwardProviderEvents → [EventAgentChunk] → TUI: 流式显示
         │   ├─ [ToolCallStart] ─→ forwardProviderEvents → [EventToolCallThinking]
         │   └─ [MessageDone] → ChatResponse
         │
         ├─ [IF 有 ToolCalls]
         │   │
         │   ├─ FOR EACH call:
         │   │   ├─ [EventToolStart] → TUI
         │   │   ├─ tools.Manager.Execute()
         │   │   │   ├─ Security Check (Allow/Deny)
         │   │   │   ├─ Sandbox Check (路径边界)
         │   │   │   └─ Tool.Execute() → ToolResult
         │   │   ├─ [EventChunk] → TUI (如果工具支持流式)
         │   │   └─ [EventToolResult] → TUI: 追加 tool 消息
         │   │
         │   └─ LOOP BACK (带着工具结果再次调用 LLM)
         │
         └─ [IF 无 ToolCalls]
             └─ [EventAgentDone] → TUI: 完成
```

### 5.2 事件流转细节

```
Provider StreamEvent          Runtime Event              TUI 反应
─────────────────────         ──────────────             ──────────
TextDelta            ─────►   EventAgentChunk      ─────►  追加到 assistant 消息
ToolCallStart        ─────►   EventToolCallThinking ──►  Activity: "Planning..."
ToolCallDelta        ─────►   (聚合，不在 RT 层暴露)   
MessageDone          ─────►   (触发 response 处理)
```

---

## 六、关键设计模式

### 6.1 ReAct Loop (推理-行动循环)

**位置**: `internal/runtime/runtime.go` → `Service.Run()`

这是系统的核心模式。LLM 在每个回合中：
1. **Reasoning (推理)**: 基于 System Prompt + 历史消息 + 工具 Schema 生成思考和决策
2. **Acting (行动)**: 如果需要获取更多信息，发起 Tool Call
3. **Observation (观察)**: 工具执行结果被追加为 tool role 消息
4. **循环**: 带着观察结果回到步骤 1

最大循环次数由 `config.MaxLoops` 控制（默认 **8**），防止无限循环。

### 6.2 Provider 模式 (驱动注册 + 工厂方法)

**位置**: `internal/provider/registry.go`

采用三件套:
- `DriverDefinition`: 定义驱动的 Name、Build 函数、Discover 函数
- `Registry`: 维护 driver 映射表 (name → DriverDefinition)
- `Builder`: 类型签名为 `func(ctx, cfg) → (Provider, error)`

新增协议只需写一个新的 `DriverDefinition` 并 `Register`，零修改核心代码。

### 6.3 Composition Root (组合根)

**位置**: `internal/app/bootstrap.go` → `NewProgram()`

所有依赖在单一入口点显式组装，没有 DI 容器框架，纯手动注入。依赖关系完全透明、易于测试。

### 6.4 事件驱动架构 (Channel-based Event Bus)

**位置**: `internal/runtime/runtime.go` → `events chan RuntimeEvent`

Runtime 通过带缓冲 channel (cap=**128**) 向 TUI 层推送事件:
- **非阻塞发送优先**（select + default）
- TUI 通过 Bubble Tea 的 `ListenForRuntimeEvent` Cmd 订阅
- 事件携带 **RunID** 用于过滤过期事件
- Provider 流式事件通过独立 goroutine + channel 转发

### 6.5 三层安全管道

**位置**: `internal/tools/manager.go` → `DefaultManager.Execute()`

```
请求 → [权限网关] → [工作区沙箱] → [实际执行]
              ↓              ↓
         Allow/Deny/Ask   路径逃逸检测
                         符号链接逃逸检测
                         跨卷访问阻止
```

### 6.6 Bubble Tea ELM 架构

**位置**: `internal/tui/`

经典的 Elm 架构三个核心方法:

| 方法 | 职责 |
|------|------|
| `Init()` | 初始命令（订阅 Runtime 事件 + 光标闪烁 + Spinner） |
| `Update(msg) (App, Cmd)` | 纯函数消息处理，返回新状态和副作用命令 |
| `View() string` | 状态到 UI 的纯函数渲染 |

### 6.7 Strategy 模式 (Shell 执行)

**位置**: `internal/tools/bash/tool.go` → `shellArgs()`

根据 shell 配置动态选择执行方式:

| shell | 命令 |
|-------|------|
| `powershell`/`pwsh` | `powershell -NoProfile -Command` |
| `bash` | `bash -lc` |
| `sh` | `sh -lc` |
| Windows 默认 | powershell |
| 其他默认 | sh |

---

## 七、配置管理总结

| 方面 | 详情 |
|------|------|
| **存储路径** | `~/.neocode/config.yaml` |
| **序列化格式** | YAML (via `gopkg.in/yaml.v3`) |
| **持久化字段** | selected_provider, current_model, shell, max_loops, tool_timeout_sec, tools.webfetch.* |
| **非持久化** | Providers 列表（硬编码）、workdir（运行时计算） |
| **线程安全** | Manager 使用 `sync.RWMutex` 保护，Get() 返回深拷贝 |
| **默认值** | MaxLoops=8, ToolTimeoutSec=20, WebFetch MaxBytes=256KB |
| **内置 Provider** | OpenAI(gpt-4.1), Gemini(gemini-2.5-flash), OpenLL(gpt-5.4), Qiniu(openai/gpt-5) |
| **API Key** | 通过环境变量注入，不落盘存储 |

---

## 八、工具系统设计总结

| 特性 | 实现 |
|------|------|
| **接口定义** | `Tool` 接口: Name/Description/Schema/Execute |
| **注册机制** | `Registry` 按 name (小写) 索引，支持 Get/Supports/GetSpecs |
| **Schema 格式** | JSON Schema (map[string]any)，传给 OpenAI function calling |
| **执行管道** | Manager → 权限检查 → 沙箱检查 → Registry 执行 |
| **流式输出** | `ToolCallInput.EmitChunk` 回调 |
| **输出限制** | ApplyOutputLimit 截断过大输出 |
| **超时控制** | 每个 Tool Call 有独立 timeout (继承自 cfg.ToolTimeoutSec) |
| **错误格式化** | FormatError / NormalizeErrorReason 统一错误呈现 |

---

## 九、V2 类型体系展望

项目已在 `v2_types.go` 中预留了 V2 接口，主要升级点:

| V1 | V2 | 说明 |
|----|----|------|
| `Provider.Chat()` | `ProviderV2.CompleteTurn()` | 单次完整回合 |
| `Message` (扁平) | `TurnItem[]` + `ContentPart[]` (结构化) | 多模态支持 |
| 4 种 StreamEvent | 7 种 StreamEventV2 | 细粒度事件 |
| 无停止原因枚举 | 10 种 StopReason | 精确停止语义 |
| 无能力标记 | CapabilitySet (9 种能力) | 能力协商 |
| 无 continuation | ResponseHandle | 对话续接 |
| 无 artifact | ArtifactRef (file/image/url) | 结构化产物引用 |

---

## 十、安全机制总览

### 10.1 多层防御

```
┌──────────────────────────────────────────────────────┐
│                    安全防御层次                         │
│                                                        │
│  第1层: 配置层                                          │
│  ├─ API Key 仅通过环境变量注入，永不落盘                    │
│  ├─ workdir 必须为绝对路径                                │
│  └─ 敏感配置启动阶段校验                                   │
│                                                        │
│  第2层: 权限引擎 (StaticGateway)                          │
│  ├─ 有序规则列表匹配                                      │
│  ├─ Type/Resource/TargetPrefix 三维过滤                  │
│  └─ Allow/Deny/Ask 三态决策                              │
│                                                        │
│  第3层: 工作区沙箱 (WorkspaceSandbox)                    │
│  ├─ filepath.Rel ".." 路径逃逸检测                      │
│  ├─ EvalSymlinks 符号链接逃逸检测                         │
│  ├─ Windows 卷名跨盘访问阻止                              │
│  └─ canonicalRoot 缓存 (sync.Map)                       │
│                                                        │
│  第4层: 工具级限制                                        │
│  ├─ bash: 超时控制 + 输出截断                              │
│  ├─ filesystem: 工作区边界二次校验                          │
│  └─ webfetch: MIME 白名单 + 响应大小限制                    │
│                                                        │
│  第5层: 运行时保护                                        │
│  ├─ MaxLoops 上限 (默认 8)                                │
│  ├─ context 取消传播                                     │
│  └─ runMu 互斥锁 (单次运行)                               │
└──────────────────────────────────────────────────────┘
```

---

## 十一、测试策略

### 11.1 测试覆盖重点

| 模块 | 重点覆盖 |
|------|----------|
| **config** | 配置校验、默认值填充、序列化/反序列化、Provider 身份去重 |
| **provider** | 请求/响应转换、tool call 解析、异常响应处理、认证/限流错误映射 |
| **runtime** | 最大轮数停止、tool result 回灌、最终响应输出、错误事件派发 |
| **tools** | Schema 校验、超时控制、错误包装、工作目录限制、输出截断 |
| **security** | 规则匹配逻辑、路径边界检测、符号链接逃逸、卷名跨盘 |
| **tui** | 消息分发、快捷键路由、状态机转换、粘贴检测、命令补全 |

### 11.2 测试文件清单

项目已包含丰富的测试文件，包括但不限于:
- `config/*_test.go` — 配置全链路测试 (~30KB)
- `provider/*_test.go` — 驱动注册、OpenAI 协议、错误分类、元数据辅助
- `runtime/runtime_test.go` — ReAct loop 完整流程 (~50KB)
- `runtime/session_test.go` — 会话 CRUD 与原子写入
- `tools/*_test.go` — 各工具独立行为 + Manager 管道集成
- `security/*_test.go` — 权限规则匹配 + 沙箱路径安全
- `tui/update_test.go` — Update 分支全覆盖 (~80KB)
- `context/*_test.go` — 规则加载、System State 收集、Prompt 组装

---

## 十二、扩展指南

### 12.1 添加新的 Provider 驱动

1. 在 `internal/provider/` 下创建新包（如 `anthropic/`）
2. 实现 `Provider` 接口的 `Chat()` 方法
3. 实现 `Driver()` 函数返回 `DriverDefinition`
4. 在 `internal/provider/builtin/builtin.go` 的 `Register()` 中注册
5. 在 `internal/config/builtin_providers.go` 添加内置定义

### 12.2 添加新的 Tool

1. 在 `internal/tools/` 下创建新包
2. 实现 `Tool` 接口的 4 个方法: `Name()`, `Description()`, `Schema()`, `Execute()`
3. 在 `internal/app/bootstrap.go` 的 `buildToolRegistry()` 中注册
4. 如需权限控制，在 `permission_mapper.go` 中添加映射

### 12.3 添加新的斜杠命令

1. 在 `internal/tui/commands.go` 中添加命令常量和描述
2. 在 `builtinSlashCommands` 切片中注册
3. 在 `executeLocalCommand()` 的 switch 中处理
4. 可选: 在 `matchingSlashCommands()` 中支持模糊匹配

---

## 附录 A: 关键接口汇总

| 接口 | 所在包 | 核心方法 | 实现者 |
|------|--------|----------|--------|
| `Provider` | provider | `Chat(ctx, req, events)` | openai.Provider |
| `ProviderV2` | provider | `CompleteTurn(ctx, req, events)` | (预留) |
| `ProviderFactory` | runtime | `Build(ctx, cfg)` | *provider.Registry |
| `Runtime` | runtime | `Run/CancelActiveRun/Events/ListSessions` | *runtime.Service |
| `Builder` | context | `Build(ctx, input)` | *context.DefaultBuilder |
| `Tool` | tools | `Name/Description/Schema/Execute` | 7 个具体工具 |
| `Manager` | tools | `ListAvailableSpecs/Execute` | *tools.DefaultManager |
| `Executor` | tools | `ListAvailableSpecs/Execute/Supports` | *tools.Registry |
| `PermissionEngine` | security | `Check(ctx, action)` | *security.StaticGateway |
| `WorkspaceSandbox` | security | `Check(ctx, action)` | *security.WorkspaceSandbox |
| `Store` (runtime) | runtime | `Save/Load/ListSummaries` | *JSONSessionStore |
| `Store` (catalog) | catalog | `Save/Load` | *JSONStore |
| `ProviderController` | tui | `ListProviders/SelectProvider/ListModels/SetCurrentModel` | *selection.Service |

---

## 附录 B: 默认配置速查

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `MaxLoops` | 8 | ReAct 最大循环次数 |
| `ToolTimeoutSec` | 20 | 单个工具超时(秒) |
| `WebFetch.MaxResponseBytes` | 256KB | webfetch 最大响应 |
| `WebFetch.SupportedContentTypes` | html/xhtml/json/xml/text | 允许的 MIME 类型 |
| `Shell` | windows=powershell, other=bash | 默认 Shell |
| `maxContextTurns` | 10 | 保留的消息轮数 |
| `defaultProviderRetryMax` | 2 | runtime 层重试次数 |
| `providerRetryBaseWait` | 1s | 重试初始等待 |
| `providerRetryMaxWait` | 5s | 重试最大等待 |
| Events Channel Cap | 128 | 事件通道缓冲 |
| `ruleFileName` | AGENTS.md | 项目规则文件名 |
| `maxRuleFileRunes` | 4000 | 单文件规则上限 |
| `maxTotalRuleRunes` | 12000 | 规则总量上限 |
| `catalogTTL` | 24h | 模型目录缓存时间 |
