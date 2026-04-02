用户发起任务
   |
   v
Claude Code 开始构建本次对话的 system prompt
   |
   +--> 1. 加载核心指令
   |        - 角色
   |        - 行为准则
   |        - 安全边界
   |
   +--> 2. 生成工具定义
   |        - 当前可用工具
   |        - 每个工具的描述和参数 schema
   |
   +--> 3. 读取用户上下文
   |        - 从当前目录向上查找 AGNETS.md
   |        - 读取全局 / 项目 / 子目录级 AGENTS.md
   |        - 合并后格式化
   |
   +--> 4. 收集系统上下文
   |        - git 当前分支
   |        - 主分支
   |        - 最近 commit
   |        - 工作区状态
   |        - 格式化为可读文本
   |
   +--> 5. 追加用户自定义 system prompt
   |        - --system-prompt
   |        - --append-system-prompt
   |
   +--> 6. 拼接成最终 system prompt
   |        - stable 部分优先
   |        - dynamic 部分后置
   |
   v
把最终 system prompt 连同用户问题一起发给模型
   |
   v
模型基于“项目信息 + 系统状态 + 工具能力”生成响应