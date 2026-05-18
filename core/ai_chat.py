import os
import json
from openai import OpenAI
from PySide6.QtCore import QObject, Signal as pyqtSignal, QThread

class AIWorker(QObject):
    finished = pyqtSignal(str)
    error = pyqtSignal(str)

    def __init__(self, client, model, system_prompt, history, user_input, temperature=0.7):
        super().__init__()
        self.client = client
        self.model = model
        self.system_prompt = system_prompt
        self.history = history
        self.user_input = user_input
        self.temperature = temperature

    def run(self):
        try:
            messages = [{"role": "system", "content": self.system_prompt}]
            messages.extend(self.history)
            messages.append({"role": "user", "content": self.user_input})
            
            # 针对不同模型启用 JSON mode 
            # 大多数现代模型（包括 deepseek-chat 和 gpt-3.5-turbo）都支持 response_format={"type": "json_object"}
            # 这能极大降低 AI 破坏 JSON 格式的概率
            response = self.client.chat.completions.create(
                model=self.model,
                messages=messages,
                temperature=self.temperature,
                timeout=10.0,
                response_format={"type": "json_object"}
            )
            content = response.choices[0].message.content
            self.finished.emit(content)
        except Exception as e:
            self.error.emit(str(e))

class AIChat(QObject):
    # 发送一个字典，包含中文文本和日文文本
    response_ready = pyqtSignal(dict)

    def __init__(self, config_path="config/settings.json"):
        super().__init__()
        self.settings = {}
        self.load_config(config_path)
        self.history = []
        self.max_history = 10
        self.personality = {}
        self.load_personality()
        
        # 根据人设配置初始化客户端
        self.init_client()
        
        self.thread = None
        self.worker = None

    def load_config(self, path):
        if os.path.exists(path):
            with open(path, 'r', encoding='utf-8') as f:
                self.settings = json.load(f)

    def init_client(self):
        api_config = self.personality.get("api_config", {})
        provider = api_config.get("provider", "openai")
        
        api_key = self.settings.get("api_keys", {}).get(provider, "")
        base_url = self.settings.get("api_urls", {}).get(provider, "https://api.openai.com/v1")
        
        # 如果 settings 中没有，尝试从环境变量读取
        if not api_key:
            env_key_name = f"{provider.upper()}_API_KEY"
            api_key = os.getenv(env_key_name, "")
            
        self.client = OpenAI(
            api_key=api_key,
            base_url=base_url
        )

    def load_personality(self, name=None):
        name = name or self.settings.get("active_personality", "default")
        p_path = f"config/personalities/{name}.json"
        if os.path.exists(p_path):
            with open(p_path, 'r', encoding='utf-8') as f:
                self.personality = json.load(f)

    def ask(self, user_input, override_name=None):
        # 优先使用 override_name，否则使用人设配置文件中的 name，最后回退到"桌宠"
        bot_name = override_name or self.personality.get('name', '桌宠')
        
        role_desc = self.personality.get('role_description', '')
        style_prompt = self.personality.get('style_prompt', '')
        
        if bot_name and bot_name != "桌宠":
            role_desc = role_desc.replace("桌宠", bot_name).replace("宠物助手", bot_name)
            
        system_prompt = f"{role_desc}\n{style_prompt}"
        
        # 强制 AI 知道自己的名字并遵循双语输出规则
        if bot_name and bot_name != "桌宠":
            system_prompt += f"\n请记住，你的名字是：{bot_name}，请以此身份与用户对话。"
            
        system_prompt += f"""
【强制格式要求】
你的回复必须是一个合法的 JSON 对象，包含两个字段：
1. "zh": 你的回复的中文版本（用于气泡显示）
2. "jp": 你的回复的对应日文翻译（用于语音合成，请尽量使用口语化、符合你人设的日语表达）
不要输出任何 Markdown 标记或多余的文字，只需输出纯 JSON。
例如：{{"zh": "你好呀！", "jp": "こんにちは！"}}
"""
            
        api_config = self.personality.get("api_config", {})
        
        self.thread = QThread()
        self.worker = AIWorker(
            self.client, 
            api_config.get("model", "gpt-3.5-turbo"),
            system_prompt,
            self.history,
            user_input,
            temperature=api_config.get("temperature", 0.7)
        )
        self.worker.moveToThread(self.thread)
        
        self.thread.started.connect(self.worker.run)
        self.worker.finished.connect(self.on_finished)
        self.worker.error.connect(self.on_error)
        
        self.thread.start()
        # 同时记录历史
        self.history.append({"role": "user", "content": user_input})

    def on_finished(self, text):
        try:
            parsed_response = json.loads(text)
            zh_text = parsed_response.get("zh", "解析失败")
            jp_text = parsed_response.get("jp", "エラーが発生しました")
            
            # 修复：历史记录中必须保存完整的 JSON 字符串，否则 AI 会“忘记”自己需要输出 JSON
            self.history.append({"role": "assistant", "content": text})
            if len(self.history) > self.max_history * 2:
                self.history = self.history[-self.max_history * 2:]
            self.response_ready.emit({"zh": zh_text, "jp": jp_text})
        except json.JSONDecodeError:
            self.history.append({"role": "assistant", "content": text})
            if len(self.history) > self.max_history * 2:
                self.history = self.history[-self.max_history * 2:]
            self.response_ready.emit({"zh": text, "jp": text})
            
        self.thread.quit()
        self.thread.wait()

    def on_error(self, err):
        print(f"AI Error: {err}")
        self.response_ready.emit({"zh": "（Mochi 似乎断网了喵... 请检查 API 设置）", "jp": "（ネットワークに接続されていません… API設定を確認してください）"})
        self.thread.quit()
        self.thread.wait()
