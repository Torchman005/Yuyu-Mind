from __future__ import annotations

import json
import os
import socket
import urllib.error
import urllib.request
from dataclasses import dataclass
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any


HOST = "127.0.0.1"
PORT = 8765
DEFAULT_PROVIDER = "deepseek"


@dataclass(frozen=True)
class ProviderConfig:
    name: str
    api_key_env: str
    model_env: str
    base_url_env: str
    default_model: str
    default_base_url: str
    mode: str


PROVIDERS: dict[str, ProviderConfig] = {
    "deepseek": ProviderConfig(
        name="deepseek",
        api_key_env="DEEPSEEK_API_KEY",
        model_env="DEEPSEEK_MODEL",
        base_url_env="DEEPSEEK_BASE_URL",
        default_model="deepseek-chat",
        default_base_url="https://api.deepseek.com",
        mode="chat_completions",
    ),
    "openai": ProviderConfig(
        name="openai",
        api_key_env="OPENAI_API_KEY",
        model_env="OPENAI_MODEL",
        base_url_env="OPENAI_BASE_URL",
        default_model="gpt-5-mini",
        default_base_url="https://api.openai.com/v1",
        mode="responses",
    ),
    "openrouter": ProviderConfig(
        name="openrouter",
        api_key_env="OPENROUTER_API_KEY",
        model_env="OPENROUTER_MODEL",
        base_url_env="OPENROUTER_BASE_URL",
        default_model="deepseek/deepseek-chat",
        default_base_url="https://openrouter.ai/api/v1",
        mode="chat_completions",
    ),
    "ollama": ProviderConfig(
        name="ollama",
        api_key_env="OLLAMA_API_KEY",
        model_env="OLLAMA_MODEL",
        base_url_env="OLLAMA_BASE_URL",
        default_model="qwen2.5:7b",
        default_base_url="http://127.0.0.1:11434/v1",
        mode="chat_completions",
    ),
}


def load_dotenv() -> None:
    env_path = Path(__file__).resolve().parents[1] / ".env"
    if not env_path.exists():
        return

    for raw_line in env_path.read_text(encoding="utf-8-sig").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip().strip('"').strip("'")
        if key:
            os.environ[key] = value


load_dotenv()


def get_provider_name() -> str:
    return os.environ.get("LLM_PROVIDER", DEFAULT_PROVIDER).strip().lower() or DEFAULT_PROVIDER


def get_provider() -> ProviderConfig:
    provider_name = get_provider_name()
    if provider_name not in PROVIDERS:
        raise RuntimeError(f"Unsupported LLM_PROVIDER: {provider_name}")
    return PROVIDERS[provider_name]


def current_model() -> str:
    provider = get_provider()
    return os.environ.get(provider.model_env, provider.default_model).strip() or provider.default_model


def current_base_url() -> str:
    provider = get_provider()
    return os.environ.get(provider.base_url_env, provider.default_base_url).strip().rstrip("/") or provider.default_base_url


def current_api_key() -> str:
    provider = get_provider()
    return os.environ.get(provider.api_key_env, "").strip()


def reply_language() -> str:
    return os.environ.get("MOCHI_REPLY_LANGUAGE", "zh_ja").strip().lower() or "zh_ja"


def desktop_pet_name() -> str:
    return os.environ.get("MOCHI_DESKTOP_PET_NAME", "Mochi").strip() or "Mochi"


