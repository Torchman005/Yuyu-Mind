# GPT-SoVITS TTS Setup

Yuyu-Mind can use a local or self-hosted GPT-SoVITS API server for desktop pet speech.

## Yuyu-Mind Configuration

Set the backend TTS provider in `.env`:

```env
TTS_PROVIDER=gpt-sovits
TTS_TIMEOUT_SECONDS=12

GPT_SOVITS_URL=http://127.0.0.1:9880/tts
GPT_SOVITS_API_STYLE=v2
GPT_SOVITS_MEDIA_TYPE=wav
GPT_SOVITS_TEXT_LANG=zh
GPT_SOVITS_REF_AUDIO_PATH=D:\voices\yuyu_ref.wav
GPT_SOVITS_PROMPT_TEXT=这里填写参考音频里真实说出的原文
GPT_SOVITS_PROMPT_LANG=zh
GPT_SOVITS_TEXT_SPLIT_METHOD=cut5
GPT_SOVITS_SPEED_FACTOR=1.0
GPT_SOVITS_STREAMING_MODE=false
```

The frontend should keep cloud playback enabled:

```env
VITE_SPEECH_OUTPUT_MODE=cloud
VITE_ALLOW_SYSTEM_TTS_FALLBACK=false
VITE_ENABLE_STREAMING_TTS=false
```

`GPT_SOVITS_API_STYLE=v2` uses the official `api_v2.py` JSON `/tts` endpoint. If you run an older GPT-SoVITS API that only accepts query parameters, set:

```env
GPT_SOVITS_API_STYLE=legacy
```

## Reference Audio

For zero-shot voice cloning, prepare a clean reference clip:

- Use mono or stereo WAV if possible.
- Keep it around 5-15 seconds for quick tests.
- Avoid music, reverb, noise, and overlapping voices.
- `GPT_SOVITS_PROMPT_TEXT` must exactly match what is spoken in the reference clip.
- `GPT_SOVITS_PROMPT_LANG` should match the reference language, for example `zh`, `ja`, or `en`.
- `GPT_SOVITS_TEXT_LANG` should match Yuyu's speech text language.

## Useful Parameters

- `GPT_SOVITS_MEDIA_TYPE=wav` is safest for local playback.
- `GPT_SOVITS_MEDIA_TYPE=mp3` can reduce payload size if your server supports it.
- `GPT_SOVITS_SPEED_FACTOR=1.0` controls speaking speed.
- `GPT_SOVITS_SAMPLE_STEPS=32` is a balanced default.
- `GPT_SOVITS_STREAMING_MODE=false` avoids chunked audio in Yuyu-Mind's current buffered playback path.
- `GPT_SOVITS_STREAMING_MODE=3` is faster on GPT-SoVITS API v2, but Yuyu-Mind currently still plays the received response as a complete audio file.

## Start GPT-SoVITS API

From the GPT-SoVITS repository, start the v2 API server:

```powershell
python api_v2.py -a 127.0.0.1 -p 9880 -c GPT_SoVITS/configs/tts_infer.yaml
```

Then restart Yuyu-Mind so it reloads `.env`.

## Quick API Smoke Test

After starting GPT-SoVITS, test it directly:

```powershell
Invoke-WebRequest `
  -Method Post `
  -Uri http://127.0.0.1:9880/tts `
  -ContentType 'application/json' `
  -Body '{"text":"你好，我是悠悠。","text_lang":"zh","ref_audio_path":"D:\\voices\\yuyu_ref.wav","prompt_text":"这里填写参考音频里真实说出的原文","prompt_lang":"zh","media_type":"wav","streaming_mode":false}' `
  -OutFile test.wav
```

If `test.wav` plays correctly, Yuyu-Mind should be able to use the same configuration.
