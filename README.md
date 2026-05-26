# MochiAI

MochiAI is a Wails + React desktop companion MVP. This first milestone focuses on the smallest useful loop:

- Text chat with a desktop companion UI
- A Live2D-ready character stage with emotion states
- Assistant speech bubble synced to the latest reply
- SQLite persistence for chat history and simple memory candidates
- Go backend bindings exposed to the React frontend
- Python Agent service for reply generation, emotion selection, and memory candidates

## Run

```powershell
wails dev
```

The Go desktop process will try to start the Python Agent automatically from `agent/main.py`.
You can also run it manually:

```powershell
python agent/main.py
```

The Agent listens on:

```text
http://127.0.0.1:8765
```

## Build

```powershell
wails build
```

The latest verified build output is:

```text
build/bin/MochiAI.exe
```

## Current Architecture

```text
Go / Wails
  app.go              SQLite, chat API, Python Agent bridge, Go fallback
  main.go             desktop process and frontend binding

React Frontend
  src/App.tsx         chat UI, companion stage, emotion state
  src/App.css         responsive desktop companion styling

Python Agent
  agent/main.py       stdlib HTTP service for /health and /chat

Storage
  data/mochi.db       created at runtime
```

## Backend API

- `GetState()` loads persisted chat history and current emotion.
- `SendMessage(content)` saves the user message, sends context to the Python Agent, saves the assistant reply, stores memory candidates, and returns the refreshed conversation.

If the Python Agent is unavailable, Go returns a small fallback reply so the desktop app remains usable.

## Next Milestones

1. Replace the rule-based Python reply function with a real LLM provider.
2. Add a real Live2D renderer in the frontend, mapping `neutral/happy/focused/thinking` to model expressions and motions.
3. Add TTS playback, then streaming ASR/VAD for voice conversation.
4. Expand memory from simple SQLite rows into profile, preference, episodic, semantic, and vector-retrieved memory.
5. Add screen reading and browser/code plugins behind explicit permission prompts.