def persona_prompt() -> str:
    name = desktop_pet_name()
    default_persona = (
        f"{name} is a cute anime-style desktop companion. "
        "She is warm, playful, slightly shy, loyal, and practical. "
        "She talks like a gentle Japanese anime assistant, with natural charm but not exaggerated roleplay. "
        "She can help with coding and desktop tasks while keeping a soft companion tone."
    )
    return os.environ.get("MOCHI_PERSONA", default_persona).strip() or default_persona


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
    name = desktop_pet_name()
    emotion = detect_emotion(message)
    dual_language = reply_language() in {"zh_ja", "zh-ja", "dual", "bilingual"}
    use_japanese = reply_language().startswith("ja")
    memory_hint = f"\n\n我还记得你之前提到过：{memories[0]}" if memories else ""
    speech_memory_hint = f"\n\n前に話してくれたことも、ちゃんと覚えてるよ：{memories[0]}" if memories else ""

    if any(word in message for word in ("记住", "喜欢")):
        text = "好，我会把这条作为长期记忆候选保存下来。后面会帮你区分偏好、事实和项目状态。"
        speech_text = "うん、覚えておくね。大事なこととして、あとでちゃんと整理できるようにしておくよ。"
        candidates = [message]
        emotion = "happy"
    elif any(word in message for word in ("代码", "项目", "报错")):
        text = "可以。代码相关问题我会先帮你稳定分析，后面可以继续接入文件读取、补丁生成和测试执行。"
        speech_text = "任せて。コードのことなら、落ち着いて一緒に見ていこうね。必要なら原因も順番に探せるよ。"
        candidates = []
        emotion = "focused"
    elif any(word in message for word in ("屏幕", "看一下", "截图")):
        text = "屏幕读取可以作为下一阶段能力：先截图和 OCR，再把当前窗口上下文交给 Agent。"
        speech_text = "画面を見られるようにすると、もっとそばで手伝えるね。スクリーン読み取りは次の機能としてつなげられるよ。"
        candidates = []
        emotion = "thinking"
    elif any(word in message for word in ("你好", "hello", "hi")):
        text = f"你好，我是 {name}。今天也会在桌面上陪你一起处理事情。"
        speech_text = f"こんにちは、{name}だよ。今日もそばで、ちゃんとお手伝いするね。"
        candidates = []
        emotion = "happy"
    else:
        text = "收到。我会先保持对话、情绪和记忆链路稳定，再逐步接入更多助手能力。"
        speech_text = "うん、わかったよ。少しずつ一緒に整えていこうね。"
        candidates = []

    if use_japanese and not dual_language:
        text = speech_text
        memory_hint = speech_memory_hint

    return {
        "text": text + (memory_hint if not candidates else ""),
        "speechText": speech_text + (speech_memory_hint if not candidates else ""),
        "emotion": emotion,
        "memoryCandidates": candidates,
        "provider": "local",
    }


def build_context(message: str, history: list[dict[str, Any]], memories: list[str]) -> dict[str, Any]:
    recent_history = history[-10:]
    return {
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
        "replyLanguage": reply_language(),
    }


def build_system_prompt() -> str:
    language = reply_language()
    name = desktop_pet_name()
    if language in {"zh_ja", "zh-ja", "dual", "bilingual"}:
        language_instruction = (
            "Return Chinese text for UI display in the `text` field. "
            "Return natural Japanese text for voice synthesis in the `speechText` field. "
            "`speechText` should carry the same meaning as `text`, but it must be Japanese and suitable for TTS."
        )
    elif language.startswith("ja"):
        language_instruction = (
            "Reply in natural Japanese in both `text` and `speechText`, regardless of the user's input language. "
            "Keep the text suitable for Japanese TTS."
        )
    else:
        language_instruction = "Reply in the user's language in `text`; provide a natural TTS version in `speechText`."
    return f"""
You are {name}.
Persona:
{persona_prompt()}

{language_instruction}
Your name is exactly "{name}". If the user asks your name, say you are "{name}", not Mochi or any other name.
Be concise, natural, cute, and emotionally aware.
Avoid heavy honorific overacting; sound like a believable anime desktop companion.
The `text` field must follow the reply language instruction.
Return only valid JSON with this schema:
{{
  "text": "Chinese UI reply or assistant reply",
  "speechText": "Japanese TTS reply",
  "emotion": "neutral|happy|focused|thinking|sad|surprised",
  "memoryCandidates": ["short durable facts or preferences worth remembering"]
}}
Only include memory candidates for stable user preferences, identity facts, project facts, or explicit remember requests.
""".strip()


def call_llm(message: str, history: list[dict[str, Any]], memories: list[str]) -> dict[str, Any]:
    load_dotenv()
    provider = get_provider()
    if provider.mode == "responses":
        return call_responses_api(provider, message, history, memories)
    return call_chat_completions(provider, message, history, memories)


def call_chat_completions(
    provider: ProviderConfig,
    message: str,
    history: list[dict[str, Any]],
    memories: list[str],
) -> dict[str, Any]:
    api_key = current_api_key()
    if provider.name != "ollama" and not api_key:
        raise RuntimeError(f"{provider.api_key_env} is not configured")

    url = f"{current_base_url()}/chat/completions"
    context = build_context(message, history, memories)
    payload = {
        "model": current_model(),
        "messages": [
            {"role": "system", "content": build_system_prompt()},
            {"role": "user", "content": json.dumps(context, ensure_ascii=False)},
        ],
        "response_format": {"type": "json_object"},
        "stream": False,
    }

    if provider.name == "deepseek":
        payload["thinking"] = {"type": os.environ.get("DEEPSEEK_THINKING", "disabled").strip() or "disabled"}

    raw = post_json(url, api_key, payload, f"{provider.name} API")
    data = json.loads(raw)
    content = data["choices"][0]["message"]["content"]
    parsed = parse_json_text(content)
    parsed["provider"] = provider.name
    return parsed


