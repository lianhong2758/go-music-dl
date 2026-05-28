package web

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type desktopLyricsState struct {
	Active       bool   `json:"active"`
	Playing      bool   `json:"playing"`
	Name         string `json:"name"`
	Artist       string `json:"artist"`
	Album        string `json:"album"`
	Source       string `json:"source"`
	ID           string `json:"id"`
	LyricURL     string `json:"lyric_url"`
	LineLyricURL string `json:"line_lyric_url"`
	PositionMS   int64  `json:"position_ms"`
	DurationMS   int64  `json:"duration_ms"`
	ClientTimeMS int64  `json:"client_time_ms"`
	UpdatedAtMS  int64  `json:"updated_at_ms"`
}

var desktopLyricsStore = struct {
	sync.RWMutex
	state desktopLyricsState
}{}

func RegisterDesktopLyricsRoutes(api *gin.RouterGroup) {
	api.GET("/desktop_lyrics", func(c *gin.Context) {
		c.HTML(http.StatusOK, "desktop_lyrics.html", gin.H{
			"Root": RoutePrefix,
		})
	})
	api.GET("/desktop_lyrics.css", func(c *gin.Context) {
		c.FileFromFS("templates/static/css/desktop_lyrics.css", http.FS(templateFS))
	})
	api.GET("/desktop_lyrics.js", func(c *gin.Context) {
		c.FileFromFS("templates/static/js/desktop_lyrics.js", http.FS(templateFS))
	})
	api.GET("/desktop_lyrics/state", func(c *gin.Context) {
		desktopLyricsStore.RLock()
		state := desktopLyricsStore.state
		desktopLyricsStore.RUnlock()
		c.JSON(http.StatusOK, gin.H{
			"state":          state,
			"server_time_ms": time.Now().UnixMilli(),
		})
	})
	api.POST("/desktop_lyrics/state", func(c *gin.Context) {
		var state desktopLyricsState
		if err := c.ShouldBindJSON(&state); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid desktop lyrics state"})
			return
		}
		state.Name = strings.TrimSpace(state.Name)
		state.Artist = strings.TrimSpace(state.Artist)
		state.Album = strings.TrimSpace(state.Album)
		state.Source = strings.TrimSpace(state.Source)
		state.ID = strings.TrimSpace(state.ID)
		state.LyricURL = strings.TrimSpace(state.LyricURL)
		state.LineLyricURL = strings.TrimSpace(state.LineLyricURL)
		state.UpdatedAtMS = time.Now().UnixMilli()

		desktopLyricsStore.Lock()
		desktopLyricsStore.state = state
		desktopLyricsStore.Unlock()
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}
