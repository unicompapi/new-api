package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DownloadStat struct {
	Key   string `json:"key" gorm:"primaryKey;size:64"`
	Count int64  `json:"count" gorm:"not null;default:0"`
}

func GetDownloadCount(key string) (int64, error) {
	var stat DownloadStat
	err := DB.First(&stat, "key = ?", key).Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	return stat.Count, err
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
