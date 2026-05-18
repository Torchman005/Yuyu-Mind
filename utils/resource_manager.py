import os
from PySide6.QtGui import QPixmap, QMovie

class ResourceManager:
    _instance = None

    def __new__(cls):
        if cls._instance is None:
            cls._instance = super(ResourceManager, cls).__new__(cls)
            cls._instance.cache = {}
        return cls._instance

    def get_pixmap(self, path):
        if path not in self.cache:
            if os.path.exists(path):
                self.cache[path] = QPixmap(path)
            else:
                return None
        return self.cache[path]

    def get_movie(self, path):
        # QMovie 通常不建议缓存实例，因为其状态（当前帧）是共享的
        # 这里返回新实例，但可以记录路径
        if os.path.exists(path):
            return QMovie(path)
        return None

    def clear_cache(self):
        self.cache.clear()

resource_manager = ResourceManager()
