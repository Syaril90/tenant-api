package complaints

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

var allowedStatuses = map[string]bool{
	"received":    true,
	"in_progress": true,
	"done":        true,
}

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

type PreviewView struct {
	ID       string `json:"id"`
	ImageURL string `json:"imageUrl"`
}

type TimelineItemView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Timestamp   string `json:"timestamp"`
	IsCurrent   bool   `json:"isCurrent,omitempty"`
}

type Item struct {
	ID           string             `json:"id"`
	Reference    string             `json:"reference"`
	ResidentName string             `json:"residentName"`
	ResidentCode string             `json:"residentCode"`
	BuildingName string             `json:"buildingName"`
	UnitCode     string             `json:"unitCode"`
	Category     string             `json:"category"`
	Title        string             `json:"title"`
	Description  string             `json:"description"`
	Location     string             `json:"location"`
	Priority     string             `json:"priority"`
	Status       string             `json:"status"`
	SubmittedAt  string             `json:"submittedAt"`
	UpdatedAt    string             `json:"updatedAt"`
	LatestUpdate string             `json:"latestUpdate"`
	AssignedTeam string             `json:"assignedTeam"`
	Attachments  []AttachmentView   `json:"attachments"`
	Previews     []PreviewView      `json:"previews"`
	Timeline     []TimelineItemView `json:"timeline"`
}

type CreateComplaintInput struct {
	AccountCode       string
	UnitCode          string
	Category          string
	Title             string
	Description       string
	Location          string
	Priority          string
	AttachmentHeaders []*multipart.FileHeader
}

func (m *Module) List(unitCode string) ([]Item, error) {
	query := m.db.
		Preload("Attachments").
		Preload("Updates", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc")
		}).
		Order("updated_at desc")
	if strings.TrimSpace(unitCode) != "" {
		query = query.Where("unit_code = ?", strings.TrimSpace(unitCode))
	}

	var complaints []Complaint
	if err := query.Find(&complaints).Error; err != nil {
		return nil, apperrors.Internal("failed to list complaints", err)
	}

	items := make([]Item, 0, len(complaints))
	for _, complaint := range complaints {
		items = append(items, mapComplaint(complaint))
	}

	return items, nil
}

func (m *Module) Get(publicID string) (*Item, error) {
	var complaint Complaint
	if err := m.db.
		Preload("Attachments").
		Preload("Updates", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc")
		}).
		Where("public_id = ?", publicID).
		First(&complaint).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("complaint not found")
		}
		return nil, apperrors.Internal("failed to get complaint", err)
	}

	item := mapComplaint(complaint)
	return &item, nil
}

func (m *Module) Create(ctx context.Context, input CreateComplaintInput) (*Item, error) {
	accountCode := strings.TrimSpace(input.AccountCode)
	unitCode := strings.TrimSpace(input.UnitCode)
	category := strings.TrimSpace(input.Category)
	title := strings.TrimSpace(input.Title)
	description := strings.TrimSpace(input.Description)
	location := strings.TrimSpace(input.Location)
	priority := strings.TrimSpace(input.Priority)

	if accountCode == "" || unitCode == "" || category == "" || title == "" || description == "" || location == "" || priority == "" {
		return nil, apperrors.Validation("accountCode, unitCode, category, title, description, location, and priority are required")
	}

	resident, err := m.lookupResidentAccount(accountCode, unitCode)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("resident account not found")
		}
		return nil, err
	}

	now := time.Now()
	publicID := fmt.Sprintf("complaint-%d", now.UnixMilli())
	record := Complaint{
		PublicID:     publicID,
		Reference:    nextReference(now),
		AccountCode:  accountCode,
		ResidentCode: resident.Account.ResidentCode,
		ResidentName: resident.Account.ResidentName,
		BuildingName: resident.Building.Name,
		UnitCode:     resident.Unit.Code,
		Category:     strings.Title(strings.ReplaceAll(category, "_", " ")),
		Title:        title,
		Description:  description,
		Location:     location,
		Priority:     strings.Title(priority),
		Status:       "received",
	}

	if err := m.db.Create(&record).Error; err != nil {
		return nil, apperrors.Internal("failed to create complaint", err)
	}

	initialUpdate := ComplaintUpdate{
		PublicID:    fmt.Sprintf("%s-update-1", publicID),
		ComplaintID: record.ID,
		Status:      "received",
		Title:       "Received",
		Comment:     "Complaint received and queued for management review.",
	}
	if err := m.db.Create(&initialUpdate).Error; err != nil {
		return nil, apperrors.Internal("failed to create complaint update", err)
	}
	record.Updates = append(record.Updates, initialUpdate)

	for index, header := range input.AttachmentHeaders {
		file, err := header.Open()
		if err != nil {
			return nil, apperrors.Internal("failed to open complaint attachment", err)
		}

		storedFile, err := m.storage.Save(ctx, storage.SaveFileInput{
			Folder:      "complaints/attachments",
			FileName:    header.Filename,
			ContentType: header.Header.Get("Content-Type"),
			Reader:      file,
		})
		file.Close()
		if err != nil {
			return nil, apperrors.Internal("failed to store complaint attachment", err)
		}

		attachmentType := attachmentTypeFor(header.Filename, storedFile.MimeType)
		attachment := ComplaintAttachment{
			PublicID:    fmt.Sprintf("%s-attachment-%d", publicID, index+1),
			ComplaintID: record.ID,
			Title:       header.Filename,
			Meta:        formatAttachmentMeta(storedFile.SizeBytes, attachmentType),
			Type:        attachmentType,
			ObjectKey:   stringPointer(storedFile.ObjectKey),
			FileURL:     stringPointer(storedFile.PublicURL),
		}
		if err := m.db.Create(&attachment).Error; err != nil {
			return nil, apperrors.Internal("failed to save complaint attachment", err)
		}

		record.Attachments = append(record.Attachments, attachment)
	}

	item := mapComplaint(record)
	return &item, nil
}

