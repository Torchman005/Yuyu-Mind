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

首次运行建议先创建当前项目自己的 Python 虚拟环境，并安装 Agent/TTS 依赖：

For the first run, create a Python virtual environment inside this project and install the Agent/TTS dependencies:

```powershell
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install -r requirements.txt
```

`requirements.txt` 只包含当前 Python Agent 和 Fish Audio TTS 需要的基础依赖。参考项目 `Mochi-AI` 里还有本地 ASR、麦克风录音、本地 TTS、GUI 等依赖；这些已单独放在 `requirements-voice.txt`，等实现本地语音流水线时再安装：

`requirements.txt` only includes the dependencies needed by the current Python Agent and Fish Audio TTS path. The reference `Mochi-AI` project also includes local ASR, microphone capture, local TTS, GUI, and other dependencies; those optional voice-pipeline dependencies are listed separately in `requirements-voice.txt` and should be installed when those features are implemented:

```powershell
.\.venv\Scripts\python.exe -m pip install -r requirements-voice.txt
```

然后启动 Wails：

Then start Wails:

```powershell
wails dev
```

Go 桌面进程会尝试自动从 `agent/main.py` 启动 Python Agent。你也可以手动启动：

The Go desktop process will try to start the Python Agent automatically from `agent/main.py`. You can also run it manually:

```powershell
.\.venv\Scripts\python.exe agent/main.py
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

## 人设与回复语言 / Persona and Reply Language

Mochi 默认是可爱的二次元桌面伙伴。默认模式是界面显示中文，语音朗读日语：Agent 会返回 `text` 作为中文 UI 文本，同时返回 `speechText` 作为日语 TTS 文本。

Mochi is a cute anime-style desktop companion by default. The default mode displays Chinese in the UI and speaks Japanese: the Agent returns `text` for Chinese UI text and `speechText` for Japanese TTS text.

```text
MOCHI_REPLY_LANGUAGE=zh_ja
MOCHI_PERSONA=Mochi is a cute anime-style desktop companion. She is warm, playful, slightly shy, loyal, and practical. She talks like a gentle Japanese anime assistant, with natural charm but not exaggerated roleplay. She can help with coding and desktop tasks while keeping a soft companion tone.
```

如果想让界面和语音都用日语，可以把 `MOCHI_REPLY_LANGUAGE` 改成 `ja`。如果想临时回到跟随用户语言回复，可以改成 `auto`，并按需要改写 `MOCHI_PERSONA`。

To make both UI and voice Japanese, set `MOCHI_REPLY_LANGUAGE` to `ja`. To temporarily reply in the user's language, set it to `auto` and adjust `MOCHI_PERSONA` as needed.

## Fish Audio TTS 配置 / Fish Audio TTS Setup

语音回复使用 Fish Audio。Key 只应写入本地 `.env`，不要提交到仓库：

Voice replies use Fish Audio. Keep the key in your local `.env` only, and do not commit it:

```text
FISH_AUDIO_API_KEY=your_fish_audio_api_key_here
FISH_AUDIO_REFERENCE_ID=your_cloned_voice_reference_id_here
FISH_AUDIO_MODEL=s2-pro
FISH_AUDIO_TTS_URL=https://api.fish.audio/v1/tts
FISH_AUDIO_PYTHON_PATH=
FISH_AUDIO_PROXY=
FISH_AUDIO_AUTO_PROXY=false
```

应用会通过 Go 后端调用 `agent/tts_fish.py`，再由 Python 请求 Fish Audio 并把音频交给前端播放。默认会优先使用当前项目的 `.venv\Scripts\python.exe`；如果你需要指定其它 Python，可填写 `FISH_AUDIO_PYTHON_PATH`。`FISH_AUDIO_PROXY` 可显式设置代理，例如 `http://127.0.0.1:7890`；如果想强制直连，可设为 `direct`。默认不盲扫本地端口，只有 `FISH_AUDIO_AUTO_PROXY=true` 时才会尝试常见本地代理端口。如果 TTS 失败，前端会临时回退到系统朗读。

The app asks the Go backend to run `agent/tts_fish.py`; Python then calls Fish Audio and passes the generated audio back to the frontend. By default, it prefers this project's `.venv\Scripts\python.exe`; set `FISH_AUDIO_PYTHON_PATH` only when you need a custom Python executable. `FISH_AUDIO_PROXY` can explicitly set a proxy, for example `http://127.0.0.1:7890`; set it to `direct` to force direct access. The script does not blindly scan local ports by default; it only tries common local proxy ports when `FISH_AUDIO_AUTO_PROXY=true`. If TTS fails, the frontend temporarily falls back to system speech synthesis.

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
