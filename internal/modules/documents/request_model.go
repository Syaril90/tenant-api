package documents

import "time"

type DocumentRequest struct {
	ID                   uint   `gorm:"primaryKey"`
	PublicID             string `gorm:"uniqueIndex;size:64;not null"`
	Reference            string `gorm:"uniqueIndex;size:64;not null"`
	AccountCode          string `gorm:"index;size:64;not null"`
	ResidentCode         string `gorm:"size:64;not null"`
	ResidentName         string `gorm:"size:255;not null"`
	BuildingCode         string `gorm:"size:64;not null"`
	BuildingName         string `gorm:"size:255;not null"`
	UnitCode             string `gorm:"index;size:64;not null"`
	RequestTypeID        string `gorm:"size:64;not null"`
	RequestTypeLabel     string `gorm:"size:255;not null"`
	PreferredFormatID    string `gorm:"size:64;not null"`
	PreferredFormatLabel string `gorm:"size:255;not null"`
	Purpose              string `gorm:"type:text;not null"`
	Notes                string `gorm:"type:text;not null"`
	Status               string `gorm:"size:32;not null"`
	LatestComment        string `gorm:"type:text;not null"`
	Attachments          []DocumentRequestAttachment
	Updates              []DocumentRequestUpdate
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type DocumentRequestAttachment struct {
	ID                uint    `gorm:"primaryKey"`
	PublicID          string  `gorm:"uniqueIndex;size:64;not null"`
	DocumentRequestID uint    `gorm:"index;not null"`
	UploadedBy        string  `gorm:"size:16;not null"`
	Title             string  `gorm:"size:255;not null"`
	Meta              string  `gorm:"size:255;not null"`
	Type              string  `gorm:"size:32;not null"`
	ObjectKey         *string `gorm:"size:255"`
	FileURL           *string `gorm:"size:1024"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (DocumentRequestAttachment) TableName() string {
	return "document_request_attachments"
}

type DocumentRequestUpdate struct {
	ID                uint   `gorm:"primaryKey"`
	PublicID          string `gorm:"uniqueIndex;size:64;not null"`
	DocumentRequestID uint   `gorm:"index;not null"`
	Status            string `gorm:"size:32;not null"`
	Title             string `gorm:"size:64;not null"`
	Comment           string `gorm:"type:text;not null"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (DocumentRequestUpdate) TableName() string {
	return "document_request_updates"
}
