package announcements

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"modular-api/internal/platform/apperrors"
	"modular-api/internal/platform/storage"

	"gorm.io/gorm"
)

type Module struct {
	db      *gorm.DB
	storage storage.FileStorage
}

func NewModule(db *gorm.DB, fileStorage storage.FileStorage) *Module {
	return &Module{db: db, storage: fileStorage}
}

type AttachmentView struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Meta    string `json:"meta"`
	Type    string `json:"type"`
	FileURL string `json:"fileUrl,omitempty"`
}

type Item struct {
	ID                   string           `json:"id"`
	Badge                string           `json:"badge"`
	BadgeTone            string           `json:"badgeTone"`
	Title                string           `json:"title"`
	Description          string           `json:"description"`
	PublishedAt          string           `json:"publishedAt"`
	Icon                 string           `json:"icon"`
	AccentColor          string           `json:"accentColor"`
	ImageURI             string           `json:"imageUri"`
	AffectedArea         string           `json:"affectedArea"`
	Schedule             string           `json:"schedule"`
	Contact              string           `json:"contact"`
	SummaryTitle         string           `json:"summaryTitle"`
	SummaryParagraphs    []string         `json:"summaryParagraphs"`
	HighlightedAreaTitle string           `json:"highlightedAreaTitle"`
	HighlightedAreaItems []string         `json:"highlightedAreaItems"`
	TimelineTitle        string           `json:"timelineTitle"`
	TimelineParagraphs   []string         `json:"timelineParagraphs"`
	ETALabel             string           `json:"etaLabel"`
	ETAValue             string           `json:"etaValue"`
	TeamLabel            string           `json:"teamLabel"`
	TeamValue            string           `json:"teamValue"`
	Attachments          []AttachmentView `json:"attachments"`
	SupportTitle         string           `json:"supportTitle"`
	SupportDescription   string           `json:"supportDescription"`
}

type CreateAnnouncementInput struct {
	Title             string
	Description       string
	BadgeTone         string
	AffectedArea      string
	EffectiveAt       string
	Contact           string
	ImageFile         multipart.File
	ImageHeader       *multipart.FileHeader
	AttachmentHeaders []*multipart.FileHeader
}

func (m *Module) ListPublished() ([]Item, error) {
	var announcements []Announcement
	if err := m.db.
		Preload("ImageAsset").
		Preload("Attachments").
		Where("status = ?", "published").
		Order("published_at desc").
		Find(&announcements).Error; err != nil {
		return nil, apperrors.Internal("list published announcements", err)
	}

	items := make([]Item, 0, len(announcements))
	for _, announcement := range announcements {
		items = append(items, mapAnnouncement(announcement))
	}

	return items, nil
}

func (m *Module) GetPublished(publicID string) (*Item, error) {
	var announcement Announcement
	if err := m.db.
		Preload("ImageAsset").
		Preload("Attachments").
		Where("public_id = ? AND status = ?", publicID, "published").
		First(&announcement).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("announcement not found")
		}
		return nil, apperrors.Internal("load published announcement", err)
	}

	item := mapAnnouncement(announcement)
	return &item, nil
}

