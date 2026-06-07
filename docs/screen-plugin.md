# Screen Plugin

MochiAI now has a small Python Agent plugin registry. Plugins are directories with a `plugin.json` manifest and an executable Python entry file. The first built-in plugin is `screen.observe`.

```text
agent/plugins/screen/
  plugin.json
  config.json
  main.py
```

`plugin.json` is the protocol layer: it declares identity, version, permissions, actions, and JSON schemas. `main.py` is the execution layer: it exports an `ACTIONS` dictionary whose keys match declared action names.

`config.json` is the runtime configuration layer. It stores local settings such as capture size and vision model details. Secrets in config are redacted from `/plugins` responses when their schema marks them with `"secret": true`.

## What It Does

- Captures the current desktop screen with `mss`, falling back to Pillow `ImageGrab`.
- Optionally sends the screenshot to an OpenAI-compatible vision model.
- Returns a concise screen summary to the Go app, which passes it into the normal companion reply flow.

## Install

```powershell
pip install -r requirements.txt
```

## Configure Vision

Screen capture works without a vision model, but Mochi can only truly describe the screen after a vision model is configured.

```env
MOCHI_SCREEN_VISION_API_KEY=your_api_key
MOCHI_SCREEN_VISION_BASE_URL=https://api.openai.com/v1
MOCHI_SCREEN_VISION_MODEL=gpt-4.1-mini
MOCHI_SCREEN_CAPTURE_MAX_WIDTH=1280
```

Or configure the plugin directly:

```json
{
  "captureMaxWidth": 1280,
  "returnImage": false,
  "vision": {
    "apiKey": "your_api_key",
    "baseUrl": "https://api.openai.com/v1",
    "model": "gpt-4.1-mini",
    "maxTokens": 420,
    "timeoutSeconds": 45
  }
}
```

`MOCHI_SCREEN_VISION_BASE_URL` can point to any OpenAI-compatible chat completions endpoint that accepts `image_url` content.

## Conversation Integration

Screen observation is also integrated into normal chat:

- If the user says things like "看一下屏幕", "这个报错", "当前界面", or "what do you see", MochiAI asks registered context plugins whether they should provide live context. The screen plugin declares its own triggers in `plugin.json`.
- During pet-mode proactive idle comments, context plugins can occasionally provide context for one low-interruption comment. The screen plugin declares a 20% chance in `plugin.json`.

Environment switches:

```env
MOCHI_PLUGIN_CONTEXT_ENABLED=true
MOCHI_PLUGIN_CONTEXT_CHAT_ENABLED=true
MOCHI_PLUGIN_CONTEXT_PROACTIVE_ENABLED=true
```

Proactive context comments still obey the frontend idle time, cooldown, quiet-hours, and pet-mode checks.

## API

List plugins:

```http
GET http://127.0.0.1:8765/plugins
```

Observe screen:

```http
POST http://127.0.0.1:8765/plugins/screen/observe
Content-Type: application/json

{"prompt":"Describe the visible screen.","includeImage":false}
```

The desktop app calls this through `ObserveScreen()` and then lets the normal Agent persona answer naturally.

## Manifest

Each plugin manifest uses `schemaVersion: "mochi.plugin.v1"`.

```json
{
  "schemaVersion": "mochi.plugin.v1",
  "name": "screen",
  "displayName": "屏幕观察",
  "description": "Capture the desktop screen and optionally summarize it with a configured vision model.",
  "version": "0.1.0",
  "author": "MochiAI",
  "enabled": true,
  "entry": "main.py",
  "permissions": ["screen.capture", "network.vision"],
  "defaultConfig": {
    "captureMaxWidth": 1280,
    "returnImage": false,
    "vision": {
      "apiKey": "",
      "baseUrl": "https://api.openai.com/v1",
      "model": "",
      "maxTokens": 420,
      "timeoutSeconds": 45
    }
  },
  "configSchema": {
    "type": "object",
    "properties": {
      "vision": {
        "type": "object",
        "properties": {
          "apiKey": {"type": "string", "secret": true}
        }
      }
    }
  },
  "actions": [
    {
      "name": "observe",
      "description": "Capture the current desktop screen and return a concise observation.",
      "inputSchema": {"type": "object"},
      "outputSchema": {"type": "object"}
    }
  ]
}
```
