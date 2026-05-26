# MochiAI

## 简介 / Overview

MochiAI 是一个基于 Wails + React 的桌面智能体女友 / 桌面助手 MVP。当前阶段聚焦最小可用闭环：文字聊天、角色舞台、情绪状态、本地记忆和 Python Agent。

MochiAI is a Wails + React desktop companion / assistant MVP. This milestone focuses on the smallest useful loop: text chat, a character stage, emotion states, local memory, and a Python Agent.

当前已支持：

Current features:

- 桌面聊天界面  
  Desktop chat UI
- 面向 Live2D 的角色舞台和情绪状态  
  Live2D-ready character stage with emotion states
- 与最新回复同步的助手气泡  
  Assistant speech bubble synced to the latest reply
- 使用 SQLite 持久化聊天记录和简单记忆候选  
  SQLite persistence for chat history and simple memory candidates
- Go 后端绑定到 React 前端  
  Go backend bindings exposed to the React frontend
- Python Agent 负责回复生成、情绪选择和记忆候选  
  Python Agent service for reply generation, emotion selection, and memory candidates
- 可选 OpenAI Responses API 集成，通过 `.env` 配置  
  Optional OpenAI Responses API integration through `.env`

## 运行 / Run

```powershell
wails dev
```

Go 桌面进程会尝试自动从 `agent/main.py` 启动 Python Agent。你也可以手动启动：

The Go desktop process will try to start the Python Agent automatically from `agent/main.py`. You can also run it manually:

```powershell
python agent/main.py
```

Agent 默认监听：

The Agent listens on:

```text
http://127.0.0.1:8765
```

## OpenAI API 配置 / OpenAI API Setup

应用不配置 API Key 也可以运行，此时会使用本地规则兜底回复。要启用真实模型回复，复制 `.env.example` 为 `.env`，然后填入你的 Key：

The app works without an API key by using the local rule-based fallback. To enable real model replies, copy `.env.example` to `.env` and fill in your key:

```powershell
Copy-Item .env.example .env
```

```text
OPENAI_API_KEY=your_api_key_here
OPENAI_MODEL=gpt-5.4-mini
```

修改 `.env` 后，请重启 `wails dev` 或 `build/bin/MochiAI.exe`。

Restart `wails dev` or `build/bin/MochiAI.exe` after editing `.env`.

Python Agent 使用 OpenAI Responses API，并要求模型返回结构化 JSON：

The Python Agent uses OpenAI's Responses API and asks the model to return structured JSON:

```json
{
  "text": "assistant reply",
  "emotion": "neutral|happy|focused|thinking|sad|surprised",
  "memoryCandidates": []
}
```

如果 API Key 缺失或模型调用失败，Agent 会自动回退到本地回复。

If the API key is missing or a provider call fails, the Agent automatically falls back to local replies.

## 构建 / Build

```powershell
wails build
```

最新验证过的构建产物路径：

The latest verified build output is:

```text
build/bin/MochiAI.exe
```

## 当前架构 / Current Architecture

```text
Go / Wails
  app.go              SQLite, chat API, Python Agent bridge, Go fallback
  main.go             desktop process and frontend binding

React Frontend
  src/App.tsx         chat UI, companion stage, emotion state
  src/App.css         responsive desktop companion styling

Python Agent
  agent/main.py       stdlib HTTP service, OpenAI bridge, local fallback

Storage
  data/mochi.db       created at runtime
```

## 后端接口 / Backend API

- `GetState()` 加载持久化聊天记录和当前情绪。  
  `GetState()` loads persisted chat history and current emotion.
- `SendMessage(content)` 保存用户消息，把上下文发送给 Python Agent，保存助手回复，写入记忆候选，并返回刷新后的会话。  
  `SendMessage(content)` saves the user message, sends context to the Python Agent, saves the assistant reply, stores memory candidates, and returns the refreshed conversation.

如果 Python Agent 不可用，Go 会返回一个简单兜底回复，保证桌面应用仍可使用。如果 Python Agent 在线但未配置 OpenAI，Agent 会使用自己的本地兜底逻辑。

If the Python Agent is unavailable, Go returns a small fallback reply so the desktop app remains usable. If the Python Agent is online but OpenAI is not configured, the Agent uses its own local fallback.

## 下一步 / Next Milestones

1. 接入真正的 Live2D 渲染器，把 `neutral/happy/focused/thinking` 映射到模型表情和动作。  
   Add a real Live2D renderer, mapping `neutral/happy/focused/thinking` to model expressions and motions.
2. 添加 TTS 语音播放，再接入流式 ASR/VAD，实现语音对话。  
   Add TTS playback, then streaming ASR/VAD for voice conversation.
3. 将简单 SQLite 记忆扩展为用户档案、偏好、事件、语义和向量检索记忆。  
   Expand memory from simple SQLite rows into profile, preference, episodic, semantic, and vector-retrieved memory.
4. 在明确权限提示下接入屏幕读取、浏览器和代码插件。  
   Add screen reading and browser/code plugins behind explicit permission prompts.
5. 接入流式模型回复，让文字、语音和口型在完整回复生成前就能开始。  
   Add streaming model responses so text, speech, and mouth movement can start before the full answer is complete.
