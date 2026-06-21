# go-tiny-claw

一个基于 Go 语言构建的轻量级 Agent 运维平台，支持飞书长连接交互、多模型接入、工具调用、子智能体协作等能力。

## 项目简介

go-tiny-claw 是一个面向运维场景的 AI Agent 框架，通过飞书 WebSocket 长连接实现人机对话交互。Agent 可以自主调用工具（执行命令、读写文件、编辑代码）来完成运维任务，同时内置了上下文压缩、错误自愈、审批拦截、可观测性追踪等生产级特性。

## 核心功能

### 1. 飞书长连接交互
- 通过 WebSocket 长连接与飞书实时通信，无需公网 IP 或内网穿透
- 支持多会话管理，每个飞书群聊拥有独立的工作区和会话历史
- Agent 执行过程实时推送到飞书群（思考、工具调用、结果反馈）

### 2. 多模型支持
- **OpenAI 兼容协议**：支持 DeepSeek、GPT 等兼容 OpenAI API 的模型
- **Claude 原生支持**：接入 Anthropic Claude 系列模型
- 统一的 `LLMProvider` 接口，方便扩展更多模型

### 3. 工具调用与子智能体协作
- 内置 `bash`、`read_file`、`write_file`、`edit_file` 等运维工具
- 支持中间件机制，可自定义审批、日志、限流等拦截逻辑
- 可派出只读子智能体进行深度代码探索，返回精炼摘要辅助决策

### 4. 上下文智能管理
- **动态组装**：自动加载 `AGENTS.md` 项目规范和 `.claw/skills/` 专业技能，注入 System Prompt
- **智能压缩**：上下文超限时自动清理早期内容，保护最近对话，截断冗长工具输出
- **错误自愈**：针对工具常见错误提供救援提示，引导 Agent 自主修正
- **死循环防护**：检测连续失败并强制注入警告，打破重试循环
- **Plan Mode**：强制将长程任务持久化到 `PLAN.md` 和 `TODO.md`，实现断点续传

### 5. 安全审批机制
- 自动拦截危险操作（文件写入/编辑、`rm -rf`、`sudo`、`kill` 等）
- 通过飞书卡片消息发送审批请求，支持一键点击"批准/拒绝"按钮
- 保留文本命令审批的向后兼容（回复 `approve/reject {taskID}`）

### 6. 可观测性与追踪
- 内置分布式追踪系统，自动记录每次 Agent 运行的完整链路
- 追踪层级：`Agent.Run` → `Turn N` → `LLM.Thinking` / `LLM.Action` → `Tool.Execute`
- 每次运行自动生成 JSON Trace 文件，保存在 `.claw/traces/` 目录
- 自动统计每次会话的 Token 消耗和费用

## 项目解决了哪些问题

| 痛点 | 解决方案 |
|------|----------|
| Agent 容易执行危险操作导致系统损坏 | 高危命令自动拦截 + 飞书卡片审批 |
| 长对话导致上下文超出 Token 限制 | 智能上下文压缩，自动清理早期内容 |
| 工具调用失败后 Agent 反复重试陷入死循环 | 错误自愈提示 + 死循环强制打断 |
| 无法追溯 Agent 的执行过程和决策链路 | 全链路 Trace 追踪，每次运行可回放 |
| 多模型切换困难 | 统一 Provider 接口，一行代码切换模型 |
| 复杂任务需要大量代码阅读，消耗主 Agent Token | 子智能体分工协作，只读探索后返回摘要 |
| 无法知道 Agent 花了多少钱 | 自动成本追踪，记录每次会话的 Token 和费用 |
| 部署需要公网 IP 或内网穿透 | 飞书 WebSocket 长连接，本地即可运行 |
| 不同项目需要不同的运维规范和操作纪律 | AGENTS.md + Skills 系统，动态注入项目专属约束 |
| Agent 重启后丢失任务进度，无法断点续传 | Plan Mode 强制状态外部化，持久化到物理文件 |

## 快速开始

### 环境要求
- Go 1.26+

### 配置

1. 复制 `.env` 文件并填写配置：
```env
OPENAI_API_KEY=your_api_key
FEISHU_APP_ID=your_app_id
FEISHU_APP_SECRET=your_app_secret
FEISHU_ENCRYPT_KEY=your_encrypt_key
FEISHU_VERIFY_TOKEN=your_verify_token
```

2. 启动 Agent 服务端：
```bash
cd cmd/claw/agentops
go run main.go
```

3. 在飞书群聊中向机器人发送消息即可开始交互。

## 项目结构

```
go-tiny-claw/
├── cmd/
│   └── claw/
│       ├── agentops/     # 飞书 Agent 运维平台入口
│       ├── bench/        # Agent 能力基准测试
│       └── main.go       # CLI 终端入口
├── internal/
│   ├── context/          # 会话管理、上下文压缩、错误自愈
│   ├── engine/           # Agent 核心引擎（ReAct 循环）
│   ├── eval/             # 基准测试框架
│   ├── feishu/           # 飞书 Bot、审批管理
│   ├── observatibility/  # 链路追踪、成本统计
│   ├── provider/         # LLM 模型接入（OpenAI / Claude）
│   ├── schema/           # 消息、工具调用等数据结构
│   └── tools/            # 内置工具（bash、文件操作、子智能体）
└── workSpace/            # 各会话的独立工作区
```

## 飞书应用配置

1. 在[飞书开放平台](https://open.feishu.cn/)创建企业自建应用
2. 开启以下权限和事件：
   - 消息接收与发送
   - **卡片回传交互**（用于审批按钮回调）
   - 回调方式选择 **使用长连接接收回调**
3. 将 App ID、App Secret、Encrypt Key、Verify Token 填入 `.env`

## License

MIT
