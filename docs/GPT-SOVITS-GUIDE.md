# GPT-SoVITS 音色复刻 + 接入指南（针对本机 v2Pro 整合包）

> 你的安装：`D:\itJinYu_toolkit\GPT-SoVITS-v2pro-20250604\GPT-SoVITS-v2pro-20250604`（内置 `runtime\python.exe` 便携环境 + ffmpeg，无需 conda）。
> 你的显卡：RTX 4060 Ti。目标音色：智乃（Chino）。
> 训练素材：`E:\Yuyu-Mind训练素材\智乃素材\语音\FIkxSkHc.mp3`（约 3MB ≈ 3 分钟）。

已确认的版本要点（与通用 GPT-SoVITS 略有差异，务必按此）：

- API 用 `api_v2.py`，**端点 `/tts`**，**非流式直接返回 WAV 字节**（`audio/wav`），出错返回 JSON 400。
- `api_v2.py` 的 CLI 只有 `-a`（地址）/`-p`（端口）/`-c`（配置文件）；**权重在 `tts_infer.yaml` 里配，不是命令行传 `-g/-s`**。
- 语言字段支持 `auto`（以及 `zh`/`ja`/`en`/`yue`/`ko` 等）。
- **默认 `device: cpu`**！必须改 `cuda` 才能用上你的 4060 Ti。

---

## 路线选择

| 路线 | 要不要训练 | 效果 | 建议 |
| --- | --- | --- | --- |
| **A. 零样本克隆**（先跑通） | 不用 | 3~10 秒参考音频即可，像但细节略糙 | **先做这个**，10 分钟跑通 |
| **B. 微调训练**（最终目标） | 要 | 用你 3 分钟素材训练，更像更稳 | 跑通 A 后再做 |

---

## 第 0 步：把 GPU 打开（必做，否则合成慢）

编辑 `GPT_SoVITS\configs\tts_infer.yaml`，把用到的版本段里的：

```yaml
device: cpu
is_half: false
```

改成：

```yaml
device: cuda
is_half: true
```

> 建议先只用 `v2` 段（或 `custom`，它默认 version: v2，指向 `gsv-v2final-pretrained` 预训练权重）。零样本用 v2 就够。

---

## 路线 A：零样本克隆（快速跑通）

### 1. 从你的 MP3 里截一段「参考音频」

用整合包自带的 ffmpeg（`runtime\ffmpeg.exe`），选一句最干净、最能代表智乃音色的 5~10 秒：

```powershell
cd D:\itJinYu_toolkit\GPT-SoVITS-v2pro-20250604\GPT-SoVITS-v2pro-20250604

# 转成单声道 32k WAV（参考音频 + 后续训练都用它）
runtime\ffmpeg.exe -y -i "E:\Yuyu-Mind训练素材\智乃素材\语音\FIkxSkHc.mp3" -ar 32000 -ac 1 -c:a pcm_s16le "E:\Yuyu-Mind训练素材\智乃素材\语音\chino_full.wav"

# 截 10 秒参考音频（时间点按你素材里干净的那句改）
runtime\ffmpeg.exe -y -ss 00:00:05 -t 10 -i "E:\Yuyu-Mind训练素材\智乃素材\语音\chino_full.wav" -ar 32000 -ac 1 -c:a pcm_s16le "E:\Yuyu-Mind训练素材\智乃素材\语音\chino_ref.wav"
```

记下这段参考音频说的**原文**（`prompt_text`）。

### 2. 启动 API

```powershell
cd D:\itJinYu_toolkit\GPT-SoVITS-v2pro-20250604\GPT-SoVITS-v2pro-20250604
runtime\python.exe api_v2.py -a 127.0.0.1 -p 9880 -c GPT_SoVITS/configs/tts_infer.yaml
```

看到 `Uvicorn running on http://127.0.0.1:9880` 即成功（注意日志里是否加载到 cuda）。

### 3. 配置 Yuyu

`config.json`（`%APPDATA%\Yuyu-Mind\config.json`）用正斜杠写路径：

```jsonc
"speech": {
  "provider": "gpt_sovits",
  "gpt_sovits": {
    "base_url": "http://127.0.0.1:9880",
    "endpoint": "/tts",
    "refer_audio_path": "E:/Yuyu-Mind训练素材/智乃素材/语音/chino_ref.wav",
    "prompt_text": "（参考音频说的那句话）",
    "prompt_lang": "zh",
    "text_lang": "zh"
  }
}
```

### 4. 验证

`npm run build` + `wails dev`，聊天应逐句走 GPT-SoVITS（provider 显示 `GPT-SoVITS`）。若不满意音色相似度 → 走路线 B。

---

## 路线 B：微调训练（高保真音色复刻）

用 WebUI 训练（入口 `go-webui.bat`，内置 Python）：

1. **切分**：把 `chino_full.wav` 切成长短句（WebUI「1-GPT-SoVITS-TTS」页 → 语音切分工具，或直接拖进去）。
2. **降噪**（推荐）：UVR5 人声分离，去掉背景音乐/噪声。
3. **ASR 打标**：自动转写每段，生成 `.list`（**人工抽查纠正错字**，智乃是日语就选 ja）。
4. **训练 GPT 模型**：选 v2 架构，batch 按 4060 Ti 调（8G 版 2~4，16G 版更大）。
5. **训练 SoVITS 模型**。
6. 训练完得到 `GPT_weights/你的模型.ckpt` + `SoVITS_weights/你的模型.pth`。

### 用训练好的权重推理

把 `tts_infer.yaml` 里对应版本段的 `t2s_weights_path` / `vits_weights_path` 改成你的权重路径，重启 `api_v2.py` 即可；其余步骤同路线 A。

---

## 常见问题

- **合成很慢**：`tts_infer.yaml` 的 `device` 没改成 `cuda`，跑在 CPU 上了。
- **返回 400 / `text_lang is not supported`**：语言字段写错（用 `zh`/`ja`/`auto`）。
- **`ref_audio_path is required`**：参考音频路径写错或为空；用绝对路径 + 正斜杠。
- **音色不像**：参考音频不干净 / 训练素材太少。换更干净的参考句，或走微调训练加时长。
- **api_v2 字段名**：本机已核对（`text`/`text_lang`/`ref_audio_path`/`prompt_text`/`prompt_lang`/`top_k`/`top_p`/`temperature`/`speed_factor`/`text_split_method`/`media_type`/`streaming_mode`），与 `internal/app/gpt_sovits.go` 的 `gptSovitsRequest` 一致。
