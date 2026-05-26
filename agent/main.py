from __future__ import annotations

import json
import os
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any


HOST = "127.0.0.1"
PORT = 8765
DEFAULT_MODEL = "gpt-5.4-mini"
OPENAI_RESPONSES_URL = "https://api.openai.com/v1/responses"


def load_dotenv() -> None:
    env_path = Path(__file__).resolve().parents[1] / ".env"
    if not env_path.exists():
        return

    for raw_line in env_path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip().strip('"').strip("'")
        if key and key not in os.environ:
            os.environ[key] = value


load_dotenv()


def detect_emotion(message: str) -> str:
    text = message.lower()
    if any(word in message for word in ("开心", "喜欢", "太好了", "谢谢")):
        return "happy"
    if any(word in message for word in ("想", "设计", "为什么", "怎么")):
        return "thinking"
    if any(word in message for word in ("代码", "项目", "报错", "bug")) or "code" in text:
        return "focused"
    return "neutral"


def build_rule_reply(message: str, memories: list[str]) -> dict[str, Any]:
    emotion = detect_emotion(message)
    memory_hint = ""
    if memories:
        memory_hint = f"我还记得你之前提到过：{memories[0]}"

    if any(word in message for word in ("记住", "喜欢")):
        text = "好，我会把这条作为长期记忆候选保存下来。后面接入记忆萃取器后，我会帮你区分偏好、事实和项目状态。"
        candidates = [message]
        emotion = "happy"
    elif any(word in message for word in ("代码", "项目", "报错")):
        text = "可以。现在 Python Agent 已经接进桌面端了，下一步就能把代码搜索、文件读取、补丁生成和测试执行做成工具。"
        candidates = []
        emotion = "focused"
    elif any(word in message for word in ("屏幕", "看一下", "截图")):
        text = "屏幕读取适合放在工具层：先截图和 OCR，再把当前窗口上下文交给 Agent。这个能力可以作为下一阶段接入。"
        candidates = []
        emotion = "thinking"
    elif any(word in message for word in ("你好", "hello", "hi")):
        text = "你好，我是 Mochi。现在我可以在没有 API Key 时本地回复；配置 OpenAI API Key 后，就会切换到真实模型。"
        candidates = []
        emotion = "happy"
    else:
        text = "收到。我现在会先保持对话、情绪和记忆链路稳定，再逐步接入工具调用、语音管线和屏幕读取。"
        candidates = []

    if memory_hint and candidates == []:
        text = f"{text}\n\n{memory_hint}"

    return {
        "text": text,
        "emotion": emotion,
        "memoryCandidates": candidates,
        "provider": "local",
    }


def call_openai(message: str, history: list[dict[str, Any]], memories: list[str]) -> dict[str, Any]:
    api_key = os.environ.get("OPENAI_API_KEY", "").strip()
    if not api_key:
        raise RuntimeError("OPENAI_API_KEY is not configured")

    model = os.environ.get("OPENAI_MODEL", DEFAULT_MODEL).strip() or DEFAULT_MODEL
    recent_history = history[-10:]
    context = {
        "recentMessages": [
            {
                "role": item.get("role", ""),
                "content": item.get("content", ""),
                "emotion": item.get("emotion", ""),
            }
            for item in recent_history
        ],
        "memories": memories[:12],
        "userMessage": message,
    }

    instructions = """
You are Mochi, a warm desktop companion and practical coding assistant.
Reply in the user's language by default.
Be concise, natural, and emotionally aware.
Return only valid JSON with this schema:
{
  "text": "assistant reply",
  "emotion": "neutral|happy|focused|thinking|sad|surprised",
  "memoryCandidates": ["short durable facts or preferences worth remembering"]
}
Only include memory candidates for stable user preferences, identity facts, project facts, or explicit remember requests.
""".strip()

    payload = {
        "model": model,
        "instructions": instructions,
        "input": json.dumps(context, ensure_ascii=False),
        "text": {
            "format": {
                "type": "json_schema",
                "name": "mochi_reply",
                "schema": {
                    "type": "object",
                    "additionalProperties": False,
                    "properties": {
                        "text": {"type": "string"},
                        "emotion": {
                            "type": "string",
                            "enum": ["neutral", "happy", "focused", "thinking", "sad", "surprised"],
                        },
                        "memoryCandidates": {
                            "type": "array",
                            "items": {"type": "string"},
                            "maxItems": 5,
                        },
                    },
                    "required": ["text", "emotion", "memoryCandidates"],
                },
                "strict": True,
            }
        },
    }

    request = urllib.request.Request(
        OPENAI_RESPONSES_URL,
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
        },
        method="POST",
    )

    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            raw = response.read().decode("utf-8")
    except urllib.error.HTTPError as error:
        detail = error.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"OpenAI API error {error.code}: {detail}") from error

    data = json.loads(raw)
    text = extract_response_text(data)
    parsed = json.loads(text)
    parsed["provider"] = "openai"
    return parsed