func (m *Module) UpdateStatus(publicID, status, comment string) (*Item, error) {
	normalizedStatus := strings.TrimSpace(status)
	if !allowedStatuses[normalizedStatus] {
		return nil, apperrors.Validation("status must be one of: received, in_progress, done")
	}
	normalizedComment := strings.TrimSpace(comment)
	if normalizedComment == "" {
		return nil, apperrors.Validation("comment is required")
	}

	var complaint Complaint
	if err := m.db.
		Preload("Attachments").
		Preload("Updates", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc")
		}).
		Where("public_id = ?", publicID).
		First(&complaint).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("complaint not found")
		}
		return nil, apperrors.Internal("failed to get complaint", err)
	}

	complaint.Status = normalizedStatus
	if err := m.db.Save(&complaint).Error; err != nil {
		return nil, apperrors.Internal("failed to update complaint status", err)
	}

	update := ComplaintUpdate{
		PublicID:    fmt.Sprintf("%s-update-%d", complaint.PublicID, time.Now().UnixMilli()),
		ComplaintID: complaint.ID,
		Status:      normalizedStatus,
		Title:       timelineTitleForStatus(normalizedStatus),
		Comment:     normalizedComment,
	}
	if err := m.db.Create(&update).Error; err != nil {
		return nil, apperrors.Internal("failed to save complaint update", err)
	}
	complaint.Updates = append([]ComplaintUpdate{update}, complaint.Updates...)

	item := mapComplaint(complaint)
	return &item, nil
}

type residentLookup struct {
	Account  property.ResidentAccount
	Unit     property.Unit
	Building property.Building
}

func (m *Module) lookupResidentAccount(accountCode, unitCode string) (*residentLookup, error) {
	var account property.ResidentAccount
	if err := m.db.Where("account_code = ?", accountCode).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("resident account not found")
		}
		return nil, apperrors.Internal("failed to find resident account", err)
	}

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

func mapComplaint(complaint Complaint) Item {
	attachments := make([]AttachmentView, 0, len(complaint.Attachments))
	previews := make([]PreviewView, 0)

	for _, attachment := range complaint.Attachments {
		fileURL := derefString(attachment.FileURL)
		attachments = append(attachments, AttachmentView{
			ID:      attachment.PublicID,
			Title:   attachment.Title,
			Meta:    attachment.Meta,
			Type:    attachment.Type,
			FileURL: fileURL,
		})
		if attachment.Type == "image" && fileURL != "" {
			previews = append(previews, PreviewView{
				ID:       attachment.PublicID,
				ImageURL: fileURL,
			})
		}
	}

	timeline := buildTimeline(complaint)

	return Item{
		ID:           complaint.PublicID,
		Reference:    complaint.Reference,
		ResidentName: complaint.ResidentName,
		ResidentCode: complaint.ResidentCode,
		BuildingName: complaint.BuildingName,
		UnitCode:     complaint.UnitCode,
		Category:     complaint.Category,
		Title:        complaint.Title,
		Description:  complaint.Description,
		Location:     complaint.Location,
		Priority:     complaint.Priority,
		Status:       complaint.Status,
		SubmittedAt:  complaint.CreatedAt.Format("02 Jan 2006 • 03:04 PM"),
		UpdatedAt:    complaint.UpdatedAt.Format("02 Jan 2006 • 03:04 PM"),
		LatestUpdate: latestUpdateLabel(complaint),
		AssignedTeam: "Management Office",
		Attachments:  attachments,
		Previews:     previews,
		Timeline:     timeline,
	}
}

func nextReference(now time.Time) string {
	return fmt.Sprintf("CMP-%06d", now.UnixMilli()%1000000)
}

func latestUpdateLabel(complaint Complaint) string {
	if len(complaint.Updates) > 0 {
		return complaint.Updates[0].Comment
	}

	switch complaint.Status {
	case "done":
		return "Complaint marked done by management."
	case "in_progress":
		return "Complaint is currently under action by the management team."
	default:
		return "Complaint received and queued for management review."
	}
}

func buildTimeline(complaint Complaint) []TimelineItemView {
	items := make([]TimelineItemView, 0, len(complaint.Updates)+1)
	for index, update := range complaint.Updates {
		items = append(items, TimelineItemView{
			ID:          update.PublicID,
			Title:       update.Title,
			Description: update.Comment,
			Timestamp:   update.CreatedAt.Format("02 Jan 2006 • 03:04 PM"),
			IsCurrent:   index == 0,
		})
	}

	if len(items) == 0 {
		items = append(items, TimelineItemView{
			ID:          fmt.Sprintf("%s-submitted", complaint.PublicID),
			Title:       "Submitted",
			Description: "Resident submitted the complaint to the management office.",
			Timestamp:   complaint.CreatedAt.Format("02 Jan 2006 • 03:04 PM"),
			IsCurrent:   true,
		})
	}

	return items
}

func timelineTitleForStatus(status string) string {
	switch status {
	case "done":
		return "Done"
	case "in_progress":
		return "In Progress"
	default:
		return "Received"
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

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
