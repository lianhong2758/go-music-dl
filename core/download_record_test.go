package core

import (
	"path/filepath"
	"testing"

	"github.com/guohuiyuan/music-lib/model"
)

func TestDownloadDedupIndexUsesSQLiteAndSurvivesHistoryClear(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("MUSIC_DL_CONFIG_DB", filepath.Join(baseDir, "settings.db"))
	resetConfigStateForTest()
	t.Cleanup(resetConfigStateForTest)

	if err := SaveDownloadRecord("Song\nTitle", "Artist\rName", "source", DownloadStatusSuccess, ""); err != nil {
		t.Fatalf("SaveDownloadRecord: %v", err)
	}

	dedupSet, err := LoadDownloadDedupSet()
	if err != nil {
		t.Fatalf("LoadDownloadDedupSet: %v", err)
	}
	song := &model.Song{Name: "SongTitle", Artist: "ArtistName"}
	if !IsSongDownloaded(song, dedupSet) {
		t.Fatal("successful record should be available through the SQLite de-duplication index")
	}

	if err := ClearDownloadRecords(); err != nil {
		t.Fatalf("ClearDownloadRecords: %v", err)
	}
	records, err := GetDownloadRecords()
	if err != nil {
		t.Fatalf("GetDownloadRecords: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("download history length = %d, want 0", len(records))
	}

	dedupSet, err = LoadDownloadDedupSet()
	if err != nil {
		t.Fatalf("LoadDownloadDedupSet after clear: %v", err)
	}
	if !IsSongDownloaded(song, dedupSet) {
		t.Fatal("clearing visible history must not clear the SQLite de-duplication index")
	}
}