def extract_response_text(response: dict[str, Any]) -> str:
    if isinstance(response.get("output_text"), str):
        return response["output_text"]

    chunks: list[str] = []
    for item in response.get("output", []):
        if not isinstance(item, dict):
            continue
        for content in item.get("content", []):
            if isinstance(content, dict) and content.get("type") == "output_text":
                chunks.append(str(content.get("text", "")))

    text = "".join(chunks).strip()
    if not text:
        raise RuntimeError("OpenAI response did not contain text output")
    return text


def build_reply(message: str, history: list[dict[str, Any]], memories: list[str]) -> dict[str, Any]:
    try:
        reply = call_openai(message, history, memories)
    except Exception as error:
        reply = build_rule_reply(message, memories)
        reply["providerError"] = str(error)
    return normalize_reply(reply)


def normalize_reply(reply: dict[str, Any]) -> dict[str, Any]:
    emotion = str(reply.get("emotion", "neutral")).strip().lower()
    if emotion not in {"neutral", "happy", "focused", "thinking", "sad", "surprised"}:
        emotion = "neutral"

    candidates = reply.get("memoryCandidates", [])
    if not isinstance(candidates, list):
        candidates = []

    text = str(reply.get("text", "")).strip()
    if not text:
        text = "我刚才没有组织好回复，但我还在。你可以再说一遍，我继续处理。"

    return {
        "text": text,
        "emotion": emotion,
        "memoryCandidates": [str(item).strip() for item in candidates if str(item).strip()],
        "provider": str(reply.get("provider", "local")),
        "providerError": str(reply.get("providerError", "")),
    }


class AgentHandler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path == "/health":
            self.send_json(
                {
                    "ok": True,
                    "service": "mochi-agent",
                    "provider": "openai" if os.environ.get("OPENAI_API_KEY") else "local",
                    "model": os.environ.get("OPENAI_MODEL", DEFAULT_MODEL),
                }
            )
            return
        self.send_error(404)

    def do_POST(self) -> None:
        if self.path != "/chat":
            self.send_error(404)
            return

        length = int(self.headers.get("Content-Length", "0"))
        raw_body = self.rfile.read(length)
        try:
            payload = json.loads(raw_body.decode("utf-8"))
        except json.JSONDecodeError:
            self.send_error(400, "Invalid JSON")
            return

        message = str(payload.get("message", "")).strip()
        memories = payload.get("memories", [])
        history = payload.get("history", [])
        if not isinstance(memories, list):
            memories = []
        if not isinstance(history, list):
            history = []

        if not message:
            self.send_error(400, "Empty message")
            return

        self.send_json(build_reply(message, history, [str(item) for item in memories]))

    def send_json(self, payload: dict[str, Any]) -> None:
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args: Any) -> None:
        return


def main() -> None:
    server = ThreadingHTTPServer((HOST, PORT), AgentHandler)
    provider = "openai" if os.environ.get("OPENAI_API_KEY") else "local"
    print(f"Mochi Agent listening on http://{HOST}:{PORT} with {provider} provider", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
