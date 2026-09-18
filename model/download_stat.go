package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DownloadStat keeps the cumulative download counter of a downloadable product.
// Key identifies the product; Count is the number of times it was downloaded.
type DownloadStat struct {
	Key   string `json:"key" gorm:"primaryKey;size:64"`
	Count int64  `json:"count" gorm:"not null;default:0"`
}

func GetDownloadCount(key string) (int64, error) {
	var stat DownloadStat
	// Key is a reserved word in MySQL, so the condition has to let GORM quote
	// the column: a raw condition such as "key = ?" is emitted unquoted and
	// fails on MySQL. The map form quotes the column on all three dialects.
	err := DB.Where(map[string]interface{}{"key": key}).First(&stat).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return stat.Count, nil
}

func IncrementDownloadCount(key string) (int64, error) {
	stat := DownloadStat{Key: key, Count: 1}
	err := DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"count": gorm.Expr("download_stats.count + ?", 1),
		}),
	}).Create(&stat).Error
	if err != nil {
		return 0, err
	}
	return GetDownloadCount(key)
}
