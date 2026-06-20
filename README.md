# Yuyu Mind

AI 桌面应用，基于 Wails + Eino 框架构建。

## 技术栈

- **后端**: Go + [Eino](https://github.com/cloudwego/eino) (AI 编排框架)
- **前端**: Wails (Web 前端)
- **存储**: SQLite

## 功能

- 💬 对话聊天（流式输出）
- 🔧 工具调用（Function Calling）
- 🔄 多 LLM 提供商支持（OpenAI / DeepSeek / Moonshot / Ollama 等）
- 💾 对话持久化（SQLite）

## 开发

```bash
# 安装依赖
go mod tidy

# 开发模式
make dev

# 构建
make build

# 测试
make test
```

## 项目结构

```
├── main.go                 # 应用入口
├── app.go                  # Wails 绑定层
├── internal/
│   ├── app/                # Wails 应用生命周期
│   ├── config/             # 配置管理
│   ├── db/                 # SQLite 数据库 & 仓库
│   ├── ai/                 # AI 核心模块
│   │   ├── provider/       # LLM 提供商注册
│   │   ├── pipeline/       # Eino Chain/Graph 流水线
│   │   ├── template/       # Prompt 模板
│   │   ├── tools/          # 工具注册 & 实现
│   │   └── callback/       # Eino 回调
│   ├── chat/               # 聊天编排服务
│   └── memory/             # 对话记忆管理
├── pkg/types/              # 公共类型
├── configs/                # 配置文件
└── frontend/               # Wails 前端
```
