import sys
import os
import json
from PySide6.QtWidgets import QApplication
from PySide6.QtCore import QTimer
from dotenv import load_dotenv

# 加载环境变量
load_dotenv()

from core.pet_window import PetWindow
from core.skin_manager import SkinManager
from core.ai_chat import AIChat
from core.voice_engine import VoiceEngine
from core.animation import AnimationController
from utils.logger import logger

def load_settings():
    settings_path = "config/settings.json"
    if os.path.exists(settings_path):
        with open(settings_path, 'r', encoding='utf-8') as f:
            return json.load(f)
    return {}

def main():
    logger.info("Mochi AI Starting...")
    app = QApplication(sys.argv)
    
    # 确保退出时清理
    app.setQuitOnLastWindowClosed(False)
    
    settings = load_settings()
    
    # 初始化组件
    skin_manager = SkinManager()
    ai_chat = AIChat()
    voice_engine = VoiceEngine()
    anim_controller = AnimationController(interval=settings.get("random_action_interval", 30) * 1000)
    
    # 创建主窗口
    pet = PetWindow(skin_manager)
    pet.show()
    
    # 绑定 AI 响应
    # ai_chat 现在返回一个字典: {"zh": "中文文本", "jp": "日文文本"}
    ai_chat.response_ready.connect(lambda response_dict: (
        pet.say(response_dict.get("zh", "")),
        voice_engine.speak(response_dict.get("jp", "")) if settings.get("voice_enabled") else None,
        logger.info(f"AI Response (ZH): {response_dict.get('zh')}, (JP): {response_dict.get('jp')}")
    ))
    
    # 绑定语音播放与动作同步
    voice_engine.playback_started.connect(lambda: pet.set_state("speaking"))
    voice_engine.playback_finished.connect(pet._on_say_finished)
    
    # 连接文字聊天
    # 在发送聊天请求时，让 ai_chat 优先使用人设中的名字
    def on_chat_requested(text):
        ai_chat.ask(text)
        logger.info(f"User Chat: {text}")
        
    pet.chat_requested.connect(on_chat_requested)
    pet.voice_requested.connect(lambda: (
        pet.say("正在听..."),
        voice_engine.listen()
    ))
    
    # 绑定语音识别
    voice_engine.speech_recognized.connect(lambda text: (
        ai_chat.ask(text),
        logger.info(f"User Voice: {text}")
    ))
    
    # 绑定动画控制器
    anim_controller.state_changed.connect(pet.set_state)
    anim_controller.start()
    
    # 模拟欢迎语
    greeting = ai_chat.personality.get("greeting", "你好呀！我是 Mochi。")
    QTimer.singleShot(2000, lambda: pet.say(greeting))

    sys.exit(app.exec())

if __name__ == "__main__":
    main()
