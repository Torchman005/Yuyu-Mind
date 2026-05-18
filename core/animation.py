import random
from PySide6.QtCore import QObject, QTimer, Signal

class AnimationController(QObject):
    state_changed = Signal(str)

    def __init__(self, interval=30000):
        super().__init__()
        self.timer = QTimer(self)
        self.timer.timeout.connect(self.random_action)
        self.interval = interval
        self.states = ["idle", "happy", "sleep"]

    def start(self):
        self.timer.start(self.interval)

    def stop(self):
        self.timer.stop()

    def set_interval(self, ms):
        self.interval = ms
        if self.timer.isActive():
            self.timer.start(ms)

    def random_action(self):
        next_state = random.choice(self.states)
        self.state_changed.emit(next_state)
        
        # 如果是 happy 动作，持续一段时间后自动切回 idle
        if next_state == "happy":
            QTimer.singleShot(3000, lambda: self.state_changed.emit("idle"))
