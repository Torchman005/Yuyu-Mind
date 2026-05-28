# MochiAI

## 简介 / Overview

MochiAI 是一个基于 Wails + React + Python Agent 的桌面智能体女友 / 桌面助手 MVP。当前阶段聚焦文字聊天、角色舞台、情绪状态、本地记忆、多模型厂商接入和可插拔 Avatar 渲染。

MochiAI is a Wails + React + Python Agent desktop companion / assistant MVP. This milestone focuses on text chat, a character stage, emotion states, local memory, multi-provider model support, and pluggable avatar rendering.

当前已支持：

Current features:

- 桌面聊天界面  
  Desktop chat UI
- SQLite 持久化聊天记录和简单记忆候选  
  SQLite persistence for chat history and simple memory candidates
- Python Agent 负责回复生成、情绪选择和记忆候选  
  Python Agent service for reply generation, emotion selection, and memory candidates
- DeepSeek / OpenAI / OpenRouter / Ollama 多模型厂商配置  
  DeepSeek / OpenAI / OpenRouter / Ollama model provider configuration
- 可插拔 Avatar 渲染：CSS fallback、legacy Pixi Live2D、Cubism 5 bridge  
  Pluggable avatar rendering: CSS fallback, legacy Pixi Live2D, and Cubism 5 bridge

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

## 模型 API 配置 / Model API Setup

应用不配置 API Key 也可以运行，此时会使用本地规则兜底回复。默认 provider 是 DeepSeek。要启用真实模型回复，复制 `.env.example` 为 `.env`，然后填入对应厂商的 Key：

The app works without an API key by using the local rule-based fallback. The default provider is DeepSeek. To enable real model replies, copy `.env.example` to `.env` and fill in the key for your selected provider:

```powershell
Copy-Item .env.example .env
```

DeepSeek 示例：

DeepSeek example:

```text
LLM_PROVIDER=deepseek
DEEPSEEK_API_KEY=your_deepseek_api_key_here
DEEPSEEK_MODEL=deepseek-chat
DEEPSEEK_BASE_URL=https://api.deepseek.com
DEEPSEEK_THINKING=disabled
```

切换 provider 只需要改 `LLM_PROVIDER` 和对应配置。

To switch providers, update `LLM_PROVIDER` and the matching provider settings.

支持的 provider：

Supported providers:

- `deepseek`
- `openai`
- `openrouter`
- `ollama`

## Avatar / Live2D 配置

前端 Avatar 渲染由 `frontend/.env` 控制。复制模板：

Avatar rendering is controlled by `frontend/.env`. Copy the template:

```powershell
Copy-Item frontend/.env.example frontend/.env
```

### CSS fallback

默认模式是 CSS fallback，稳定且不依赖 Live2D runtime：

The default mode is CSS fallback. It is stable and does not require a Live2D runtime:

```text
VITE_AVATAR_RENDERER=css
```

### Cubism 5 bridge

Cubism 5 不再使用 `pixi-live2d-display` 直接加载。请使用 Live2D 官方 Cubism SDK for Web 构建一个 renderer bridge，并让它暴露：

Cubism 5 is no longer loaded directly through `pixi-live2d-display`. Use the official Live2D Cubism SDK for Web to build a renderer bridge that exposes:

```ts
window.MochiCubism5Renderer = {
  async create({ canvas, modelUrl, emotion }) {
    return {
      setEmotion(emotion) {},
      resize() {},
      destroy() {}
    }
  }
}
```

然后配置：

Then configure:

```text
VITE_AVATAR_RENDERER=cubism5
VITE_LIVE2D_MODEL_URL=/models/yumi/yumi.model3.json
VITE_CUBISM5_CORE_URL=/live2d/cubism5/live2dcubismcore.min.js
VITE_CUBISM5_RENDERER_URL=/live2d/cubism5/mochi-cubism5-renderer.js
```

文件放置建议：

Recommended file layout:

```text
frontend/public/models/yumi/yumi.model3.json
frontend/public/live2d/cubism5/live2dcubismcore.min.js
frontend/public/live2d/cubism5/mochi-cubism5-renderer.js
```

### Legacy Pixi Live2D

旧的 Pixi 渲染器只建议用于 Cubism 3/4 模型：

The legacy Pixi renderer is only recommended for Cubism 3/4 models:

```text
VITE_AVATAR_RENDERER=legacy-pixi
VITE_LIVE2D_MODEL_URL=/models/yumi/yumi.model3.json
VITE_LIVE2D_CUBISM_CORE_URL=/live2d/legacy/live2dcubismcore.min.js
```

## 构建 / Build

```powershell
wails build
```

构建产物路径：

Build output:

```text
build/bin/MochiAI.exe
```

## 当前架构 / Current Architecture

```text
Go / Wails
  app.go              SQLite, chat API, Python Agent bridge, Go fallback
  main.go             desktop process and frontend binding

React Frontend
  src/App.tsx
  src/components/Live2DStage.tsx
  src/App.css

Python Agent
  agent/main.py       stdlib HTTP service, provider registry, local fallback

Storage
  data/mochi.db       created at runtime
```

## 下一步 / Next Milestones

1. 实现官方 Cubism 5 renderer bridge。  
   Implement the official Cubism 5 renderer bridge.
2. 添加 TTS 语音播放，再接入流式 ASR/VAD。  
   Add TTS playback, then streaming ASR/VAD.
3. 将简单 SQLite 记忆扩展为用户档案、偏好、事件、语义和向量检索记忆。  
   Expand memory into profile, preference, episodic, semantic, and vector-retrieved memory.
4. 在明确权限提示下接入屏幕读取、浏览器和代码插件。  
   Add screen reading and browser/code plugins behind explicit permission prompts.
