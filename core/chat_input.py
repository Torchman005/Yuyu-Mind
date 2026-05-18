from PySide6.QtWidgets import QWidget, QVBoxLayout, QLineEdit
from PySide6.QtCore import Qt, Signal, QPoint

class ChatInput(QWidget):
    submitted = Signal(str)

    def __init__(self, parent=None):
        super().__init__(parent)
        self.init_ui()
        self.hide()

    def init_ui(self):
        # 无边框、工具窗口、始终置顶
        self.setWindowFlags(Qt.WindowType.FramelessWindowHint | Qt.WindowType.WindowStaysOnTopHint | Qt.WindowType.Tool)
        self.setAttribute(Qt.WidgetAttribute.WA_TranslucentBackground)
        
        layout = QVBoxLayout(self)
        layout.setContentsMargins(5, 5, 5, 5)
        
        self.input_field = QLineEdit(self)
        self.input_field.setPlaceholderText("想对 Mochi 说什么？(回车发送)")
        self.input_field.setStyleSheet("""
            QLineEdit {
                background-color: rgba(255, 255, 255, 230);
                border: 2px solid #888888;
                border-radius: 10px;
                padding: 8px;
                color: #333333;
                font-family: "Microsoft YaHei";
                font-size: 13px;
                min-width: 200px;
            }
        """)
        
        self.input_field.returnPressed.connect(self.on_submit)
        layout.addWidget(self.input_field)
        
        # 移除之前的 editingFinished 自动隐藏，改为更精确的控制
        # self.input_field.editingFinished.connect(self.hide)

    def on_submit(self):
        text = self.input_field.text().strip()
        if text:
            self.submitted.emit(text)
            self.input_field.clear()
        # 提交后隐藏，但保留实例
        self.hide()

    def show_at(self, pos):
        self.move(pos)
        self.show()
        self.input_field.setFocus()

    def keyPressEvent(self, event):
        if event.key() == Qt.Key.Key_Escape:
            self.parent().is_continuous_chat = False # 显式通知父窗口退出连续模式
            self.hide()
        super().keyPressEvent(event)
