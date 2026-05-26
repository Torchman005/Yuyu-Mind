from __future__ import annotations

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any


HOST = "127.0.0.1"
PORT = 8765


def detect_emotion(message: str) -> str:
    text = message.lower()
    if any(word in message for word in ("开心", "喜欢", "太好了", "谢谢")):
        return "happy"
    if any(word in message for word in ("想", "设计", "为什么", "怎么")):
        return "thinking"
    if any(word in message for word in ("代码", "项目", "报错", "bug")) or "code" in text:
        return "focused"
    return "neutral"


def build_reply(message: str, memories: list[str]) -> dict[str, Any]:
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
        text = "你好，我是 Mochi。现在我已经不是纯 Go 里的模拟回复了，而是通过 Python Agent 服务在和你说话。"
        candidates = []
        emotion = "happy"
    else:
        text = "收到。我现在会先保持对话、情绪和记忆链路稳定，再逐步接入真实模型、工具调用和语音管线。"
        candidates = []

    if memory_hint and candidates == []:
        text = f"{text}\n\n{memory_hint}"

    return {
        "text": text,
        "emotion": emotion,
        "memoryCandidates": candidates,
    }


class AgentHandler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path == "/health":
            self.send_json({"ok": True, "service": "mochi-agent"})
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
        if not isinstance(memories, list):
            memories = []

        if not message:
            self.send_error(400, "Empty message")
            return

        self.send_json(build_reply(message, [str(item) for item in memories]))

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
    print(f"Mochi Agent listening on http://{HOST}:{PORT}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
