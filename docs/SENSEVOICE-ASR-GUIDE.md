# SenseVoice（FunASR）本地语音识别接入指南

> 目标：用**本地 SenseVoice** 替代浏览器 Web Speech，解决中文/日语识别不准的问题。
> SenseVoice 优势：中文识别极准、日语也好、**自动识别语言**（不用手动切 zh/ja）、本地 GPU、免费。
> 你的显卡 RTX 4060 Ti 完全够用。

---

## 1. 安装 FunASR（含 OpenAI 兼容服务）

FunASR 官方提供了 OpenAI 兼容的 serving（`funasr-server`），接口和 OpenAI `/audio/transcriptions` 一致，所以 Yuyu 后端已能直接对接。

```powershell
# 建议用独立 conda/venv 环境（避免和 GPT-SoVITS 的环境冲突）
conda create -n funasr python=3.10 -y
conda activate funasr

pip install "funasr[server]" torch torchaudio
# 若显卡是 4060 Ti，装 CUDA 版 torch（按你环境选 cu118/cu121/cu124）
# pip install torch==2.1.0+cu118 torchaudio==2.1.0+cu118 -f https://download.pytorch.org/whl/torch_stable.html
```

> 参考官方 OpenAI 兼容示例：[FunASR examples/openai_api](https://github.com/modelscope/FunASR/blob/main/examples/openai_api/README_zh.md)。

## 2. 启动 SenseVoice 服务

```powershell
python -m funasr_server --model iic/SenseVoiceSmall --port 10095
# 或按官方示例的 server.py 启动（命令因版本略有差异，以官方 README 为准）
```

看到监听 `http://127.0.0.1:10095` 即成功。首次会自动下载 SenseVoiceSmall 模型（约 1GB）。

## 3. 配置 Yuyu

编辑 `config.json`（`%APPDATA%\Yuyu-Mind\config.json`）加 `asr` 段：

```jsonc
"asr": {
  "base_url": "http://127.0.0.1:10095/v1",   // 若端点是 /audio/transcriptions 则不带 /v1
  "api_key": "",                              // 本地服务无需 key
  "model": "iic/SenseVoiceSmall"
}
```

## 4. 让前端走模型 ASR（构建期环境变量）

```powershell
cd frontend
$env:VITE_ASR_PROVIDER="model"      # 从浏览器识别切到模型识别
$env:VITE_ASR_LANGUAGE="auto"       # SenseVoice 自动识别语言，不强制 zh/ja
npm run build
cd ..
wails dev
```

---

## 常见问题

- **`ASR 模型未配置`**：`config.json` 没写 `asr.model`，或没设 `VITE_ASR_PROVIDER=model` 重新 build。
- **连接被拒**：SenseVoice 服务没启动，或 `base_url` 端口/路径不对。
- **识别还是不准**：确认跑在 GPU（启动日志看是否加载 cuda）；确认麦克风输入清晰；`VITE_ASR_LANGUAGE=auto` 让模型自动判断语言。
- **端点不确定**：FunASR OpenAI 兼容服务的端点可能是 `/v1/audio/transcriptions` 或 `/audio/transcriptions`。看启动日志里打印的路由，`base_url` 里带上或去掉 `/v1` 即可（后端是 `base_url + /audio/transcriptions` 拼接）。

> 后端说明：`internal/app/companion.go` 的 `TranscribeAudio` 已优先用 `asr.base_url/api_key/model`（留空才回退激活 LLM Provider），`internal/ai/asr` 走 OpenAI 兼容 multipart + `{"text":...}` 解析，与 FunASR 的 OpenAI 兼容服务对接无需改代码。