func (m *Module) Create(ctx context.Context, input CreateAnnouncementInput) (*Item, error) {
	title := strings.TrimSpace(input.Title)
	description := strings.TrimSpace(input.Description)
	badgeTone := strings.TrimSpace(input.BadgeTone)
	affectedArea := strings.TrimSpace(input.AffectedArea)
	effectiveAtRaw := strings.TrimSpace(input.EffectiveAt)
	contact := strings.TrimSpace(input.Contact)

	if title == "" || description == "" || affectedArea == "" || effectiveAtRaw == "" || contact == "" {
		return nil, apperrors.Validation("title, description, affectedArea, effectiveAt, and contact are required")
	}

	if badgeTone != "danger" {
		badgeTone = "brand"
	}

	now := time.Now()
	effectiveAt, err := parseEffectiveAt(effectiveAtRaw)
	if err != nil {
		return nil, apperrors.Validation("effectiveAt must be a valid datetime")
	}

	publicID := fmt.Sprintf("announcement-%d", now.UnixMilli())
	record := Announcement{
		PublicID:              publicID,
		Status:                "published",
		AudienceScope:         "all_residents",
		Badge:                 badgeForTone(badgeTone),
		BadgeTone:             badgeTone,
		Title:                 title,
		Description:           description,
		Icon:                  iconForTone(badgeTone),
		AccentColor:           accentForTone(badgeTone),
		AffectedArea:          affectedArea,
		Schedule:              formatEffectiveDateTime(effectiveAt),
		Contact:               contact,
		SummaryTitle:          "Announcement Summary",
		SummaryParagraphsRaw:  description,
		HighlightTitle:        "Key Areas Impacted",
		HighlightItemsRaw:     affectedArea,
		TimelineTitle:         "Implementation Timeline",
		TimelineParagraphsRaw: fmt.Sprintf("This announcement takes effect on %s.", formatEffectiveDateTime(effectiveAt)),
		ETALabel:              "Effective Date",
		ETAValue:              formatEffectiveDateTime(effectiveAt),
		TeamLabel:             "Publishing Team",
		TeamValue:             "Management Office",
		SupportTitle:          "Need more clarification?",
		SupportDescription:    "Residents can contact the management office if they need help understanding the announcement scope or schedule.",
		ImageURL:              defaultImageURLForTone(badgeTone),
		PublishedAt:           now,
	}

	if input.ImageFile != nil && input.ImageHeader != nil {
		storedFile, err := m.storage.Save(ctx, storage.SaveFileInput{
			Folder:      "announcements",
			FileName:    input.ImageHeader.Filename,
			ContentType: input.ImageHeader.Header.Get("Content-Type"),
			Reader:      input.ImageFile,
		})
		if err != nil {
			return nil, apperrors.Internal("store announcement image", err)
		}

		asset := MediaAsset{
			PublicID:        fmt.Sprintf("media-%d", now.UnixNano()),
			StorageProvider: storedFile.StorageProvider,
			ObjectKey:       storedFile.ObjectKey,
			OriginalName:    storedFile.OriginalName,
			MimeType:        storedFile.MimeType,
			SizeBytes:       storedFile.SizeBytes,
			PublicURL:       storedFile.PublicURL,
		}
		if err := m.db.Create(&asset).Error; err != nil {
			return nil, apperrors.Internal("create announcement image asset", err)
		}

		record.ImageAssetID = &asset.ID
		record.ImageAsset = &asset
		record.ImageURL = storedFile.PublicURL
	}

	if err := m.db.Create(&record).Error; err != nil {
		return nil, apperrors.Internal("create announcement", err)
	}

	for index, header := range input.AttachmentHeaders {
		file, err := header.Open()
		if err != nil {
			return nil, apperrors.Internal("open announcement attachment", err)
		}

		storedFile, err := m.storage.Save(ctx, storage.SaveFileInput{
			Folder:      "announcements/attachments",
			FileName:    header.Filename,
			ContentType: header.Header.Get("Content-Type"),
			Reader:      file,
		})
		file.Close()
		if err != nil {
			return nil, apperrors.Internal("store announcement attachment", err)
		}

		attachment := Attachment{
			PublicID:       fmt.Sprintf("%s-attachment-%d", publicID, index+1),
			AnnouncementID: record.ID,
			Title:          header.Filename,
			Meta:           formatAttachmentMeta(storedFile.SizeBytes, fileTypeForName(header.Filename)),
			Type:           fileTypeForName(header.Filename),
			ObjectKey:      stringPointer(storedFile.ObjectKey),
			FileURL:        stringPointer(storedFile.PublicURL),
		}
		if err := m.db.Create(&attachment).Error; err != nil {
			return nil, apperrors.Internal("create announcement attachment", err)
		}
		record.Attachments = append(record.Attachments, attachment)
	}

	item := mapAnnouncement(record)
	return &item, nil
}

