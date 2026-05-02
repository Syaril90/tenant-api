package feedback

import "time"

type Feedback struct {
	ID           uint   `gorm:"primaryKey"`
	PublicID     string `gorm:"uniqueIndex;size:64;not null"`
	AccountCode  string `gorm:"index;size:64;not null"`
	ResidentCode string `gorm:"size:64;not null"`
	ResidentName string `gorm:"size:255;not null"`
	BuildingName string `gorm:"size:255;not null"`
	UnitCode     string `gorm:"index;size:64;not null"`
	Type         string `gorm:"size:64;not null"`
	Rating       string `gorm:"size:32;not null"`
	Details      string `gorm:"type:text;not null"`
	Status       string `gorm:"size:32;not null"`
	Attachments  []FeedbackAttachment
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type FeedbackAttachment struct {
	ID         uint    `gorm:"primaryKey"`
	PublicID   string  `gorm:"uniqueIndex;size:64;not null"`
	FeedbackID uint    `gorm:"index;not null"`
	Title      string  `gorm:"size:255;not null"`
	Meta       string  `gorm:"size:255;not null"`
	Type       string  `gorm:"size:32;not null"`
	ObjectKey  *string `gorm:"size:255"`
	FileURL    *string `gorm:"size:1024"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (FeedbackAttachment) TableName() string {
	return "feedback_attachments"
}
