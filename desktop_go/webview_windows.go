//go:build windows

package main

import (
	"log"
	"runtime"

	"github.com/guohuiyuan/go-music-dl/internal/web"
	"github.com/jchv/go-webview2"
)

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	go web.StartDesktop("37777")
	desktopLyrics := newDesktopLyricsWindowManager()

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "music-dl-desktop-go",
			Width:  1350,
			Height: 780,
			IconId: 2, // icon resource id
			Center: true,
		},
	})
	if w == nil {
		log.Fatalln("Failed to load webview.")
	}
	defer w.Destroy()
	if err := w.Bind("musicDlOpenDesktopLyrics", func() error {
		desktopLyrics.Open()
		return nil
	}); err != nil {
		log.Printf("failed to bind desktop lyrics opener: %v", err)
	}
	w.SetSize(1350, 780, webview2.Hint(webview2.HintNone))
	w.Navigate("http://localhost:37777/music/")
	w.Run()
}
