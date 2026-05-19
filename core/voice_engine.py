import os
import json
import threading
import requests
import pygame
import tempfile
import ormsgpack
import speech_recognition as sr
from PySide6.QtCore import QObject, Signal as pyqtSignal

class VoiceEngine(QObject):
    speech_recognized = pyqtSignal(str)
    speech_recognition_failed = pyqtSignal()
    speech_processing = pyqtSignal() # 新增：正在识别处理中信号
    playback_started = pyqtSignal()
    playback_finished = pyqtSignal()

    def __init__(self):
        super().__init__()
        self.recognizer = sr.Recognizer()
        
        # 强制设置一个较低的固定阈值，关闭动态适应
        self.recognizer.energy_threshold = 100  # 极低阈值，确保能收到音
        self.recognizer.dynamic_energy_threshold = False # 关闭动态适应，防止环境音过大导致门槛被抬高
        self.recognizer.pause_threshold = 1.0   # 稍微增加停顿时间，防止一句话没说完就断开
        
        self.is_listening = False
        
        # 初始化 pygame mixer 用于播放音频
        pygame.mixer.init()
        self.settings = self.load_settings()

    def load_settings(self):
        settings_path = "config/settings.json"
        if os.path.exists(settings_path):
            with open(settings_path, 'r', encoding='utf-8') as f:
                return json.load(f)
        return {}

    def speak(self, text):
        """异步调用 TTS 并在后台播放"""
        def _speak():
            # 每次说话时重新加载配置，确保读取到最新的 API Key 和配置
            self.settings = self.load_settings()
            provider = self.settings.get("tts_config", {}).get("provider", "edge-tts")
            
            if provider == "fishaudio":
                self._speak_fishaudio(text)
            else:
                # 默认回退到 edge-tts
                self._speak_edge(text)
                
        threading.Thread(target=_speak, daemon=True).start()

    def _speak_fishaudio(self, text):
        api_key = self.settings.get("api_keys", {}).get("fishaudio", "")
        voice_id = self.settings.get("tts_config", {}).get("voice_id", "")
        url = self.settings.get("api_urls", {}).get("fishaudio", "https://api.fish.audio/v1/tts")

        if not api_key or not voice_id:
            print("FishAudio config missing, falling back to edge-tts.")
            self._speak_edge(text)
            return

        headers = {
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
            "model": "s2-pro"
        }
        
        payload = {
            "text": text,
            "reference_id": voice_id,
            "format": "mp3"
        }

        try:
            # 恢复为 json 格式并输出详细报错
            # 添加 verify=False 忽略 SSL 证书错误 (针对梯子或特定网络环境)
            # 添加 proxies 读取系统环境变量代理
            proxies = {
                "http": os.environ.get("HTTP_PROXY") or os.environ.get("http_proxy"),
                "https": os.environ.get("HTTPS_PROXY") or os.environ.get("https_proxy"),
            }
            
            # 清除为 None 的代理，让 requests 自己决定
            proxies = {k: v for k, v in proxies.items() if v}

            import urllib3
            urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
            
            response = requests.post(url, json=payload, headers=headers, verify=False, proxies=proxies if proxies else None)
            
            if response.status_code == 200:
                self._play_audio_data(response.content, ".mp3")
            else:
                print(f"FishAudio API Error: {response.status_code} - {response.text}")
                self._speak_edge(text)
        except Exception as e:
            print(f"FishAudio Request Failed: {e}")
            self._speak_edge(text)

    def _speak_edge(self, text):
        import edge_tts
        import asyncio
        
        async def _generate():
            voice = "zh-CN-XiaoxiaoNeural"
            communicate = edge_tts.Communicate(text, voice)
            audio_data = b""
            async for chunk in communicate.stream():
                if chunk["type"] == "audio":
                    audio_data += chunk["data"]
            self._play_audio_data(audio_data, ".mp3")
            
        asyncio.run(_generate())

    def _play_audio_data(self, audio_data, suffix):
        """将音频数据写入临时文件并使用 pygame 播放"""
        with tempfile.NamedTemporaryFile(delete=False, suffix=suffix) as temp_audio:
            temp_audio.write(audio_data)
            temp_path = temp_audio.name

        try:
            self.playback_started.emit()
            pygame.mixer.music.load(temp_path)
            pygame.mixer.music.play()
            
            # 等待播放完成
            while pygame.mixer.music.get_busy():
                pygame.time.Clock().tick(10)
                
            self.playback_finished.emit()
        finally:
            pygame.mixer.music.unload()
            try:
                os.remove(temp_path)
            except Exception as e:
                print(f"Could not remove temp audio file: {e}")

    def listen(self):
        if self.is_listening:
            return
        
        self.is_listening = True
        threading.Thread(target=self._listen_thread, daemon=True).start()

    def _listen_thread(self):
        try:
            print("Listening...")
            
            # 明确使用默认麦克风
            with sr.Microphone() as source:
                # 完全移除环境音适应，直接开始录音
                print(f"Energy threshold set to: {self.recognizer.energy_threshold}")
                
                # 开始录音，最长等待 8 秒没人说话就超时，单句最多录制 15 秒
                audio = self.recognizer.listen(source, timeout=8, phrase_time_limit=15)
            
            print("Processing audio...")
            self.speech_processing.emit()
            # 将音频存为临时文件供 STT 引擎读取
            with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as f:
                f.write(audio.get_wav_data())
                temp_audio_path = f.name
            
            text = self._transcribe(temp_audio_path)
            
            # 清理临时文件
            try:
                os.remove(temp_audio_path)
            except Exception:
                pass
                
            if text and text.strip():
                print(f"Recognized: {text}")
                self.speech_recognized.emit(text)
            else:
                print("No speech recognized.")
                self.speech_recognition_failed.emit()
                
        except sr.WaitTimeoutError:
            print("Listening timed out.")
            self.speech_recognition_failed.emit()
        except Exception as e:
            print(f"Speech Recognition Error: {e}")
            self.speech_recognition_failed.emit()
        finally:
            self.is_listening = False

    def _transcribe(self, audio_path):
        """根据配置选择本地或云端 STT 引擎进行识别"""
        stt_config = self.settings.get("stt_config", {})
        provider = stt_config.get("provider", "cloud")
        
        if provider == "local":
            return self._stt_local(audio_path, stt_config)
        else:
            return self._stt_cloud(audio_path, stt_config)
            
    def _stt_cloud(self, audio_path, stt_config):
        cloud_provider = stt_config.get("cloud_provider", "openai")
        
        if cloud_provider in ["openai", "siliconflow"]:
            from openai import OpenAI
            api_key = self.settings.get("api_keys", {}).get(cloud_provider, "")
            base_url = self.settings.get("api_urls", {}).get(cloud_provider, "")
            
            # 设置默认 base_url
            if cloud_provider == "openai" and not base_url:
                base_url = "https://api.openai.com/v1"
            elif cloud_provider == "siliconflow" and not base_url:
                base_url = "https://api.siliconflow.cn/v1"
            
            if not api_key or "your_" in api_key:
                print(f"{cloud_provider.upper()} API Key is missing or invalid for STT.")
                return ""
                
            try:
                # 使用 http_client 来自动读取系统代理环境变量，解决国内连不上的问题
                import httpx
                proxy = os.environ.get("HTTP_PROXY") or os.environ.get("http_proxy") or \
                        os.environ.get("HTTPS_PROXY") or os.environ.get("https_proxy")
                
                # 如果是硅基流动等国内节点，通常不需要走代理
                if cloud_provider == "siliconflow":
                    proxy = None
                    
                http_client = httpx.Client(proxies=proxy) if proxy else None
                
                client = OpenAI(
                    api_key=api_key, 
                    base_url=base_url,
                    http_client=http_client
                )
                
                model_name = "whisper-1"
                if cloud_provider == "siliconflow":
                    model_name = stt_config.get("siliconflow_model", "FunAudioLLM/SenseVoiceSmall")
                
                with open(audio_path, "rb") as audio_file:
                    kwargs = {
                        "model": model_name,
                        "file": audio_file
                    }
                    # 硅基流动的 SenseVoice 暂不需要强制传 language，Whisper 则可以传
                    if "whisper" in model_name.lower() or cloud_provider == "openai":
                        kwargs["language"] = "zh"
                        
                    transcript = client.audio.transcriptions.create(**kwargs)
                return transcript.text
            except Exception as e:
                print(f"Cloud STT Error ({cloud_provider}): {e}")
                return ""
        return ""
        
    def _stt_local(self, audio_path, stt_config):
        model_size = stt_config.get("local_model_size", "base")
        try:
            # 延迟加载 faster_whisper，因为模型加载较慢且占用内存大
            if not hasattr(self, 'whisper_model'):
                print(f"Loading local Whisper model ({model_size})...")
                # 屏蔽 huggingface 缓存时的 symlink 和 token 警告
                os.environ["HF_HUB_DISABLE_SYMLINKS_WARNING"] = "1"
                os.environ["HF_HUB_DISABLE_TELEMETRY"] = "1"
                
                # 读取系统可能配置的代理
                proxy = os.environ.get("HTTP_PROXY") or os.environ.get("http_proxy") or \
                        os.environ.get("HTTPS_PROXY") or os.environ.get("https_proxy")
                
                # 如果没有系统代理，强制使用国内镜像源以防止连不上外网
                if not proxy:
                    os.environ["HF_ENDPOINT"] = "https://hf-mirror.com"
                
                import warnings
                warnings.filterwarnings("ignore", category=UserWarning, module="huggingface_hub")
                
                from faster_whisper import WhisperModel
                self.whisper_model = WhisperModel(model_size, device="auto", compute_type="int8")
                
            # 修复：指定 language="zh" 可能会因为音频片段太短或全是静音报错
            # 移除强制的 language="zh"，让模型自动检测语言，增加鲁棒性
            segments, info = self.whisper_model.transcribe(audio_path, beam_size=5)
            text = "".join([segment.text for segment in segments])
            return text
        except ImportError:
            print("faster-whisper is not installed. Please run `uv add faster-whisper`")
            return ""
        except Exception as e:
            print(f"Local STT Error: {e}")
            return ""
