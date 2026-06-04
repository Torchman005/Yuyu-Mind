//go:build windows

package main

import (
	"errors"
	"math"
	"sync"
	"syscall"
	"time"
	"unsafe"
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
	wsExTransparent  = 0x00000020
	wsExLayered      = 0x00080000
	petHitPollPeriod = 45 * time.Millisecond
)

var (
	petHitMu             sync.Mutex
	petHitState          PetHitTestState
	petHitPollStarted    bool
	petMousePassthrough  bool
	petWindowHandleCache uintptr

	user32                = syscall.NewLazyDLL("user32.dll")
	procFindWindowW       = user32.NewProc("FindWindowW")
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
	return state
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
	petHitMu.Unlock()

	shouldPassThrough := false
	if state.Enabled && !state.ControlsOpen {
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

	title, err := syscall.UTF16PtrFromString("MochiAI")
	if err != nil {
		return 0, err
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return 0, errors.New("MochiAI window was not found")
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