func mapAnnouncement(announcement Announcement) Item {
	attachments := make([]AttachmentView, 0, len(announcement.Attachments))
	for _, attachment := range announcement.Attachments {
		attachments = append(attachments, AttachmentView{
			ID:      attachment.PublicID,
			Title:   attachment.Title,
			Meta:    attachment.Meta,
			Type:    attachment.Type,
			FileURL: derefString(attachment.FileURL),
		})
	}

	imageURI := announcement.ImageURL
	if announcement.ImageAsset != nil && announcement.ImageAsset.PublicURL != "" {
		imageURI = announcement.ImageAsset.PublicURL
	}

	return Item{
		ID:                   announcement.PublicID,
		Badge:                announcement.Badge,
		BadgeTone:            announcement.BadgeTone,
		Title:                announcement.Title,
		Description:          announcement.Description,
		PublishedAt:          announcement.PublishedAt.Format("02 Jan 2006 • 03:04 PM"),
		Icon:                 announcement.Icon,
		AccentColor:          announcement.AccentColor,
		ImageURI:             imageURI,
		AffectedArea:         announcement.AffectedArea,
		Schedule:             announcement.Schedule,
		Contact:              announcement.Contact,
		SummaryTitle:         announcement.SummaryTitle,
		SummaryParagraphs:    splitMultiline(announcement.SummaryParagraphsRaw),
		HighlightedAreaTitle: announcement.HighlightTitle,
		HighlightedAreaItems: splitMultiline(announcement.HighlightItemsRaw),
		TimelineTitle:        announcement.TimelineTitle,
		TimelineParagraphs:   splitMultiline(announcement.TimelineParagraphsRaw),
		ETALabel:             announcement.ETALabel,
		ETAValue:             announcement.ETAValue,
		TeamLabel:            announcement.TeamLabel,
		TeamValue:            announcement.TeamValue,
		Attachments:          attachments,
		SupportTitle:         announcement.SupportTitle,
		SupportDescription:   announcement.SupportDescription,
	}
}

func splitMultiline(value string) []string {
	lines := strings.Split(value, "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func badgeForTone(badgeTone string) string {
	if badgeTone == "danger" {
		return "URGENT MAINTENANCE"
	}
	return "COMMUNITY UPDATE"
}

func iconForTone(badgeTone string) string {
	if badgeTone == "danger" {
		return "construct-outline"
	}
	return "megaphone-outline"
}

func accentForTone(badgeTone string) string {
	if badgeTone == "danger" {
		return "#E77A34"
	}
	return "#003178"
}

func defaultImageURLForTone(badgeTone string) string {
	if badgeTone == "danger" {
		return "https://images.unsplash.com/photo-1621905252507-b35492cc74b4?auto=format&fit=crop&w=1200&q=80"
	}
	return "https://images.unsplash.com/photo-1518005020951-eccb494ad742?auto=format&fit=crop&w=1200&q=80"
}

func parseEffectiveAt(value string) (time.Time, error) {
	if parsed, err := time.Parse("2006-01-02T15:04", value); err == nil {
		return parsed, nil
	}

	return time.Parse(time.RFC3339, value)
}

func formatEffectiveDateTime(value time.Time) string {
	return value.Format("02 Jan 2006 • 03:04 PM")
}

func fileTypeForName(fileName string) string {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".png", ".jpg", ".jpeg", ".webp":
		return "image"
	default:
		return "pdf"
	}
}

func formatAttachmentMeta(sizeBytes int64, fileType string) string {
	sizeKB := float64(sizeBytes) / 1024
	if sizeKB >= 1024 {
		return fmt.Sprintf("%.1f MB • %s", sizeKB/1024, attachmentLabel(fileType))
	}
	return fmt.Sprintf("%.0f KB • %s", sizeKB, attachmentLabel(fileType))
}

func attachmentLabel(fileType string) string {
	if fileType == "image" {
		return "Image"
	}
	return "PDF Document"
}

func stringPointer(value string) *string {
	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