def call_responses_api(
    provider: ProviderConfig,
    message: str,
    history: list[dict[str, Any]],
    memories: list[str],
) -> dict[str, Any]:
    api_key = current_api_key()
    if not api_key:
        raise RuntimeError(f"{provider.api_key_env} is not configured")

    url = f"{current_base_url()}/responses"
    context = build_context(message, history, memories)
    payload = {
        "model": current_model(),
        "instructions": build_system_prompt(),
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
                        "speechText": {"type": "string"},
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
                    "required": ["text", "speechText", "emotion", "memoryCandidates"],
                },
                "strict": True,
            }
        },
    }

    raw = post_json(url, api_key, payload, f"{provider.name} API")
    data = json.loads(raw)
    text = extract_response_text(data)
    parsed = parse_json_text(text)
    parsed["provider"] = provider.name
    return parsed


def post_json(url: str, api_key: str, payload: dict[str, Any], label: str) -> str:
    headers = {"Content-Type": "application/json"}
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"

    request = urllib.request.Request(
        url,
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers=headers,
        method="POST",
    )

    try:
        with urllib.request.urlopen(request, timeout=45) as response:
            return response.read().decode("utf-8")
    except urllib.error.HTTPError as error:
        detail = error.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"{label} error {error.code}: {detail}") from error


def parse_json_text(text: str) -> dict[str, Any]:
    cleaned = text.strip()
    if cleaned.startswith("```"):
        cleaned = cleaned.strip("`").strip()
        if cleaned.startswith("json"):
            cleaned = cleaned[4:].strip()
    return json.loads(cleaned)


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
        raise RuntimeError("Response did not contain text output")
    return text


def build_reply(message: str, history: list[dict[str, Any]], memories: list[str]) -> dict[str, Any]:
    try:
        reply = call_llm(message, history, memories)
    except Exception as error:
        reply = build_provider_error_reply(error)
        reply["providerError"] = str(error)
    return normalize_reply(reply)


def build_provider_error_reply(error: Exception) -> dict[str, Any]:
    provider = get_provider_name()
    model = current_model()
    detail = str(error).strip()
    if len(detail) > 180:
        detail = detail[:180] + "..."

    if reply_language() in {"zh_ja", "zh-ja", "dual", "bilingual"}:
        return {
            "text": f"现在不是我只会固定回复，而是 {provider} 的 {model} 调用失败了。请检查模型名、余额、API Key 或网络代理。错误：{detail}",
            "speechText": "今は固定返答しかできないわけじゃなくて、LLMの呼び出しが失敗しているみたい。モデル名、残高、APIキー、ネットワーク設定を確認してね。",
            "emotion": "thinking",
            "memoryCandidates": [],
            "provider": provider,
        }

    if reply_language().startswith("ja"):
        text = f"今は固定返答しかできないわけじゃなくて、{provider} の {model} 呼び出しが失敗しているみたい。モデル名、残高、APIキー、ネットワーク設定を確認してね。"
    else:
        text = f"现在不是我只会固定回复，而是 {provider} 的 {model} 调用失败了。请检查模型名、余额、API Key 或网络代理。错误：{detail}"

    return {
        "text": text,
        "speechText": text,
        "emotion": "thinking",
        "memoryCandidates": [],
        "provider": provider,
    }


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

    speech_text = str(reply.get("speechText", "")).strip()
    if not speech_text:
        speech_text = text

    return {
        "text": text,
        "speechText": speech_text,
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
                    "provider": get_provider_name(),
                    "model": current_model(),
                    "baseUrl": current_base_url(),
                    "capabilities": ["chat", "speechText"],
                    "supportedProviders": sorted(PROVIDERS.keys()),
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
        try:
            self.send_response(200)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        except (BrokenPipeError, ConnectionAbortedError, ConnectionResetError, socket.timeout):
            return

    def log_message(self, format: str, *args: Any) -> None:
        return


def main() -> None:
    server = ThreadingHTTPServer((HOST, PORT), AgentHandler)
    print(f"Mochi Agent listening on http://{HOST}:{PORT} with {get_provider_name()} provider", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
