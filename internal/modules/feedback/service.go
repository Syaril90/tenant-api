package feedback

import (
	"context"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"modular-api/internal/modules/property"
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
	ID           string           `json:"id"`
	AccountCode  string           `json:"accountCode"`
	ResidentCode string           `json:"residentCode"`
	ResidentName string           `json:"residentName"`
	BuildingName string           `json:"buildingName"`
	UnitCode     string           `json:"unitCode"`
	Type         string           `json:"type"`
	Rating       string           `json:"rating"`
	Details      string           `json:"details"`
	Status       string           `json:"status"`
	SubmittedAt  string           `json:"submittedAt"`
	UpdatedAt    string           `json:"updatedAt"`
	Attachments  []AttachmentView `json:"attachments"`
}

type CreateFeedbackInput struct {
	AccountCode       string
	UnitCode          string
	Type              string
	Rating            string
	Details           string
	AttachmentHeaders []*multipart.FileHeader
}

func (m *Module) List(unitCode string) ([]Item, error) {
	query := m.db.Preload("Attachments").Order("updated_at desc")
	if strings.TrimSpace(unitCode) != "" {
		query = query.Where("unit_code = ?", strings.TrimSpace(unitCode))
	}

	var feedbackItems []Feedback
	if err := query.Find(&feedbackItems).Error; err != nil {
		return nil, apperrors.Internal("failed to list feedback", err)
	}

	items := make([]Item, 0, len(feedbackItems))
	for _, feedbackItem := range feedbackItems {
		items = append(items, mapFeedback(feedbackItem))
	}

	return items, nil
}

func (m *Module) Get(publicID string) (*Item, error) {
	var feedbackItem Feedback
	if err := m.db.Preload("Attachments").Where("public_id = ?", publicID).First(&feedbackItem).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("feedback not found")
		}
		return nil, apperrors.Internal("failed to get feedback", err)
	}

	item := mapFeedback(feedbackItem)
	return &item, nil
}

func (m *Module) Create(ctx context.Context, input CreateFeedbackInput) (*Item, error) {
	unitCode := strings.TrimSpace(input.UnitCode)
	feedbackType := strings.TrimSpace(input.Type)
	rating := strings.TrimSpace(input.Rating)
	details := strings.TrimSpace(input.Details)

	if unitCode == "" || feedbackType == "" || rating == "" || details == "" {
		return nil, apperrors.Validation("unitCode, type, rating, and details are required")
	}

	resident, err := m.lookupResidentAccount(strings.TrimSpace(input.AccountCode), unitCode)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	publicID := fmt.Sprintf("feedback-%d", now.UnixMilli())
	record := Feedback{
		PublicID:     publicID,
		AccountCode:  resident.Account.AccountCode,
		ResidentCode: resident.Account.ResidentCode,
		ResidentName: resident.Account.ResidentName,
		BuildingName: resident.Building.Name,
		UnitCode:     resident.Unit.Code,
		Type:         strings.Title(strings.ReplaceAll(feedbackType, "_", " ")),
		Rating:       strings.Title(strings.ReplaceAll(rating, "_", " ")),
		Details:      details,
		Status:       "submitted",
	}

	if err := m.db.Create(&record).Error; err != nil {
		return nil, apperrors.Internal("failed to create feedback", err)
	}

	for index, header := range input.AttachmentHeaders {
		file, err := header.Open()
		if err != nil {
			return nil, apperrors.Internal("failed to open feedback attachment", err)
		}

		storedFile, err := m.storage.Save(ctx, storage.SaveFileInput{
			Folder:      "feedback/attachments",
			FileName:    header.Filename,
			ContentType: header.Header.Get("Content-Type"),
			Reader:      file,
		})
		file.Close()
		if err != nil {
			return nil, apperrors.Internal("failed to store feedback attachment", err)
		}

		attachmentType := attachmentTypeFor(header.Filename, storedFile.MimeType)
		attachment := FeedbackAttachment{
			PublicID:   fmt.Sprintf("%s-attachment-%d", publicID, index+1),
			FeedbackID: record.ID,
			Title:      header.Filename,
			Meta:       formatAttachmentMeta(storedFile.SizeBytes, attachmentType),
			Type:       attachmentType,
			ObjectKey:  stringPointer(storedFile.ObjectKey),
			FileURL:    stringPointer(storedFile.PublicURL),
		}
		if err := m.db.Create(&attachment).Error; err != nil {
			return nil, apperrors.Internal("failed to save feedback attachment", err)
		}

		record.Attachments = append(record.Attachments, attachment)
	}

	item := mapFeedback(record)
	return &item, nil
}

