//go:build windows

package tools

import (
	"fmt"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var platformInput platformInputExecutor = &windowsInput{}

type keybdinput struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type input struct {
	Type uint32
	Ki   keybdinput
}

const (
	inputKeyboard    = 1
	keyEventfKeyUp   = 0x0002
	keyEventfUnicode = 0x0004
)

var procSendInput = syscall.NewLazyDLL("user32.dll").NewProc("SendInput")

type windowsInput struct{}

func (w *windowsInput) KeyPress(vk uint16) error {
	inputs := []input{
		{Type: inputKeyboard, Ki: keybdinput{WVk: vk}},
		{Type: inputKeyboard, Ki: keybdinput{WVk: vk, DwFlags: keyEventfKeyUp}},
	}
	return sendInputs(inputs)
}

func (w *windowsInput) TypeText(text string) error {
	units := utf16.Encode([]rune(text))
	for _, ch := range units {
		inputs := []input{
			{Type: inputKeyboard, Ki: keybdinput{WScan: ch, DwFlags: keyEventfUnicode}},
			{Type: inputKeyboard, Ki: keybdinput{WScan: ch, DwFlags: keyEventfUnicode | keyEventfKeyUp}},
		}
		if err := sendInputs(inputs); err != nil {
			return err
		}
	}
	return nil
}

func sendInputs(inputs []input) error {
	if len(inputs) == 0 {
		return nil
	}
	n, _, callErr := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if n == 0 {
		return fmt.Errorf("SendInput failed: %v", callErr)
	}
	return nil
}
