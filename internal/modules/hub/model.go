package hub

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

type AuthorSnapshot struct {
	DisplayName    string `json:"displayName"`
	AvatarURL      string `json:"avatarUrl,omitempty"`
	BuildingName   string `json:"buildingName"`
	BuildingCode   string `json:"buildingCode"`
	UnitCode       string `json:"unitCode"`
	ResidentRole   string `json:"residentRole"`
	SecondaryLabel string `json:"secondaryLabel,omitempty"`
}

func (snapshot AuthorSnapshot) Value() (driver.Value, error) {
	return marshalJSONB(snapshot)
}

func (snapshot *AuthorSnapshot) Scan(value any) error {
	return scanJSONB(value, snapshot)
}

type MediaAsset struct {
	Kind            string `json:"kind"`
	StorageProvider string `json:"storageProvider"`
	ObjectKey       string `json:"objectKey"`
	OriginalName    string `json:"originalName"`
	MimeType        string `json:"mimeType"`
	SizeBytes       int64  `json:"sizeBytes"`
	PublicURL       string `json:"publicUrl"`
}

type MediaAssets []MediaAsset

func (assets MediaAssets) Value() (driver.Value, error) {
	return marshalJSONB(assets)
}

func (assets *MediaAssets) Scan(value any) error {
	return scanJSONB(value, assets)
}

type Post struct {
	ID             uint           `gorm:"primaryKey"`
	PublicID       string         `gorm:"uniqueIndex;size:64;not null"`
	PostType       string         `gorm:"index;size:32;not null;default:'post'"`
	AccountCode    string         `gorm:"index;size:64;not null"`
	ResidentCode   string         `gorm:"size:64;not null"`
	ResidentName   string         `gorm:"size:255;not null"`
	BuildingCode   string         `gorm:"index;size:64;not null"`
	BuildingName   string         `gorm:"size:255;not null"`
	UnitCode       string         `gorm:"index;size:64;not null"`
	Content        string         `gorm:"type:text;not null"`
	AuthorSnapshot AuthorSnapshot `gorm:"type:jsonb;not null"`
	Media          MediaAssets    `gorm:"type:jsonb;not null"`
	EventDetails   *EventDetails  `gorm:"type:jsonb"`
	LikeCount      int            `gorm:"not null;default:0"`
	ReplyCount     int            `gorm:"not null;default:0"`
	LastActivityAt time.Time      `gorm:"index;not null"`
	Replies        []Reply
	Likes          []PostLike
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type EventDetails struct {
	Title        string `json:"title"`
	Location     string `json:"location"`
	StartsAtISO  string `json:"startsAtIso"`
	StartsAtText string `json:"startsAtText"`
	EndsAtISO    string `json:"endsAtIso,omitempty"`
	EndsAtText   string `json:"endsAtText,omitempty"`
}

func (details EventDetails) Value() (driver.Value, error) {
	return marshalJSONB(details)
}

func (details *EventDetails) Scan(value any) error {
	return scanJSONB(value, details)
}

type Reply struct {
	ID             uint           `gorm:"primaryKey"`
	PublicID       string         `gorm:"uniqueIndex;size:64;not null"`
	PostID         uint           `gorm:"index;not null"`
	AccountCode    string         `gorm:"index;size:64;not null"`
	ResidentCode   string         `gorm:"size:64;not null"`
	ResidentName   string         `gorm:"size:255;not null"`
	BuildingCode   string         `gorm:"index;size:64;not null"`
	BuildingName   string         `gorm:"size:255;not null"`
	UnitCode       string         `gorm:"index;size:64;not null"`
	Content        string         `gorm:"type:text;not null"`
	AuthorSnapshot AuthorSnapshot `gorm:"type:jsonb;not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type PostLike struct {
	ID           uint   `gorm:"primaryKey"`
	PostID       uint   `gorm:"uniqueIndex:idx_hub_post_like_post_account;index;not null"`
	AccountCode  string `gorm:"uniqueIndex:idx_hub_post_like_post_account;size:64;not null"`
	ResidentCode string `gorm:"size:64;not null"`
	BuildingCode string `gorm:"index;size:64;not null"`
	UnitCode     string `gorm:"index;size:64;not null"`
	CreatedAt    time.Time
}

func (Reply) TableName() string {
	return "hub_replies"
}

func (PostLike) TableName() string {
	return "hub_post_likes"
}

func marshalJSONB(value any) (driver.Value, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	return string(raw), nil
}

func scanJSONB(value any, target any) error {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		if len(typed) == 0 {
			return nil
		}
		return json.Unmarshal(typed, target)
	case string:
		if typed == "" {
			return nil
		}
		return json.Unmarshal([]byte(typed), target)
	default:
		return fmt.Errorf("unsupported jsonb type %T", value)
	}
}
