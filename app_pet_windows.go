//go:build windows

package main

import (
	"errors"
	"math"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type winPoint struct {
	X int32
	Y int32
}

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

const (
	gwlExStyle       = -20
	vkControl        = 0x11
	wsExTransparent  = 0x00000020
	wsExLayered      = 0x00080000
	petHitPollPeriod = 45 * time.Millisecond
	petHotkeyPeriod  = 35 * time.Millisecond
)

var (
	petHitMu             sync.Mutex
	petHitState          PetHitTestState
	petHotkey            = petShortcutSpec{Ctrl: true, Key: 0x48}
	petControlsHotkey    = petShortcutSpec{Ctrl: true, Shift: true, Key: 0x4D}
	petVoiceHotkey       = petShortcutSpec{Ctrl: true, Key: 0x56}
	petHitPollStarted    bool
	petHotkeyStarted     bool
	petMousePassthrough  bool
	petWindowHandleCache uintptr

	user32                = syscall.NewLazyDLL("user32.dll")
	procFindWindowW       = user32.NewProc("FindWindowW")
	procGetAsyncKeyState  = user32.NewProc("GetAsyncKeyState")
	procGetCursorPos      = user32.NewProc("GetCursorPos")
	procGetWindowRect     = user32.NewProc("GetWindowRect")
	procGetWindowLongPtrW = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
	procGetWindowLongW    = user32.NewProc("GetWindowLongW")
	procSetWindowLongW    = user32.NewProc("SetWindowLongW")
)

func (a *App) UpdatePetHitTest(state PetHitTestState) error {
	petHitMu.Lock()
	petHitState = sanitizePetHitTestState(state)
	if !petHitPollStarted {
		petHitPollStarted = true
		go pollPetHitTest()
	}
	petHitMu.Unlock()

	applyPetHitTest()
	return nil
}

func sanitizePetHitTestState(state PetHitTestState) PetHitTestState {
	if !state.Enabled || state.Width <= 0 || state.Height <= 0 {
		return PetHitTestState{}
	}
	if !isFinite(state.X) || !isFinite(state.Y) || !isFinite(state.Width) || !isFinite(state.Height) {
		return PetHitTestState{}
	}
	state.MouseShortcut = normalizePetShortcut(state.MouseShortcut)
	return state
}

func (a *App) startPetModeHotkey() {
	petHitMu.Lock()
	if petHotkeyStarted {
		petHitMu.Unlock()
		return
	}
	petHotkeyStarted = true
	petHitMu.Unlock()

	go a.pollPetModeHotkey()
}

func (a *App) pollPetModeHotkey() {
	ticker := time.NewTicker(petHotkeyPeriod)
	defer ticker.Stop()

	wasDown := false
	controlsWasDown := false
	voiceWasDown := false
	for {
		select {
		case <-ticker.C:
			petHitMu.Lock()
			state := petHitState
			hotkey := petHotkey
			controlsHotkey := petControlsHotkey
			voiceHotkey := petVoiceHotkey
			petHitMu.Unlock()
			isDown := hotkey.isDown()
			if isDown && !wasDown {
				forcePassthrough := togglePetForcePassthrough()
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, "mochi:pet:mouse-mode", map[string]any{
						"forcePassthrough": forcePassthrough,
					})
				}
			}
			wasDown = isDown
			controlsDown := controlsHotkey.isDown()
			if controlsDown && !controlsWasDown && a.ctx != nil {
				runtime.EventsEmit(a.ctx, "mochi:pet:controls-toggle", map[string]any{})
			}
			controlsWasDown = controlsDown
			voiceDown := voiceHotkey.isDown()
			if voiceDown && !voiceWasDown && state.Enabled && state.ForcePassthrough && a.ctx != nil {
				runtime.EventsEmit(a.ctx, "mochi:pet:voice-input", map[string]any{})
			}
			voiceWasDown = voiceDown
		case <-a.ctx.Done():
			return
		}
	}
}

func asyncKeyDown(key int) bool {
	state, _, _ := procGetAsyncKeyState.Call(uintptr(key))
	return state&0x8000 != 0
}

type petShortcutSpec struct {
	Ctrl  bool
	Shift bool
	Alt   bool
	Key   int
}

func (shortcut petShortcutSpec) isDown() bool {
	if shortcut.Key == 0 {
		return false
	}
	if shortcut.Ctrl != asyncKeyDown(vkControl) {
		return false
	}
	if shortcut.Shift != asyncKeyDown(0x10) {
		return false
	}
	if shortcut.Alt != asyncKeyDown(0x12) {
		return false
	}
	return asyncKeyDown(shortcut.Key)
}

func normalizePetShortcut(value string) string {
	shortcut := strings.TrimSpace(value)
	if shortcut == "" {
		return "Ctrl+H"
	}
	compact := strings.ToUpper(strings.ReplaceAll(shortcut, " ", ""))
	switch compact {
	case "CTRL+M", "CONTROL+M", "CTRL+SHIFT+M", "CONTROL+SHIFT+M":
		return "Ctrl+H"
	}
	return shortcut
}

