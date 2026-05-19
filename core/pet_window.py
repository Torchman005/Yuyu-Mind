import sys
import random
import os
import json
from PySide6.QtWidgets import QWidget, QLabel, QMenu, QApplication, QInputDialog, QSystemTrayIcon, QVBoxLayout
from PySide6.QtCore import Qt, QPoint, QSize, QTimer, Signal as pyqtSignal
from PySide6.QtGui import QPixmap, QMovie, QCursor, QAction, QIcon, QPainter, QColor
from core.bubble import SpeechBubble
from core.chat_input import ChatInput
from utils.resource_manager import resource_manager

class PetWindow(QWidget):
    chat_requested = pyqtSignal(str)
    voice_requested = pyqtSignal()
    response_finished = pyqtSignal() # 新增：回复完成信号
    
    def __init__(self, skin_manager):
        super().__init__()
        self.skin_manager = skin_manager
        self.current_state = "idle"
        self.is_continuous_chat = False
        
        # 强制在初始化前再次读取设置中的 active_skin 以防止遗漏
        settings_path = "config/settings.json"
        if os.path.exists(settings_path):
            with open(settings_path, 'r', encoding='utf-8') as f:
                settings = json.load(f)
                if "active_skin" in settings and settings["active_skin"] in self.skin_manager.skins:
                    self.skin_manager.current_skin_name = settings["active_skin"]
        
        # 加载基础尺寸设置
        self.load_base_settings()
        
        # 必须在 init_ui 之前设置
        self.setAttribute(Qt.WidgetAttribute.WA_TranslucentBackground)
        
        self.init_ui()
        self.bubble = SpeechBubble()
        self.chat_input = ChatInput()
        
        # 获取当前皮肤的配置名字
        # 这里不需要在这里做传递了，已经移动到 main.py 统一处理
        self.chat_input.submitted.connect(lambda text: self.chat_requested.emit(text))
        
        # 修复：不再硬编码 "default"，而是使用 skin_manager 初始化时读取的名字
        self.load_skin(self.skin_manager.current_skin_name)
        self.random_position()
        self.setup_tray()

    def load_base_settings(self):
        settings_path = "config/settings.json"
        if os.path.exists(settings_path):
            with open(settings_path, 'r', encoding='utf-8') as f:
                settings = json.load(f)
                size_cfg = settings.get("base_size", {"width": 200, "height": 200})
                self.base_size = QSize(size_cfg["width"], size_cfg["height"])
                
                # 再次确认如果 skin_manager 没有正确初始化皮肤，从这里读一次
                if self.skin_manager.current_skin_name == "default" and settings.get("active_skin") != "default":
                    self.skin_manager.current_skin_name = settings.get("active_skin", "default")
        else:
            self.base_size = QSize(200, 200)

    def init_ui(self):
        # 彻底解决边框：仅仅使用 FramelessWindowHint 和 WindowStaysOnTopHint
        # 不要使用 Tool 类型，因为 Tool 窗口在 Windows 11 默认会有一层细边框
        self.setWindowFlags(
            Qt.WindowType.FramelessWindowHint | 
            Qt.WindowType.WindowStaysOnTopHint
        )
        
        # 核心设置：开启透明背景支持
        self.setAttribute(Qt.WidgetAttribute.WA_TranslucentBackground, True)
        # 明确告诉系统这个部件不要绘制系统背景
        self.setAttribute(Qt.WidgetAttribute.WA_NoSystemBackground, True)
        
        # 使用布局管理，防止边缘留白
        self.layout = QVBoxLayout(self)
        self.layout.setContentsMargins(0, 0, 0, 0)
        self.layout.setSpacing(0)
        
        self.pet_label = QLabel(self)
        self.pet_label.setAlignment(Qt.AlignmentFlag.AlignCenter)
        self.pet_label.setScaledContents(True)
        # 彻底移除 QLabel 可能带有的框架
        from PySide6.QtWidgets import QFrame
        self.pet_label.setFrameShape(QFrame.Shape.NoFrame)
        self.pet_label.setStyleSheet("background: transparent; border: none; outline: none; margin: 0px; padding: 0px;")
        self.pet_label.setAttribute(Qt.WidgetAttribute.WA_TransparentForMouseEvents, True)
        
        self.layout.addWidget(self.pet_label)
        
        self.dragging = False
        self.drag_pos = QPoint()

    def showEvent(self, event):
        super().showEvent(event)
        # 窗口首次显示时，由于尺寸刚刚计算完毕，强制更新一次掩码，防止部分图片被截断
        QTimer.singleShot(50, self.update_mask)

    def paintEvent(self, event):
        """
        覆盖 paintEvent 并直接返回，强制不进行任何默认绘制，
        这是在 Windows 下彻底消除 QWidget 默认边框或底色的最后手段。
        """
        pass

    def update_mask(self):
        """
        动态计算点击掩码。
        """
        pixmap = None
        if self.pet_label.movie() and self.pet_label.movie().isRunning():
            pixmap = self.pet_label.movie().currentPixmap()
        else:
            pixmap = self.pet_label.pixmap()
            
        if pixmap and not pixmap.isNull() and self.size().width() > 0 and self.size().height() > 0:
            # 修复：获取真实的物理尺寸
            actual_size = self.pet_label.size()
            
            # 关键：确保掩码生成时的尺寸与当前窗口物理尺寸 100% 对应
            mask_pixmap = pixmap.scaled(
                actual_size, 
                Qt.AspectRatioMode.IgnoreAspectRatio, 
                Qt.TransformationMode.SmoothTransformation
            )
            # 应用掩码：只有非透明像素可被点击
            self.setMask(mask_pixmap.mask())
        else:
            # 如果没有图片，则清空掩码（不可点击）
            self.clearMask()

    def setup_tray(self):
        if not hasattr(self, 'tray_icon'):
            self.tray_icon = QSystemTrayIcon(self)
        
        # 使用当前皮肤的 idle 作为图标，或者找个固定的
        icon_path = self.skin_manager.get_animation_path("idle")
        if icon_path:
            self.tray_icon.setIcon(QIcon(icon_path))
        
        tray_menu = QMenu()
        show_action = tray_menu.addAction("显示")
        show_action.triggered.connect(self.show)
        quit_action = tray_menu.addAction("退出")
        quit_action.triggered.connect(QApplication.quit)
        
        self.tray_icon.setContextMenu(tray_menu)
        self.tray_icon.show()

    def load_skin(self, skin_name):
        self.skin_manager.current_skin_name = skin_name
        
        # 获取当前皮肤的配置名字
        skin_config = self.skin_manager.get_skin_config(skin_name)
        display_name = skin_config.get("name", "桌宠")
        
        # 尝试将皮肤名称同步到设置文件，使其成为下次启动的默认皮肤
        settings_path = "config/settings.json"
        if os.path.exists(settings_path):
            try:
                with open(settings_path, 'r', encoding='utf-8') as f:
                    settings = json.load(f)
                settings["active_skin"] = skin_name
                
                # 同步修改 active_personality 使得人设也能跟皮肤联动 (如果存在同名人设)
                personality_path = f"config/personalities/{skin_name}.json"
                if os.path.exists(personality_path):
                     settings["active_personality"] = skin_name
                     
                with open(settings_path, 'w', encoding='utf-8') as f:
                    json.dump(settings, f, indent=4, ensure_ascii=False)
            except Exception as e:
                print(f"Error saving skin to settings: {e}")
                
        self.set_state("idle")

    def set_state(self, state):
        path = self.skin_manager.get_animation_path(state)
        
        # 鲁棒性检查：如果路径无效，回退到 idle
        if not path or not os.path.exists(path):
            if state != "idle":
                self.set_state("idle")
            return

        self.current_state = state
        success = False
        
        # 获取当前皮肤的缩放比例
        config = self.skin_manager.get_skin_config()
        skin_scale = config.get("scale", 1.0)
        target_size = self.base_size * skin_scale

        if path.endswith('.gif'):
            movie = resource_manager.get_movie(path)
            if movie and movie.isValid():
                print(f"[DEBUG] Loading GIF: {path}")
                if self.pet_label.movie():
                    self.pet_label.movie().stop()
                    # 断开之前的连接，防止多个连接冲突
                    try: self.pet_label.movie().frameChanged.disconnect()
                    except: pass
                
                # 设置 GIF 缩放以适配目标尺寸
                movie.setScaledSize(target_size)
                self.pet_label.setMovie(movie)
                # 关键：GIF 每一帧改变时都要更新掩码，以实现动态穿透
                movie.frameChanged.connect(self.update_mask)
                movie.start()
                success = True
        else:
            pixmap = resource_manager.get_pixmap(path)
            if pixmap and not pixmap.isNull():
                print(f"[DEBUG] Loading PNG: {path}")
                if self.pet_label.movie():
                    self.pet_label.movie().stop()
                    try: self.pet_label.movie().frameChanged.disconnect()
                    except: pass
                self.pet_label.setMovie(None)
                
                # 缩放 Pixmap 以适配目标尺寸，保持平滑
                scaled_pixmap = pixmap.scaled(
                    target_size, 
                    Qt.AspectRatioMode.KeepAspectRatio, 
                    Qt.TransformationMode.SmoothTransformation
                )
                self.pet_label.setPixmap(scaled_pixmap)
                # 静态图只需设置一次掩码
                self.setMask(scaled_pixmap.mask())
                success = True
            
        if not success:
            self.pet_label.setText(f"Error: {state}")
            self.pet_label.setStyleSheet("background-color: rgba(255,0,0,150); color: white; border: none; margin: 0px; padding: 0px;")
        else:
            self.pet_label.setText("")
            self.pet_label.setStyleSheet("background: transparent; border: none; outline: none; margin: 0px; padding: 0px;")

        # 调整窗口和标签大小为计算后的目标尺寸
        self.setFixedSize(target_size)
        self.pet_label.setFixedSize(target_size)

        # 强制界面刷新
        self.update()

        if success:
            # 必须在尺寸调整完成后，延迟一丁点时间再更新掩码，否则可能会按旧尺寸裁剪导致图片显示不全
            QTimer.singleShot(10, self.update_mask)

    def random_position(self):
        screen = QApplication.primaryScreen().geometry()
        x = random.randint(0, screen.width() - self.width())
        y = random.randint(0, screen.height() - self.height())
        self.move(x, y)

    def moveEvent(self, event):
        super().moveEvent(event)
        if hasattr(self, 'bubble'):
            config = self.skin_manager.get_skin_config()
            offset = config.get("bubble_offset", {"x": 0, "y": -50})
            self.bubble.move_to_pet(self.pos(), self.size(), offset['x'], offset['y'])
        if hasattr(self, 'chat_input') and self.chat_input.isVisible():
            input_pos = QPoint(self.pos().x(), self.pos().y() + self.height() + 5)
            self.chat_input.move(input_pos)

    def say(self, text):
        self.bubble.show_text(text)
        # 在显示文本后立即重新计算位置，因为气泡高度可能已经改变
        config = self.skin_manager.get_skin_config()
        offset = config.get("bubble_offset", {"x": 0, "y": -50})
        self.bubble.move_to_pet(self.pos(), self.size(), offset['x'], offset['y'])
        
        # 不再使用固定时间的定时器切回 idle，而是依赖语音播放完毕的信号
        # 如果语音被禁用，则退回到定时器模式
        settings_path = "config/settings.json"
        voice_enabled = True
        if os.path.exists(settings_path):
            import json
            with open(settings_path, 'r') as f:
                settings = json.load(f)
                voice_enabled = settings.get("voice_enabled", True)
                
        if not voice_enabled:
            self.set_state("speaking")
            duration = len(text) * 50 + 3000
            QTimer.singleShot(duration, self._on_say_finished)

    def _on_say_finished(self):
        # 隐藏气泡
        if hasattr(self.bubble, 'hide'):
            self.bubble.hide()
        self.set_state("idle")
        self.response_finished.emit() # 触发回复完成信号
        
        # 恢复焦点到主窗口以便接收键盘事件
        self.setFocus()
        
        if self.is_continuous_chat:
            # 延迟弹出，让动作平滑一点
            QTimer.singleShot(1000, self.open_chat_dialog)

    def mousePressEvent(self, event):
        if event.button() == Qt.MouseButton.LeftButton:
            self.dragging = True
            self.drag_pos = event.globalPosition().toPoint() - self.pos()
            event.accept()
            self.setCursor(QCursor(Qt.CursorShape.OpenHandCursor))

    def mouseMoveEvent(self, event):
        if (event.buttons() & Qt.MouseButton.LeftButton) and self.dragging:
            new_pos = event.globalPosition().toPoint() - self.drag_pos
            self.move(new_pos)
            event.accept()

    def mouseReleaseEvent(self, event):
        self.dragging = False
        self.setCursor(QCursor(Qt.CursorShape.ArrowCursor))
        self.check_edge_snapping()

    def check_edge_snapping(self):
        screen = QApplication.primaryScreen().geometry()
        pos = self.pos()
        x, y = pos.x(), pos.y()
        threshold = 30
        
        if x < threshold: x = 0
        if y < threshold: y = 0
        if screen.width() - (x + self.width()) < threshold: x = screen.width() - self.width()
        if screen.height() - (y + self.height()) < threshold: y = screen.height() - self.height()
        
        self.move(x, y)

    def contextMenuEvent(self, event):
        menu = QMenu(self)
        
        chat_action = menu.addAction("文字聊天")
        chat_action.triggered.connect(self.open_chat_dialog)
        
        voice_action = menu.addAction("语音输入")
        voice_action.triggered.connect(self.voice_requested.emit)
        
        menu.addSeparator()
        
        # 尺寸调整菜单
        size_menu = menu.addMenu("调整尺寸")
        scales = [("特小 (0.5x)", 0.5), ("较小 (0.8x)", 0.8), ("标准 (1.0x)", 1.0), ("较大 (1.2x)", 1.2), ("特大 (1.5x)", 1.5)]
        for label, val in scales:
            action = QAction(label, self)
            action.triggered.connect(lambda checked, s=val: self.change_scale(s))
            size_menu.addAction(action)

        state_menu = menu.addMenu("切换状态")
        states = ["idle", "happy", "speaking", "sleep"]
        for s in states:
            action = QAction(s.capitalize(), self)
            action.triggered.connect(lambda checked, state=s: self.set_state(state))
            state_menu.addAction(action)
            
        skin_menu = menu.addMenu("更换皮肤")
        skins = self.skin_manager.get_available_skins()
        for skin in skins:
            action = QAction(skin, self)
            action.triggered.connect(lambda checked, name=skin: self.load_skin(name))
            skin_menu.addAction(action)
            
        menu.addSeparator()
        quit_action = menu.addAction("退出程序")
        quit_action.triggered.connect(QApplication.quit)
        
        menu.exec(event.globalPos())

    def change_scale(self, new_scale):
        """
        动态调整桌宠尺寸缩放。
        """
        config = self.skin_manager.get_skin_config()
        config["scale"] = new_scale # 临时更新当前皮肤配置的缩放
        self.set_state(self.current_state) # 重新触发当前状态以应用缩放

    def open_chat_dialog(self):
        self.is_continuous_chat = True # 开启连续对话模式
        # 计算输入框出现的位置（桌宠下方）
        pos = self.pos()
        input_pos = QPoint(pos.x(), pos.y() + self.height() + 5)
        self.chat_input.show_at(input_pos)

    def keyPressEvent(self, event):
        # 按回车键或 C 键快速打开聊天
        if event.key() in (Qt.Key.Key_Return, Qt.Key.Key_Enter, Qt.Key.Key_C):
            self.open_chat_dialog()
        elif event.key() == Qt.Key.Key_V:
            self.voice_requested.emit()
        elif event.key() == Qt.Key.Key_Escape:
            self.is_continuous_chat = False # 按 ESC 退出连续对话
            self.chat_input.hide()
        super().keyPressEvent(event)

    def closeEvent(self, event):
        self.chat_input.close()
        self.bubble.close()
        super().closeEvent(event)

    def mouseDoubleClickEvent(self, event):
        self.set_state("happy")
        QTimer.singleShot(2000, lambda: self.set_state("idle"))
