# GPT-SoVITS 音色复刻 + 接入指南

> 目标：用你的语音复刻一个音色，让 Yuyu 桌宠用**本地 GPT-SoVITS** 逐句合成自然语音。
> 你的显卡 RTX 4060 Ti（8G/16G）对「推理」绰绰有余，对「训练」也够用（半精度）。

分两阶段：**A. 音色复刻（训练）** 和 **B. 接入 Yuyu（合成）**。合成（B）只需加载训练好的权重，吃显存很少。

---

## A. 音色复刻（训练）

### 1. 安装 GPT-SoVITS

任选其一：

- **Windows 整合包**（推荐，免配环境）：到 [RVC-Boss/GPT-SoVITS](https://github.com/RVC-Boss/GPT-SoVITS) 的 Release 下载「整合包」，解压即用。
- **源码安装**：`git clone` 后按 README 用 conda 建环境（PyTorch + CUDA，装对应 4060 Ti 的 CUDA 版本）。

### 2. 准备你的语音数据

你已备好语音，只需满足：

| 要求 | 说明 |
| --- | --- |
| 单说话人 | 只保留「目标音色」一个人的声音 |
| 干净 | 无背景音乐/多人声/明显噪声；有杂音先过一遍 UVR5 降噪 |
| 时长 | 建议 **1~5 分钟**（几十秒也能出效果，越长越像、越稳） |
| 格式 | WAV，16k/32k 均可，单声道 |
| 内容 | 尽量自然口语，语速稳定，少极端情绪 |

### 3. 训练流程（WebUI）

启动 `GPT-SoVITS-Train` 的 WebUI，按序号走：

1. **数据集格式化 / 切分**：把你的长音频切成 3~10 秒的短句。
2. **UVR5 语音降噪**（可选，但推荐）：分离人声与伴奏/噪声。
3. **ASR 打标**：自动转写每段音频，生成 `.list` 训练清单（记得人工抽查纠正错字）。
4. **训练 GPT 模型**：`SoVITS 训练` 前的 `GPT 训练`，batch size 按显存调（4060 Ti 8G 用 2~4 起步，16G 可用更大）。
5. **训练 SoVITS 模型**：同样的数据接着训 SoVITS。

> 训练完成后你会得到两组权重：`GPT_weights/xxx.ckpt` 和 `SoVITS_weights/xxx.pth`。

### 4. 准备「参考音频」（推理用）

训练完还需要一段 **3~10 秒的干净参考音频**（音色锚点）：

- 从你数据里挑一句最干净、最能代表目标音色的短句。
- 记下它对应的**文本**（`prompt_text`），后面配置要用。

---

## B. 接入 Yuyu（合成）

### 5. 启动 GPT-SoVITS 的 API 服务

用 WebUI 的「语音合成」页，或直接跑 API 脚本（参数以你安装版本的 README 为准，大致如下）：

```bash
python api_v2.py -a 127.0.0.1 -p 9880 \
  -g GPT_weights/你的模型.ckpt \
  -s SoVITS_weights/你的模型.pth \
  -dr 参考音频.wav -dt "参考音频说的文本" -dl zh
```

默认端口 `9880`，端点 `/tts`。

### 6. 配置 Yuyu

编辑 `config.json`（默认在 `%APPDATA%\Yuyu-Mind\config.json`，或 `YUYU_CONFIG_DIR` 指向的目录）：

```jsonc
{
  "speech": {
    "provider": "gpt_sovits",          // 关键：从 fish_audio 切到 gpt_sovits
    "gpt_sovits": {
      "base_url": "http://127.0.0.1:9880",
      "endpoint": "/tts",              // api_v2 用 /tts；老版 api.py 可设 "/"
      "refer_audio_path": "D:/voices/yuyu_ref.wav",  // 参考音频绝对路径
      "prompt_text": "参考音频说的那句话",
      "prompt_lang": "zh",             // 参考文本语言：auto/zh/ja/en
      "text_lang": "zh"                // 合成文本语言
    }
  }
}
```

### 7. 重启并验证

```powershell
cd frontend; npm run build; cd ..
wails dev
```

聊天时看前端 `Speech timing`（若开启 `VITE_SHOW_SPEECH_DEBUG=true`）应显示 provider 为 `GPT-SoVITS`，且逐句快速出声。

---

## 常见问题

- **`GPT-SoVITS base url 未配置` / `参考音频路径未配置`**：`config.json` 里 `speech.provider` 没切到 `gpt_sovits`，或 `gpt_sovits` 字段没填全。
- **返回 4xx/5xx**：端点或字段名和你的 API 版本不匹配——`api.py`（老版）端点一般是 `/` 且用表单，`api_v2.py` 用 `/tts` 且 JSON。先确认 `endpoint`，必要时看 `api_v2.py`/`api.py` 源码里的入参字段。
- **音色不像**：训练数据时长/质量不够，或参考音频不干净；加数据、加长时长、用 UVR5 降噪后重训。
- **合成慢**：确认跑在 GPU 上（api 启动日志看是否加载到 cuda），并检查是否用了半精度。

> 说明：本项目的 `internal/app/gpt_sovits.go` 已实现 POST JSON → `/tts`（可配 `endpoint`），并兼容两种响应格式——`api_v2.py` 的 `{"data":[{"audio":"base64"}]}` 与 `api.py` 直接返回 WAV 字节。若你的版本入参字段名不同，改 `gpt_sovits.go` 里的 `gptSovitsRequest` 字段即可。
