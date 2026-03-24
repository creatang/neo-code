# NeoCode TUI 增强版架构设计指南 (细化版)

## 核心设计

本架构基于 **TEA (The Elm Architecture)** 模式，遵循“**单向数据流**”与“**逻辑/视图/通信彻底解耦**”的原则。
- **状态驱动**：界面是状态（State）的函数。
- **异步解耦**：所有耗时操作（网络、I/O）必须通过 `tea.Cmd` 异步执行。
- **物理隔离**：TUI 层严禁引用 `internal/server` 目录下的任何非 API 结构。

---

## 细化后的六层架构模型

我们将整个 TUI 客户端细化为六个逻辑层，每个层级有严格的边界限制：

### 入口层 (Entry) - `cmd/tui/`

*   **职责**：程序的物理起点。
*   **具体任务**：
    *   解析命令行启动参数（如 `--debug`, `--config`）。
    *   调用 `bootstrap` 层获取初始化好的 `Program` 实例。
    *   启动 `tea.NewProgram` 并处理最终的退出错误。
*   **禁止**：严禁编写任何业务逻辑或具体的 UI 布局代码。

### 启动与注入层 (Bootstrap) - `internal/tui/bootstrap/`

*   **职责**：系统的“总装车间”，负责**依赖注入 (DI)**。
*   **具体任务**：
    *   读取 `config.yaml` 配置文件。
    *   实例化 `services` 层（如 `APIClient`, `Logger`）。
    *   将这些服务注入到 `app` 层中。
    *   **关键作用**：通过在这一层注入不同的实现，可以轻松实现“离线测试模式”或“Mock 测试”。

### 应用逻辑层 (App/Core) - `internal/tui/app/`

*   **职责**：状态机中心，负责调度 `Update` 和 `View`。
*   **具体文件**：
    *   `model.go`: 定义顶层 `Model`，聚合各子模块状态。
    *   `update.go`: 核心业务逻辑路由器。根据收到的 `tea.Msg` 决定调用哪个 Service 或更新哪个 State。
    *   `view.go`: 顶层布局管理器。决定 `Header`, `Content`, `Footer` 的排版位置（使用 Lipgloss）。
    *   `msg.go`: 定义所有自定义消息类型（如 `AIGeneratingMsg`, `SocketErrorMsg`）。

### 纯状态层 (State) - `internal/tui/state/`

*   **职责**：**数据容器**。仅存放纯粹的 Go 结构体。
*   **具体任务**：
    *   `ui_state.go`: 记录 UI 细节（如：窗口宽高、当前焦点在哪个输入框、滚动条位置）。
    *   `chat_state.go`: 存放当前的聊天历史、AI 思考中的临时文本。
*   **准则**：这一层**不含任何方法**，只存放数据，确保状态的可序列化和易测试性。

### 视图组件层 (Components) - `internal/tui/components/`

*   **职责**：**原子级 UI 渲染器**（“傻瓜组件”）。
*   **具体任务**：
    *   `code_block.go`: 负责代码高亮渲染。
    *   `status_bar.go`: 负责底部状态栏的样式。
*   **原则**：
    *   **输入**：仅接收基础数据或 State 结构体。
    *   **输出**：返回渲染好的字符串（`string`）。
    *   **禁止**：组件内严禁发起任何网络请求或修改全局状态。

### 服务对接层 (Services) - `internal/tui/services/`

*   **职责**：**外交部**。负责与后端 Server 或系统环境通信。
*   **具体任务**：
    *   `api_client.go`: 封装对 `internal/server/transport` 的调用。
    *   `file_service.go`: 处理本地文件的临时读取。
*   **原则**：所有方法必须返回 `tea.Cmd` 或在回调中触发 `tea.Msg`。

---

## 标准数据流向 (Lifecycle)

