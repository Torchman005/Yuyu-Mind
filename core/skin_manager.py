import os
import json
from PySide6.QtGui import QPixmap, QMovie

class SkinManager:
    def __init__(self, skins_dir="assets/skins"):
        self.skins_dir = skins_dir
        
        # 从全局设置读取默认皮肤
        self.current_skin_name = "default"
        settings_path = "config/settings.json"
        if os.path.exists(settings_path):
            try:
                with open(settings_path, 'r', encoding='utf-8') as f:
                    settings = json.load(f)
                    self.current_skin_name = settings.get("active_skin", "default")
            except Exception as e:
                print(f"Error loading settings in SkinManager: {e}")
                
        self.skins = {}
        self.load_available_skins()
        
        # 确保当前皮肤有效，否则回退
        if self.current_skin_name not in self.skins and self.skins:
            self.current_skin_name = list(self.skins.keys())[0]

    def load_available_skins(self):
        if not os.path.exists(self.skins_dir):
            return
        
        for skin_name in os.listdir(self.skins_dir):
            skin_path = os.path.join(self.skins_dir, skin_name)
            config_path = os.path.join(skin_path, "config.json")
            if os.path.isdir(skin_path) and os.path.exists(config_path):
                try:
                    with open(config_path, 'r', encoding='utf-8') as f:
                        config = json.load(f)
                        config['path'] = skin_path
                        self.skins[skin_name] = config
                except Exception as e:
                    print(f"Error loading skin {skin_name}: {e}")

    def get_skin_config(self, skin_name=None):
        name = skin_name or self.current_skin_name
        return self.skins.get(name, {})

    def get_animation_config(self, state, skin_name=None):
        config = self.get_skin_config(skin_name)
        if not config:
            return None
        return config.get("animations", {}).get(state)

    def get_animation_path(self, state, skin_name=None):
        config = self.get_skin_config(skin_name)
        anim_config = self.get_animation_config(state, skin_name)
        if not config or not anim_config:
            return None
        return os.path.join(config['path'], anim_config['file'])

    def get_available_skins(self):
        return list(self.skins.keys())
