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
    playback_started = pyqtSignal()
    playback_finished = pyqtSignal()

    def __init__(self):
        super().__init__()
        self.recognizer = sr.Recognizer()
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
            with sr.Microphone() as source:
                self.recognizer.adjust_for_ambient_noise(source, duration=0.5)
                audio = self.recognizer.listen(source, timeout=5, phrase_time_limit=5)
                text = self.recognizer.recognize_google(audio, language='zh-CN')
                self.speech_recognized.emit(text)
        except Exception as e:
            print(f"Speech Recognition Error: {e}")
        finally:
            self.is_listening = False
