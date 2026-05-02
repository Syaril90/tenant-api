package documents

import "time"

type Document struct {
	ID              uint   `gorm:"primaryKey"`
	PublicID        string `gorm:"uniqueIndex;size:64;not null"`
	Status          string `gorm:"size:32;not null"`
	AudienceScope   string `gorm:"size:32;not null"`
	BuildingCode    string `gorm:"size:64"`
	BuildingName    string `gorm:"size:255"`
	CategoryID      string `gorm:"size:64;not null"`
	CategoryLabel   string `gorm:"size:128;not null"`
	Title           string `gorm:"size:255;not null"`
	Description     string `gorm:"type:text;not null"`
	PreviewTitle    string `gorm:"size:255;not null"`
	PreviewBody     string `gorm:"type:text;not null"`
	StorageProvider string `gorm:"size:32;not null"`
	ObjectKey       string `gorm:"size:255;not null"`
	OriginalName    string `gorm:"size:255;not null"`
	MimeType        string `gorm:"size:128;not null"`
	SizeBytes       int64  `gorm:"not null"`
	PublicURL       string `gorm:"size:512;not null"`
	FileTypeLabel   string `gorm:"size:16;not null"`
	Tone            string `gorm:"size:16;not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
