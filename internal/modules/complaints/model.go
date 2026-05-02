package complaints

import "time"

type Complaint struct {
	ID           uint   `gorm:"primaryKey"`
	PublicID     string `gorm:"uniqueIndex;size:64;not null"`
	Reference    string `gorm:"uniqueIndex;size:64;not null"`
	AccountCode  string `gorm:"index;size:64;not null"`
	ResidentCode string `gorm:"size:64;not null"`
	ResidentName string `gorm:"size:255;not null"`
	BuildingName string `gorm:"size:255;not null"`
	UnitCode     string `gorm:"index;size:64;not null"`
	Category     string `gorm:"size:64;not null"`
	Title        string `gorm:"size:255;not null"`
	Description  string `gorm:"type:text;not null"`
	Location     string `gorm:"size:255;not null"`
	Priority     string `gorm:"size:32;not null"`
	Status       string `gorm:"size:32;not null"`
	Attachments  []ComplaintAttachment
	Updates      []ComplaintUpdate
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ComplaintAttachment struct {
	ID          uint    `gorm:"primaryKey"`
	PublicID    string  `gorm:"uniqueIndex;size:64;not null"`
	ComplaintID uint    `gorm:"index;not null"`
	Title       string  `gorm:"size:255;not null"`
	Meta        string  `gorm:"size:255;not null"`
	Type        string  `gorm:"size:32;not null"`
	ObjectKey   *string `gorm:"size:255"`
	FileURL     *string `gorm:"size:1024"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (ComplaintAttachment) TableName() string {
	return "complaint_attachments"
}

type ComplaintUpdate struct {
	ID          uint   `gorm:"primaryKey"`
	PublicID    string `gorm:"uniqueIndex;size:64;not null"`
	ComplaintID uint   `gorm:"index;not null"`
	Status      string `gorm:"size:32;not null"`
	Title       string `gorm:"size:64;not null"`
	Comment     string `gorm:"type:text;not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (ComplaintUpdate) TableName() string {
	return "complaint_updates"
}
