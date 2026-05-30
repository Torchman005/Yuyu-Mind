from __future__ import annotations

import base64
import json
import os
import socket
import sys
from pathlib import Path


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


def local_port_open(port: int) -> bool:
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=0.2):
            return True
    except OSError:
        return False


def proxy_candidates() -> list[str | None]:
    values: list[str | None] = []
    explicit = os.environ.get("FISH_AUDIO_PROXY", "").strip()
    if explicit.lower() in {"direct", "none", "off", "false", "0"}:
        values.append(None)
    elif explicit:
        values.append(explicit)

    env_proxy = (
        os.environ.get("HTTPS_PROXY")
        or os.environ.get("https_proxy")
        or os.environ.get("HTTP_PROXY")
        or os.environ.get("http_proxy")
    )
    if env_proxy:
        values.append(env_proxy)

    auto_probe = os.environ.get("FISH_AUDIO_AUTO_PROXY", "").strip().lower()
    if auto_probe in {"1", "true", "yes", "on"}:
        for port in (7890, 7897, 7899, 10809, 10808, 1080, 20171, 2080, 8080, 8118):
            if local_port_open(port):
                values.append(f"http://127.0.0.1:{port}")

    values.append(None)

    seen: set[str] = set()
    unique: list[str | None] = []
    for value in values:
        marker = value or "direct"
        if marker in seen:
            continue
        seen.add(marker)
        unique.append(value)
    return unique


def proxy_map(proxy: str | None) -> dict[str, str] | None:
    if not proxy:
        return None
    return {"http": proxy, "https": proxy}


def main() -> int:
    load_dotenv()
    text = sys.stdin.read().strip()
    if not text:
        raise RuntimeError("speech text cannot be empty")

    api_key = os.environ.get("FISH_AUDIO_API_KEY", "").strip()
    reference_id = os.environ.get("FISH_AUDIO_REFERENCE_ID", "").strip()
    url = os.environ.get("FISH_AUDIO_TTS_URL", "https://api.fish.audio/v1/tts").strip()
    model = os.environ.get("FISH_AUDIO_MODEL", "s2-pro").strip() or "s2-pro"

    if not api_key:
        raise RuntimeError("FISH_AUDIO_API_KEY is not configured")
    if not reference_id:
        raise RuntimeError("FISH_AUDIO_REFERENCE_ID is not configured")

    import requests
    import urllib3

    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
        "model": model,
    }
    payload = {
        "text": text,
        "reference_id": reference_id,
        "format": "mp3",
    }
    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
    errors: list[str] = []
    response = None
    for proxy in proxy_candidates():
        label = proxy or "direct"
        try:
            response = requests.post(
                url,
                json=payload,
                headers=headers,
                verify=False,
                proxies=proxy_map(proxy),
                timeout=60,
            )
            if response.status_code == 200:
                break
            errors.append(f"{label}: HTTP {response.status_code} {response.text[:300]}")
        except Exception as error:
            errors.append(f"{label}: {error}")
            response = None

    if response is None or response.status_code != 200:
        raise RuntimeError("Fish Audio request failed via all routes: " + " | ".join(errors))

    result = {
        "audioBase64": base64.b64encode(response.content).decode("ascii"),
        "contentType": response.headers.get("Content-Type") or "audio/mpeg",
        "provider": "fish-audio-python-script",
    }
    sys.stdout.write(json.dumps(result, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as error:
        sys.stderr.write(str(error))
        raise SystemExit(1)