type residentLookup struct {
	Account  property.ResidentAccount
	Unit     property.Unit
	Building property.Building
}

func (m *Module) lookupResidentAccount(accountCode, unitCode string) (*residentLookup, error) {
	if accountCode != "" {
		resident, err := m.lookupResidentByAccountCode(accountCode, unitCode)
		if err == nil {
			return resident, nil
		}
		if err != gorm.ErrRecordNotFound {
			return nil, err
		}
	}

	return m.lookupResidentByUnitCode(unitCode)
}

func (m *Module) lookupResidentByAccountCode(accountCode, unitCode string) (*residentLookup, error) {
	var account property.ResidentAccount
	if err := m.db.Where("account_code = ?", accountCode).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("resident account not found")
		}
		return nil, apperrors.Internal("failed to find resident account", err)
	}

	return m.lookupResidentFromAccount(account, unitCode)
}

func (m *Module) lookupResidentByUnitCode(unitCode string) (*residentLookup, error) {
	var unit property.Unit
	if err := m.db.Where("code = ?", unitCode).First(&unit).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("resident account not found")
		}
		return nil, apperrors.Internal("failed to find unit", err)
	}

	var account property.ResidentAccount
	if err := m.db.Where("unit_id = ?", unit.ID).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("resident account not found")
		}
		return nil, apperrors.Internal("failed to find resident account", err)
	}

	return m.lookupResidentFromAccount(account, unitCode)
}

func (m *Module) lookupResidentFromAccount(account property.ResidentAccount, unitCode string) (*residentLookup, error) {
	var unit property.Unit
	if err := m.db.Where("id = ? AND code = ?", account.UnitID, unitCode).First(&unit).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("resident account not found")
		}
		return nil, apperrors.Internal("failed to match unit for resident account", err)
	}

	var area property.Area
	if err := m.db.Where("id = ?", unit.AreaID).First(&area).Error; err != nil {
		return nil, apperrors.Internal("failed to load area", err)
	}

	var building property.Building
	if err := m.db.Where("id = ?", area.BuildingID).First(&building).Error; err != nil {
		return nil, apperrors.Internal("failed to load building", err)
	}

	return &residentLookup{
		Account:  account,
		Unit:     unit,
		Building: building,
	}, nil
}

func mapFeedback(feedbackItem Feedback) Item {
	attachments := make([]AttachmentView, 0, len(feedbackItem.Attachments))
	for _, attachment := range feedbackItem.Attachments {
		attachments = append(attachments, AttachmentView{
			ID:      attachment.PublicID,
			Title:   attachment.Title,
			Meta:    attachment.Meta,
			Type:    attachment.Type,
			FileURL: derefString(attachment.FileURL),
		})
	}

	return Item{
		ID:           feedbackItem.PublicID,
		AccountCode:  feedbackItem.AccountCode,
		ResidentCode: feedbackItem.ResidentCode,
		ResidentName: feedbackItem.ResidentName,
		BuildingName: feedbackItem.BuildingName,
		UnitCode:     feedbackItem.UnitCode,
		Type:         feedbackItem.Type,
		Rating:       feedbackItem.Rating,
		Details:      feedbackItem.Details,
		Status:       feedbackItem.Status,
		SubmittedAt:  feedbackItem.CreatedAt.Format("02 Jan 2006 • 03:04 PM"),
		UpdatedAt:    feedbackItem.UpdatedAt.Format("02 Jan 2006 • 03:04 PM"),
		Attachments:  attachments,
	}
}

func attachmentTypeFor(fileName, mimeType string) string {
	if strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return "image"
	}
	if strings.HasPrefix(strings.ToLower(mimeType), "video/") {
		return "video"
	}

	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".heic":
		return "image"
	case ".mp4", ".mov", ".avi":
		return "video"
	default:
		return "document"
	}
}

func formatAttachmentMeta(sizeBytes int64, attachmentType string) string {
	sizeLabel := fmt.Sprintf("%d KB", maxInt64(1, (sizeBytes+1023)/1024))
	typeLabel := map[string]string{
		"image":    "Image",
		"video":    "Video",
		"document": "Document",
	}[attachmentType]
	if typeLabel == "" {
		typeLabel = "File"
	}

	return fmt.Sprintf("%s • %s", typeLabel, sizeLabel)
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
