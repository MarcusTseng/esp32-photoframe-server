package publicart

import (
	"log"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"gorm.io/gorm"
)

// gormDedupHistoryDB implements DedupHistoryDB using GORM.
type gormDedupHistoryDB struct {
	db *gorm.DB
}

// NewDedupHistoryDB creates a DedupHistoryDB backed by GORM.
func NewDedupHistoryDB(db *gorm.DB) DedupHistoryDB {
	return &gormDedupHistoryDB{db: db}
}

// Record inserts a serving history entry.
func (d *gormDedupHistoryDB) Record(deviceID uint, source, artworkID string) {
	entry := model.PublicArtServingHistory{
		DeviceID:  deviceID,
		Source:    source,
		ArtworkID: artworkID,
		ServedAt:  time.Now(),
	}
	if err := d.db.Create(&entry).Error; err != nil {
		// Dedup is best-effort — log and continue
		log.Printf("publicart: failed to record serving history: %v", err)
	}
}

// IsRecentlyServed returns true if this (device, source, artwork) tuple
// has been served within the given deduplication window (hours).
func (d *gormDedupHistoryDB) IsRecentlyServed(deviceID uint, source, artworkID string, hours int) bool {
	if hours <= 0 {
		return false
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	var count int64
	err := d.db.Model(&model.PublicArtServingHistory{}).
		Where("device_id = ? AND source = ? AND artwork_id = ? AND served_at > ?",
			deviceID, source, artworkID, cutoff).
		Count(&count).Error
	if err != nil {
		log.Printf("publicart: dedup check failed: %v", err)
		return false
	}
	return count > 0
}

// Cleanup removes history entries older than the deduplication window for the given device.
func (d *gormDedupHistoryDB) Cleanup(deviceID uint, hours int) {
	if hours <= 0 {
		// Cleanup disabled — only delete when explicitly requested with a positive window
		return
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	if err := d.db.Where("device_id = ? AND served_at < ?", deviceID, cutoff).
		Delete(&model.PublicArtServingHistory{}).Error; err != nil {
		log.Printf("publicart: failed to cleanup old history entries: %v", err)
	}
}