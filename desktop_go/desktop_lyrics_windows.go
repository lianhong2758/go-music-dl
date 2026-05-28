//go:build windows

package main

import (
	"log"
	"reflect"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

const desktopLyricsURL = "http://localhost:37777/music/desktop_lyrics"

type desktopLyricsWindowManager struct {
	mu      sync.Mutex
	running bool
}

func newDesktopLyricsWindowManager() *desktopLyricsWindowManager {
	return &desktopLyricsWindowManager{}
}

func (m *desktopLyricsWindowManager) Open() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer func() {
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
		}()
		runDesktopLyricsWindow()
	}()
}

func runDesktopLyricsWindow() {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "music-dl-desktop-lyrics",
			Width:  920,
			Height: 260,
			Center: true,
		},
	})
	if w == nil {
		log.Println("failed to load desktop lyrics webview")
		return
	}
	defer w.Destroy()

	hwnd := uintptr(w.Window())
	w.SetSize(920, 260, webview2.HintFixed)
	configureDesktopLyricsWindow(w, hwnd)

	if err := w.Bind("musicDlCloseDesktopLyrics", func() error {
		w.Dispatch(func() {
			w.Destroy()
		})
		return nil
	}); err != nil {
		log.Printf("failed to bind desktop lyrics close: %v", err)
	}
	if err := w.Bind("musicDlMinimizeDesktopLyrics", func() error {
		minimizeDesktopLyricsWindow(hwnd)
		return nil
	}); err != nil {
		log.Printf("failed to bind desktop lyrics minimize: %v", err)
	}
	if err := w.Bind("musicDlMoveDesktopLyrics", func() error {
		beginDesktopLyricsWindowMove(hwnd)
		return nil
	}); err != nil {
		log.Printf("failed to bind desktop lyrics move: %v", err)
	}

	w.Navigate(desktopLyricsURL)
	w.Run()
}

func configureDesktopLyricsWindow(w webview2.WebView, hwnd uintptr) {
	makeWindowFramelessTransparent(hwnd)
	setWebViewTransparentBackground(w)
	go func() {
		time.Sleep(250 * time.Millisecond)
		w.Dispatch(func() {
			setWebViewTransparentBackground(w)
		})
	}()
}

func setWebViewTransparentBackground(w webview2.WebView) {
	controller := webViewController(w)
	if controller == nil {
		return
	}
	controller2 := controller.GetICoreWebView2Controller2()
	if controller2 == nil {
		return
	}
	if err := controller2.PutDefaultBackgroundColor(edge.COREWEBVIEW2_COLOR{A: 0, R: 0, G: 0, B: 0}); err != nil {
		log.Printf("failed to set transparent webview background: %v", err)
	}
}

type webViewControllerGetter interface {
	GetController() *edge.ICoreWebView2Controller
}

func webViewController(w webview2.WebView) *edge.ICoreWebView2Controller {
	value := reflect.ValueOf(w)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return nil
	}
	elem := value.Elem()
	if !elem.IsValid() {
		return nil
	}
	field := elem.FieldByName("browser")
	if !field.IsValid() || !field.CanAddr() {
		return nil
	}
	browser := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
	getter, ok := browser.(webViewControllerGetter)
	if !ok {
		return nil
	}
	return getter.GetController()
}

var (
	user32                     = windows.NewLazySystemDLL("user32.dll")
	dwmapi                     = windows.NewLazySystemDLL("dwmapi.dll")
	procGetWindowLongPtrW      = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW      = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos           = user32.NewProc("SetWindowPos")
	procSetLayeredWindowAttrs  = user32.NewProc("SetLayeredWindowAttributes")
	procShowWindow             = user32.NewProc("ShowWindow")
	procReleaseCapture         = user32.NewProc("ReleaseCapture")
	procSendMessageW           = user32.NewProc("SendMessageW")
	procDwmExtendFrameIntoArea = dwmapi.NewProc("DwmExtendFrameIntoClientArea")
)

const (
	gwlStyle   = ^uintptr(15) + 1
	gwlExStyle = ^uintptr(19) + 1

	wsBorder      = 0x00800000
	wsCaption     = 0x00c00000
	wsDlgFrame    = 0x00400000
	wsMaximizeBox = 0x00010000
	wsMinimizeBox = 0x00020000
	wsPopup       = 0x80000000
	wsSysMenu     = 0x00080000
	wsThickFrame  = 0x00040000

	wsExClientEdge    = 0x00000200
	wsExDlgModalFrame = 0x00000001
	wsExLayered       = 0x00080000
	wsExStaticEdge    = 0x00020000
	wsExTopMost       = 0x00000008
	wsExToolWindow    = 0x00000080
	wsExWindowEdge    = 0x00000100

	lwaAlpha   = 0x00000002
	swMinimize = 6

	swpNoMove       = 0x0002
	swpNoSize       = 0x0001
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020

	hwndTopMost = ^uintptr(0)

	wmNCLButtonDown = 0x00a1
	htCaption       = 2
)

type dwmMargins struct {
	CxLeftWidth    int32
	CxRightWidth   int32
	CyTopHeight    int32
	CyBottomHeight int32
}

func makeWindowFramelessTransparent(hwnd uintptr) {
	style := getWindowLongPtr(hwnd, gwlStyle)
	style &^= wsCaption | wsThickFrame | wsMinimizeBox | wsMaximizeBox | wsSysMenu | wsBorder | wsDlgFrame
	style |= wsPopup
	setWindowLongPtr(hwnd, gwlStyle, style)

	exStyle := getWindowLongPtr(hwnd, gwlExStyle)
	exStyle &^= wsExClientEdge | wsExDlgModalFrame | wsExStaticEdge | wsExWindowEdge
	exStyle |= wsExLayered | wsExTopMost | wsExToolWindow
	setWindowLongPtr(hwnd, gwlExStyle, exStyle)

	procSetLayeredWindowAttrs.Call(hwnd, 0, 255, lwaAlpha)
	margins := dwmMargins{-1, -1, -1, -1}
	procDwmExtendFrameIntoArea.Call(hwnd, uintptr(unsafe.Pointer(&margins)))
	procSetWindowPos.Call(hwnd, hwndTopMost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpFrameChanged)
}

func minimizeDesktopLyricsWindow(hwnd uintptr) {
	procShowWindow.Call(hwnd, swMinimize)
}

func beginDesktopLyricsWindowMove(hwnd uintptr) {
	procReleaseCapture.Call()
	procSendMessageW.Call(hwnd, wmNCLButtonDown, htCaption, 0)
}

func getWindowLongPtr(hwnd uintptr, index uintptr) uintptr {
	ret, _, _ := procGetWindowLongPtrW.Call(hwnd, index)
	return ret
}

func setWindowLongPtr(hwnd uintptr, index uintptr, value uintptr) uintptr {
	ret, _, _ := procSetWindowLongPtrW.Call(hwnd, index, value)
	return ret
}