func parsePetShortcut(value string) (petShortcutSpec, bool) {
	parts := strings.Split(strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", "")), "+")
	shortcut := petShortcutSpec{}
	for _, part := range parts {
		switch part {
		case "CTRL", "CONTROL":
			shortcut.Ctrl = true
		case "SHIFT":
			shortcut.Shift = true
		case "ALT":
			shortcut.Alt = true
		case "":
		default:
			key, ok := petShortcutKey(part)
			if !ok || shortcut.Key != 0 {
				return petShortcutSpec{}, false
			}
			shortcut.Key = key
		}
	}
	if shortcut.Key == 0 || (!shortcut.Ctrl && !shortcut.Shift && !shortcut.Alt) {
		return petShortcutSpec{}, false
	}
	return shortcut, true
}

func petShortcutKey(key string) (int, bool) {
	if len(key) == 1 {
		character := key[0]
		if character >= 'A' && character <= 'Z' {
			return int(character), true
		}
		if character >= '0' && character <= '9' {
			return int(character), true
		}
	}
	switch key {
	case "SPACE":
		return 0x20, true
	case "TAB":
		return 0x09, true
	case "F1":
		return 0x70, true
	case "F2":
		return 0x71, true
	case "F3":
		return 0x72, true
	case "F4":
		return 0x73, true
	case "F5":
		return 0x74, true
	case "F6":
		return 0x75, true
	case "F7":
		return 0x76, true
	case "F8":
		return 0x77, true
	case "F9":
		return 0x78, true
	case "F10":
		return 0x79, true
	case "F11":
		return 0x7A, true
	case "F12":
		return 0x7B, true
	default:
		return 0, false
	}
}

func togglePetForcePassthrough() bool {
	petHitMu.Lock()
	if !petHitState.Enabled {
		petHitState.ForcePassthrough = false
		petHitMu.Unlock()
		applyPetHitTest()
		return false
	}
	petHitState.ForcePassthrough = !petHitState.ForcePassthrough
	forcePassthrough := petHitState.ForcePassthrough
	petHitMu.Unlock()

	applyPetHitTest()
	return forcePassthrough
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func pollPetHitTest() {
	ticker := time.NewTicker(petHitPollPeriod)
	defer ticker.Stop()

	for range ticker.C {
		applyPetHitTest()
	}
}

func applyPetHitTest() {
	petHitMu.Lock()
	state := petHitState
	if shortcut, ok := parsePetShortcut(state.MouseShortcut); ok {
		petHotkey = shortcut
	}
	petHitMu.Unlock()

	shouldPassThrough := false
	if state.Enabled && state.ForcePassthrough {
		shouldPassThrough = true
	} else if state.Enabled && !state.ControlsOpen {
		inside, err := cursorInsidePetHitRect(state)
		shouldPassThrough = err == nil && !inside
	}

	_ = setPetMousePassthrough(shouldPassThrough)
}

func cursorInsidePetHitRect(state PetHitTestState) (bool, error) {
	hwnd, err := petWindowHandle()
	if err != nil {
		return true, err
	}

	var point winPoint
	if ok, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&point))); ok == 0 {
		return true, errors.New("GetCursorPos failed")
	}

	var rect winRect
	if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ok == 0 {
		return true, errors.New("GetWindowRect failed")
	}

	clientX := float64(point.X - rect.Left)
	clientY := float64(point.Y - rect.Top)
	if clientX < state.X ||
		clientX > state.X+state.Width ||
		clientY < state.Y ||
		clientY > state.Y+state.Height {
		return false, nil
	}

	return cursorInsidePetContour(
		(clientX-state.X)/state.Width,
		(clientY-state.Y)/state.Height,
	), nil
}

func cursorInsidePetContour(x float64, y float64) bool {
	if x < 0 || x > 1 || y < 0 || y > 1 {
		return false
	}

	left, right := petContourBand(y)
	return x >= left && x <= right
}

func petContourBand(y float64) (float64, float64) {
	switch {
	case y < 0.08:
		return 0.32, 0.68
	case y < 0.18:
		return 0.22, 0.78
	case y < 0.32:
		return 0.14, 0.86
	case y < 0.72:
		return 0.04, 0.96
	case y < 0.90:
		return 0.12, 0.88
	default:
		return 0.22, 0.78
	}
}

func petWindowHandle() (uintptr, error) {
	if petWindowHandleCache != 0 {
		return petWindowHandleCache, nil
	}

	title, err := syscall.UTF16PtrFromString("Yuyu-Mind")
	if err != nil {
		return 0, err
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return 0, errors.New("Yuyu-Mind window was not found")
	}
	petWindowHandleCache = hwnd
	return hwnd, nil
}

func setPetMousePassthrough(enabled bool) error {
	if petMousePassthrough == enabled {
		return nil
	}

	hwnd, err := petWindowHandle()
	if err != nil {
		return err
	}

	style := getWindowExStyle(hwnd)
	style |= wsExLayered
	if enabled {
		style |= wsExTransparent
	} else {
		style &^= wsExTransparent
	}
	setWindowExStyle(hwnd, style)
	petMousePassthrough = enabled
	return nil
}

func getWindowExStyle(hwnd uintptr) uintptr {
	index := windowLongExStyleIndex()
	if unsafe.Sizeof(uintptr(0)) == 8 {
		style, _, _ := procGetWindowLongPtrW.Call(hwnd, index)
		return style
	}
	style, _, _ := procGetWindowLongW.Call(hwnd, index)
	return style
}

func setWindowExStyle(hwnd uintptr, style uintptr) {
	index := windowLongExStyleIndex()
	if unsafe.Sizeof(uintptr(0)) == 8 {
		_, _, _ = procSetWindowLongPtrW.Call(hwnd, index, style)
		return
	}
	_, _, _ = procSetWindowLongW.Call(hwnd, index, style)
}

func windowLongExStyleIndex() uintptr {
	return ^uintptr(0) - uintptr(-gwlExStyle) + 1
}
