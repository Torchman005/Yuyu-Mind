from PySide6.QtWidgets import QWidget, QVBoxLayout, QLabel
from PySide6.QtCore import Qt, QTimer, QSize
from PySide6.QtGui import QColor, QPalette, QFontMetrics

class SpeechBubble(QWidget):
    def __init__(self, parent=None):
        super().__init__(parent)
        self.init_ui()
        self.hide()
        
        # 打字机效果相关
        self.timer = QTimer(self)
        self.timer.timeout.connect(self.update_text)
        self.full_text = ""
        self.current_index = 0
        
        # 自动消失定时器
        self.hide_timer = QTimer(self)
        self.hide_timer.setSingleShot(True)
        self.hide_timer.timeout.connect(self.hide)

    def init_ui(self):
        self.setWindowFlags(Qt.WindowType.FramelessWindowHint | Qt.WindowType.WindowStaysOnTopHint | Qt.WindowType.Tool)
        self.setAttribute(Qt.WidgetAttribute.WA_TranslucentBackground)
        
        self.layout = QVBoxLayout(self)
        self.layout.setContentsMargins(10, 10, 10, 10)
        
        self.label = QLabel(self)
        self.label.setWordWrap(True)
        self.label.setStyleSheet("""
            background-color: rgba(255, 255, 255, 220);
            border: 2px solid #888888;
            border-radius: 15px;
            padding: 12px;
            color: #333333;
            font-family: "Microsoft YaHei";
            font-size: 14px;
            line-height: 1.4;
        """)
        self.layout.addWidget(self.label)
        
        # 设置气泡的最大宽度
        self.max_width = 300
        self.setFixedWidth(self.max_width)

    def show_text(self, text, duration=5000):
        self.full_text = text
        self.current_index = 0
        self.label.setText("")
        self.show()
        
        # 动态计算高度
        metrics = QFontMetrics(self.label.font())
        # 减去 padding 和 margins
        content_width = self.max_width - 40 
        
        # 使用较大的高度限制以获取完整的边界矩形
        rect = metrics.boundingRect(
            0, 0, 
            content_width, 5000, 
            Qt.AlignmentFlag.AlignLeft | Qt.TextFlag.TextWordWrap, 
            text
        )
        
        # 设置气泡高度，额外增加一些缓冲空间（padding/margin）
        new_height = rect.height() + 60
        self.setFixedHeight(new_height)
        
        self.timer.start(50)  # 打字机速度 50ms/char
        self.hide_timer.start(duration + len(text) * 50)

    def update_text(self):
        if self.current_index < len(self.full_text):
            self.current_index += 1
            self.label.setText(self.full_text[:self.current_index])
        else:
            self.timer.stop()

    def move_to_pet(self, pet_pos, pet_size, offset_x=0, offset_y=-50):
        # 移动到宠物指定偏移位置
        self.move(pet_pos.x() + (pet_size.width() - self.width()) // 2 + offset_x, 
                  pet_pos.y() - self.height() + offset_y)
