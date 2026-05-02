package announcements

import "time"

type MediaAsset struct {
	ID              uint   `gorm:"primaryKey"`
	PublicID        string `gorm:"uniqueIndex;size:64;not null"`
	StorageProvider string `gorm:"size:32;not null"`
	ObjectKey       string `gorm:"size:255;not null"`
	OriginalName    string `gorm:"size:255;not null"`
	MimeType        string `gorm:"size:128;not null"`
	SizeBytes       int64  `gorm:"not null"`
	PublicURL       string `gorm:"size:512;not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Announcement struct {
	ID                    uint   `gorm:"primaryKey"`
	PublicID              string `gorm:"uniqueIndex;size:64;not null"`
	Status                string `gorm:"size:32;not null"`
	AudienceScope         string `gorm:"size:32;not null"`
	BuildingCode          string `gorm:"size:64"`
	Badge                 string `gorm:"size:128;not null"`
	BadgeTone             string `gorm:"size:32;not null"`
	Title                 string `gorm:"size:255;not null"`
	Description           string `gorm:"type:text;not null"`
	Icon                  string `gorm:"size:64;not null"`
	AccentColor           string `gorm:"size:32;not null"`
	AffectedArea          string `gorm:"type:text;not null"`
	Schedule              string `gorm:"type:text;not null"`
	Contact               string `gorm:"size:255;not null"`
	SummaryTitle          string `gorm:"size:255;not null"`
	SummaryParagraphsRaw  string `gorm:"type:text;not null"`
	HighlightTitle        string `gorm:"size:255;not null"`
	HighlightItemsRaw     string `gorm:"type:text;not null"`
	TimelineTitle         string `gorm:"size:255;not null"`
	TimelineParagraphsRaw string `gorm:"type:text;not null"`
	ETALabel              string `gorm:"size:255;not null"`
	ETAValue              string `gorm:"size:255;not null"`
	TeamLabel             string `gorm:"size:255;not null"`
	TeamValue             string `gorm:"size:255;not null"`
	SupportTitle          string `gorm:"size:255;not null"`
	SupportDescription    string `gorm:"type:text;not null"`
	ImageAssetID          *uint  `gorm:"index"`
	ImageAsset            *MediaAsset
	ImageURL              string `gorm:"size:512;not null"`
	Attachments           []Attachment
	PublishedAt           time.Time `gorm:"index;not null"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type Attachment struct {
	ID             uint    `gorm:"primaryKey"`
	PublicID       string  `gorm:"uniqueIndex;size:64;not null"`
	AnnouncementID uint    `gorm:"index;not null"`
	Title          string  `gorm:"size:255;not null"`
	Meta           string  `gorm:"size:255;not null"`
	Type           string  `gorm:"size:32;not null"`
	ObjectKey      *string `gorm:"size:255"`
	FileURL        *string `gorm:"size:512"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
