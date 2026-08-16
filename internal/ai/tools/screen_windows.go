//go:build windows

package tools

import (
	"errors"
	"image"
	"syscall"
	"unsafe"
)

var screenCapture screenCaptureExecutor = &windowsScreenCapture{}

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct {
	BmiHeader bitmapInfoHeader
}

const (
	smCXScreen = 0
	smCYScreen = 1
	srcCopy    = 0x00CC0020
	dibRGB     = 0
)

var (
	user32DLL = syscall.NewLazyDLL("user32.dll")
	gdi32DLL  = syscall.NewLazyDLL("gdi32.dll")

	procGetDC                  = user32DLL.NewProc("GetDC")
	procReleaseDC              = user32DLL.NewProc("ReleaseDC")
	procGetSystemMetrics       = user32DLL.NewProc("GetSystemMetrics")
	procCreateCompatibleDC     = gdi32DLL.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32DLL.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32DLL.NewProc("SelectObject")
	procDeleteObject           = gdi32DLL.NewProc("DeleteObject")
	procDeleteDC               = gdi32DLL.NewProc("DeleteDC")
	procBitBlt                 = gdi32DLL.NewProc("BitBlt")
	procGetDIBits              = gdi32DLL.NewProc("GetDIBits")
)

type windowsScreenCapture struct{}

// Capture 截取主屏幕并返回 RGBA 图像。
func (w *windowsScreenCapture) Capture() (*image.RGBA, error) {
	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		return nil, errors.New("GetDC failed")
	}
	defer procReleaseDC.Call(0, hdc)

	width, _, _ := procGetSystemMetrics.Call(smCXScreen)
	height, _, _ := procGetSystemMetrics.Call(smCYScreen)
	if width == 0 || height == 0 {
		return nil, errors.New("screen size is zero")
	}
	wInt, hInt := int(width), int(height)

	memDC, _, _ := procCreateCompatibleDC.Call(hdc)
	if memDC == 0 {
		return nil, errors.New("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(memDC)

	bitmap, _, _ := procCreateCompatibleBitmap.Call(hdc, width, height)
	if bitmap == 0 {
		return nil, errors.New("CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(bitmap)

	old, _, _ := procSelectObject.Call(memDC, bitmap)
	defer procSelectObject.Call(memDC, old)

	if ret, _, _ := procBitBlt.Call(memDC, 0, 0, width, height, hdc, 0, 0, srcCopy); ret == 0 {
		return nil, errors.New("BitBlt failed")
	}

	bi := bitmapInfo{
		BmiHeader: bitmapInfoHeader{
			BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth:       int32(wInt),
			BiHeight:      -int32(hInt), // 负高度 = 自顶向下
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: 0, // BI_RGB
		},
	}

	pixels := make([]byte, wInt*hInt*4)
	if ret, _, _ := procGetDIBits.Call(memDC, bitmap, 0, height, uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&bi)), dibRGB); ret == 0 {
		return nil, errors.New("GetDIBits failed")
	}

	img := image.NewRGBA(image.Rect(0, 0, wInt, hInt))
	for i := 0; i < wInt*hInt; i++ {
		// GetDIBits 32bpp BI_RGB 返回 BGRA 顺序。
		img.Pix[i*4] = pixels[i*4+2]   // R
		img.Pix[i*4+1] = pixels[i*4+1] // G
		img.Pix[i*4+2] = pixels[i*4]   // B
		img.Pix[i*4+3] = pixels[i*4+3] // A
	}
	return img, nil
}