以“用户发送消息”为例：
1.  **用户按下回车**：`app/update.go` 捕获到按键事件。
2.  **更新本地状态**：`update.go` 将用户输入追加到 `state/chat_state.go`，并返回一个 `tea.Cmd` 触发发送请求。
3.  **服务调用**：`services/api_client.go` 执行异步 API 调用。
4.  **结果反馈**：API 返回结果后，封装成 `APIResponseMsg` 发回给 `app/update.go`。
5.  **界面重绘**：`app/view.go` 根据更新后的 `state` 重新生成字符串，渲染到屏幕。



#### 用户视角：一个“发送消息”动作的全层级演变


  假设用户在终端输入了 "你好" 并按下 回车键。以下是各层级像齿轮一样咬合转动的过程：                       
 第一阶段：输入捕获 (Entry -> App)
   * 用户看到：手指按下回车。
   * 层级变化：入口层 (Entry) 的底层的 tea.Program 捕获到操作系统发来的按键信号，并将其包装成一个 KeyMsg  
     发送给 应用逻辑层 (App) 的 Update 函数。


  第二阶段：本地状态更新 (App -> State -> View)
   * 层级变化：
       1. App (Update)：识别出这是回车键。
       2. State：Update 把 state.ChatState.InputBuffer 里的 "你好" 提取出来，清空输入框，并把这条消息塞进 
          state.ChatState.History 数组。
       3. View：Runtime 立即调用 View。View 发现 History 里多了一条消息，于是命令 组件层 (Components)  渲染一个新的气泡。
   * 用户看到：屏幕上自己发送的 "你好" 瞬间出现在了聊天记录区域，且输入框变空了。（此时 AI
     还没说话，但用户感觉响应非常快）


  第三阶段：发起异步请求 (App -> Services)
   * 层级变化：
       1. App (Update)：在更新完本地状态的同时，返回一个 tea.Cmd。这个命令指向 服务层 (Services) 的      SendToAI 函数。
       2. Services：在后台偷偷发起网络请求，把 "你好" 发给后端的 Go Server。


  第四阶段：AI 响应回流 (Services -> App -> State -> View)
   * 层级变化：
       1. Services：收到后端返回的 AI 回复（如 "你好！我是 NeoCode"），将其包装成一个 AIResponseMsg 投递回App。
       2. App (Update)：收到这个消息，再次修改 State，把 AI 的话加入 History。
       3. View：Runtime 再次触发 View 重绘。
   * 用户看到：屏幕上刷新出了 AI 的回复。

---

## 细化后的目录结构

```text
internal/tui/
├── bootstrap/          # 依赖装配 (Runtime 构造器)
│   └── runtime.go
├── app/                # 状态机核心 (TEA 循环)
│   ├── model.go        # 顶层模型定义
│   ├── update.go       # 消息分发逻辑
│   ├── view.go         # 顶层布局 (Layout)
│   ├── msg.go          # 消息类型定义
│   └── keymap.go       # 快捷键配置
├── state/              # 纯状态定义 (数据结构)
│   ├── ui_state.go     # 窗口、焦点等状态
│   └── chat_state.go   # 聊天记录等数据
├── components/         # 纯 UI 组件 (Lipgloss 渲染)
│   ├── code_block.go   # 代码块组件
│   ├── input_box.go    # 输入框增强
│   └── status_bar.go   # 状态栏组件
└── services/           # 外部适配器 (API/I/O)
    ├── api_client.go   # 后端通信
    └── config_svc.go   # 配置读取
```

---

## 开发守则 (Constraints)

1.  **禁止跨层修改**：`components` 里的代码绝对不能修改 `state` 里的数据，必须通过 `Update` 函数统一处理。
2.  **样式与逻辑分离**：所有的颜色、边距（Lipgloss 样式）应在 `components` 中定义，`app/view.go` 仅负责大框架的拼装。
3.  **零后端业务依赖**：TUI 只能依赖 `api/proto` 中定义的结构。如果后端修改了业务逻辑，只要 API 不变，TUI 代码应保持一行不改。
4.  **异步命令化**：任何可能超过 10ms 的操作（读取大文件、调用 AI）必须封装在 `services` 层返回 `tea.Cmd`。
