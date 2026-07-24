package core

import (
	"strings"
	"time"

	"github.com/guohuiyuan/music-lib/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DownloadStatusSuccess = "success"
	DownloadStatusSkipped = "skipped"
	DownloadStatusFailed  = "failed"
)

// DownloadRecord keeps the user-visible download history in SQLite.
type DownloadRecord struct {
	ID        uint      `gorm:"primaryKey"`
	Name      string    `gorm:"size:512;not null;index"`
	Artist    string    `gorm:"size:512;not null;index"`
	Source    string    `gorm:"size:64;not null"`
	Status    string    `gorm:"size:32;not null;index"`
	Error     string    `gorm:"size:1024"`
	CreatedAt time.Time `gorm:"autoCreateTime;index"`
}

// DownloadDedupEntry is intentionally separate from the visible history. Clearing
// the history therefore does not make previously downloaded songs downloadable again.
type DownloadDedupEntry struct {
	SongKey   string    `gorm:"primaryKey;size:1024"`
	Name      string    `gorm:"size:512;not null"`
	Artist    string    `gorm:"size:512;not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func initDownloadRecordTable() error {
	if err := ensureConfigDB(); err != nil {
		return err
	}
	return configDB.AutoMigrate(&DownloadRecord{}, &DownloadDedupEntry{})
}

// SaveDownloadRecord persists one download outcome and records successful songs in
// the durable de-duplication index. Control characters are removed before writing.
func SaveDownloadRecord(name, artist, source, status, errStr string) error {
	if err := initDownloadRecordTable(); err != nil {
		return err
	}

	name = cleanDownloadRecordText(name)
	artist = cleanDownloadRecordText(artist)
	record := DownloadRecord{
		Name:   name,
		Artist: artist,
		Source: cleanDownloadRecordText(source),
		Status: cleanDownloadRecordText(status),
		Error:  cleanDownloadRecordText(errStr),
	}

	return configDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		if record.Status != DownloadStatusSuccess {
			return nil
		}

		return saveDownloadDedupEntry(tx, record.Name, record.Artist)
	})
}

// SaveDownloadDedupEntry records a song as already available locally without
// adding an item to the user-visible download history.
func SaveDownloadDedupEntry(name, artist string) error {
	if err := initDownloadRecordTable(); err != nil {
		return err
	}
	return saveDownloadDedupEntry(configDB, name, artist)
}

func saveDownloadDedupEntry(db *gorm.DB, name, artist string) error {
	name = cleanDownloadRecordText(name)
	artist = cleanDownloadRecordText(artist)
	entry := DownloadDedupEntry{
		SongKey: songKeyFromParts(name, artist),
		Name:    name,
		Artist:  artist,
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry).Error
}

func cleanDownloadRecordText(value string) string {
	return strings.TrimSpace(stripControl(value))
}

// GetDownloadRecords returns the most recent 200 user-visible download records.
func GetDownloadRecords() ([]DownloadRecord, error) {
	if err := initDownloadRecordTable(); err != nil {
		return nil, err
	}
	var records []DownloadRecord
	err := configDB.Order("created_at DESC").Limit(200).Find(&records).Error
	return records, err
}

// ClearDownloadRecords clears only the history displayed in the UI. The durable
// de-duplication index is retained so the download decision remains correct.
func ClearDownloadRecords() error {
	if err := initDownloadRecordTable(); err != nil {
		return err
	}
	return configDB.Where("1 = 1").Delete(&DownloadRecord{}).Error
}

// SongKey generates a stable de-duplication key from artist and title.
func SongKey(song *model.Song) string {
	artist := cleanDownloadRecordText(song.Artist)
	name := cleanDownloadRecordText(song.Name)
	if artist == "" {
		artist = "Unknown"
	}
	if name == "" {
		name = "Unknown"
	}
	return artist + " - " + name
}

func songKeyFromParts(name, artist string) string {
	return SongKey(&model.Song{Name: name, Artist: artist})
}

func stripControl(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= 0x20 && r != 0x7f {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func IsSongDownloaded(song *model.Song, dedupSet map[string]struct{}) bool {
	if dedupSet == nil {
		return false
	}
	_, exists := dedupSet[SongKey(song)]
	return exists
}

// LoadDownloadDedupSet loads the SQLite de-duplication index. On first use after
// upgrading, it migrates existing successful history rows into the new index.
func LoadDownloadDedupSet() (map[string]struct{}, error) {
	if err := initDownloadRecordTable(); err != nil {
		return nil, err
	}

	var entries []DownloadDedupEntry
	if err := configDB.Find(&entries).Error; err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		var records []DownloadRecord
		if err := configDB.Select("name", "artist").Where("status = ?", DownloadStatusSuccess).Find(&records).Error; err != nil {
			return nil, err
		}
		if len(records) > 0 {
			entries = make([]DownloadDedupEntry, 0, len(records))
			for _, record := range records {
				entries = append(entries, DownloadDedupEntry{
					SongKey: songKeyFromParts(record.Name, record.Artist),
					Name:    cleanDownloadRecordText(record.Name),
					Artist:  cleanDownloadRecordText(record.Artist),
				})
			}
			if err := configDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&entries).Error; err != nil {
				return nil, err
			}
		}
	}

	set := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if key := cleanDownloadRecordText(entry.SongKey); key != "" {
			set[key] = struct{}{}
		}
	}
	return set, nil
}

func CountSkippable(queue []model.Song, dedupSet map[string]struct{}) int {
	count := 0
	for _, song := range queue {
		if IsSongDownloaded(&song, dedupSet) {
			count++
		}
	}
	return count
}

func DownloadWithDedupCheck(song *model.Song, outDir string, withCover, withLyrics bool, dedupSet map[string]struct{}) (*DownloadedSong, error) {
	return DownloadWithDedupCheckWithTemplate(song, outDir, withCover, withLyrics, "", dedupSet)
}

func DownloadWithDedupCheckWithTemplate(song *model.Song, outDir string, withCover, withLyrics bool, filenameTemplate string, dedupSet map[string]struct{}) (*DownloadedSong, error) {
	key := SongKey(song)
	if IsSongDownloaded(song, dedupSet) {
		_ = SaveDownloadRecord(song.Name, song.Artist, song.Source, DownloadStatusSkipped, "")
		return &DownloadedSong{Skipped: true, Filename: key}, nil
	}

	var (
		result *DownloadedSong
		dlErr  error
	)
	if filenameTemplate == "" {
		result, dlErr = SaveSongToFile(song, outDir, withCover, withLyrics)
	} else {
		result, dlErr = SaveSongToFileWithTemplate(song, outDir, withCover, withLyrics, filenameTemplate)
	}
	if dlErr != nil {
		_ = SaveDownloadRecord(song.Name, song.Artist, song.Source, DownloadStatusFailed, dlErr.Error())
		return result, dlErr
	}

	_ = SaveDownloadRecord(song.Name, song.Artist, song.Source, DownloadStatusSuccess, "")
	if dedupSet != nil {
		dedupSet[key] = struct{}{}
	}
	return result, nil
}
