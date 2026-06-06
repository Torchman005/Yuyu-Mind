# Yuyu-Mind

Yuyu-Mind 是一个基于 Wails + React + Python Agent 的桌面智能体 / 桌宠助手项目。它围绕 Live2D 桌宠、语音对话、长期记忆、多模型接入和插件扩展构建，目标是让桌宠更像一个能陪伴、能观察、能协助工作的常驻伙伴。

Yuyu-Mind is a Wails + React + Python Agent desktop companion. It focuses on Live2D desktop pet interaction, voice chat, memory, multi-provider LLM support, and a lightweight plugin framework.

## 功能

- 桌面聊天界面与桌宠模式
- Live2D Avatar 渲染，支持桌宠缩放、拖动和透明点击穿透
- SQLite 持久化聊天记录与简单记忆候选
- Python Agent 负责回复生成、情绪选择和记忆候选
- DeepSeek / OpenAI / OpenRouter / Ollama 多模型厂商配置
- Fish Audio / GPT-SoVITS 等 TTS 路径
- 连续语音模式，包含噪声过滤、音量阈值门控和打断对话
- 插件框架，具体插件可放在独立插件仓库中维护

## 运行

首次运行建议创建项目自己的 Python 虚拟环境，并安装 Agent/TTS 依赖：

```powershell
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install -r requirements.txt
```

启动前端和桌面应用：

```powershell
wails dev
```

如果需要单独启动 Python Agent：

```powershell
.\.venv\Scripts\python.exe agent/main.py
```

Agent 默认监听：

```text
http://127.0.0.1:8765
```

## 配置

复制示例配置：

```powershell
Copy-Item .env.example .env
```

DeepSeek 示例：

```env
LLM_PROVIDER=deepseek
DEEPSEEK_API_KEY=your_deepseek_api_key_here
DEEPSEEK_MODEL=deepseek-v4-flash
DEEPSEEK_BASE_URL=https://api.deepseek.com
DEEPSEEK_THINKING=disabled
```

支持的 provider：

- `deepseek`
- `openai`
- `openrouter`
- `ollama`

## 人设与回复语言

默认桌宠名是 `Yuyu`。界面默认显示中文，语音默认可配置为日语 TTS 文本：

```env
YUYU_REPLY_LANGUAGE=zh_ja
YUYU_DESKTOP_PET_NAME=Yuyu
VITE_YUYU_DESKTOP_PET_NAME=Yuyu
YUYU_USER_NICKNAME=主人
YUYU_PERSONA=Yuyu is a cute anime-style desktop companion. She is warm, playful, slightly shy, loyal, and practical. She talks like a gentle Japanese anime assistant, with natural charm but not exaggerated roleplay. She can help with coding and desktop tasks while keeping a soft companion tone.
```

如果想让界面和语音都用日语，可以设置：

```env
YUYU_REPLY_LANGUAGE=ja
```

如果想跟随用户语言回复，可以设置：

```env
YUYU_REPLY_LANGUAGE=auto
```

旧的 `MOCHI_*` 环境变量仍然兼容，但新配置建议使用 `YUYU_*`。

## Fish Audio TTS

Fish Audio 配置示例：

```env
TTS_PROVIDER=fish
TTS_TIMEOUT_SECONDS=12
FISH_AUDIO_API_KEY=your_fish_audio_api_key_here
FISH_AUDIO_REFERENCE_ID=your_cloned_voice_reference_id_here
FISH_AUDIO_MODEL=s2-pro
FISH_AUDIO_TTS_URL=https://api.fish.audio/v1/tts
FISH_AUDIO_PROXY=
FISH_AUDIO_AUTO_PROXY=false
```

Fish Audio live / streaming 语音路径：

```env
VITE_SPEECH_OUTPUT_MODE=cloud
VITE_ENABLE_STREAMING_TTS=true
VITE_REALTIME_SPEECH=true
FISH_AUDIO_WS_URL=wss://api.fish.audio/v1/tts/live
FISH_AUDIO_STREAM_CHUNK_LENGTH=300
FISH_AUDIO_END_SILENCE_SECONDS=2
```

调试语音耗时面板：

```env
VITE_SHOW_SPEECH_DEBUG=true
```

## 连续语音与噪声门控

连续语音模式会在桌宠说完后自动重新等待用户说话。为了避免环境杂音被当成输入，前端支持音量阈值门控：

```env
VITE_VOICE_GATE_ENABLED=true
VITE_VOICE_GATE_THRESHOLD=0.035
VITE_VOICE_GATE_HOLD_MS=160
VITE_VOICE_GATE_TIMEOUT_MS=12000
```

如果杂音仍会触发识别，可以提高 `VITE_VOICE_GATE_THRESHOLD`，例如 `0.045`。如果你正常说话不容易触发，可以降低到 `0.025` 或 `0.03`。

## GPT-SoVITS

Yuyu-Mind 支持通过本地 GPT-SoVITS API 进行语音克隆。先启动 GPT-SoVITS API：

```powershell
python api_v2.py -a 127.0.0.1 -p 9880 -c GPT_SoVITS/configs/tts_infer.yaml
```

然后配置：

```env
TTS_PROVIDER=gpt-sovits
GPT_SOVITS_URL=http://127.0.0.1:9880/tts
GPT_SOVITS_API_STYLE=v2
GPT_SOVITS_MEDIA_TYPE=wav
GPT_SOVITS_REF_AUDIO_PATH=D:\voices\yuyu_ref.wav
GPT_SOVITS_PROMPT_TEXT=这里填写参考音频里真实说出的原文
GPT_SOVITS_PROMPT_LANG=zh
GPT_SOVITS_TEXT_LANG=zh
```

更完整的配置说明见：

```text
docs/gpt-sovits-tts.md
```

## Live2D

Avatar 渲染由前端环境变量控制：

```env
VITE_AVATAR_RENDERER=css
```

Cubism 5 bridge 示例：

```env
VITE_AVATAR_RENDERER=cubism5
VITE_LIVE2D_MODEL_URL=/models/yumi/yumi.model3.json
VITE_CUBISM5_CORE_URL=/live2d/cubism5/live2dcubismcore.min.js
VITE_CUBISM5_RENDERER_URL=/live2d/cubism5/mochi-cubism5-renderer.js
```

`MochiCubism5Renderer` 和 `mochi-cubism5-renderer.js` 是历史桥接命名，当前仍保留以兼容已有 Live2D 渲染文件。

## 插件

主仓库只保留插件框架入口：

```text
agent/plugins/__init__.py
```

具体插件建议放在独立仓库中维护，再复制或安装到：

```text
agent/plugins/<plugin-name>/
```

插件 manifest 支持新的协议版本：

```json
{
  "schemaVersion": "yuyu.plugin.v1"
}
```

旧的 `mochi.plugin.v1` 仍然兼容。

## 构建

```powershell
wails build
```

构建产物：

```text
build/bin/Yuyu-Mind.exe
```

## 当前架构

```text
Go / Wails
  app.go              SQLite, chat API, Python Agent bridge, Go fallback
  main.go             desktop process and frontend binding

React Frontend
  frontend/src/App.tsx
  frontend/src/components/Live2DStage.tsx
  frontend/src/App.css

Python Agent
  agent/main.py       stdlib HTTP service, provider registry, local fallback
  agent/plugins/      plugin framework entry and external plugins

Storage
  data/yuyu-mind.db   created at runtime
  data/mochi.db       legacy database path, still read when present
```

## 备注

- `.env`、`frontend/.env` 和任何 API Key 不要提交到仓库。
- 具体读屏插件、视觉插件等可以单独拆到插件仓库。
- 如果旧配置里仍有 `MOCHI_*`，应用会继续读取；新配置建议逐步迁移到 `YUYU_*`。
