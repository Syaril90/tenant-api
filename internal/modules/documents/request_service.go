package documents

import (
	"context"
	"errors"
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

var allowedDocumentRequestStatuses = map[string]bool{
	"submitted": true,
	"in_review": true,
	"fulfilled": true,
	"rejected":  true,
}

type DocumentRequestAttachmentView struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Meta       string `json:"meta"`
	Type       string `json:"type"`
	FileURL    string `json:"fileUrl,omitempty"`
	UploadedBy string `json:"uploadedBy"`
}

type DocumentRequestTimelineItemView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Timestamp   string `json:"timestamp"`
	IsCurrent   bool   `json:"isCurrent,omitempty"`
}

type DocumentRequestItem struct {
	ID                   string                            `json:"id"`
	Reference            string                            `json:"reference"`
	ResidentName         string                            `json:"residentName"`
	ResidentCode         string                            `json:"residentCode"`
	BuildingCode         string                            `json:"buildingCode"`
	BuildingName         string                            `json:"buildingName"`
	UnitCode             string                            `json:"unitCode"`
	RequestTypeID        string                            `json:"requestTypeId"`
	RequestTypeLabel     string                            `json:"requestTypeLabel"`
	PreferredFormatID    string                            `json:"preferredFormatId"`
	PreferredFormatLabel string                            `json:"preferredFormatLabel"`
	Purpose              string                            `json:"purpose"`
	Notes                string                            `json:"notes"`
	Status               string                            `json:"status"`
	LatestComment        string                            `json:"latestComment"`
	SubmittedAt          string                            `json:"submittedAt"`
	UpdatedAt            string                            `json:"updatedAt"`
	Attachments          []DocumentRequestAttachmentView   `json:"attachments"`
	Timeline             []DocumentRequestTimelineItemView `json:"timeline"`
}

type DocumentRequestListPayload struct {
	Items []DocumentRequestItem `json:"items"`
}

type CreateDocumentRequestInput struct {
	AccountCode       string
	UnitCode          string
	RequestTypeID     string
	Purpose           string
	PreferredFormatID string
	Notes             string
	AttachmentHeaders []*multipart.FileHeader
}

type UpdateDocumentRequestInput struct {
	Status            string
	Comment           string
	AttachmentHeaders []*multipart.FileHeader
}

type documentRequestOption struct {
	ID    string
	Label string
}

type requestResidentLookup struct {
	Account  property.ResidentAccount
	Unit     property.Unit
	Building property.Building
}

func (m *Module) ListAdminRequests() ([]DocumentRequestItem, error) {
	var requests []DocumentRequest
	if err := m.db.
		Preload("Attachments", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc")
		}).
		Preload("Updates", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc")
		}).
		Order("updated_at desc").
		Find(&requests).Error; err != nil {
		return nil, apperrors.Internal("list document requests", err)
	}

	items := make([]DocumentRequestItem, 0, len(requests))
	for _, request := range requests {
		items = append(items, mapDocumentRequest(request))
	}

	return items, nil
}

func (m *Module) ListResidentRequests(accountCode, unitCode string) ([]DocumentRequestItem, error) {
	normalizedAccountCode := strings.TrimSpace(accountCode)
	normalizedUnitCode := strings.TrimSpace(unitCode)

	if normalizedAccountCode == "" || normalizedUnitCode == "" {
		return nil, apperrors.Validation("accountCode and unitCode query parameters are required")
	}

	var requests []DocumentRequest
	if err := m.db.
		Preload("Attachments", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc")
		}).
		Preload("Updates", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc")
		}).
		Where("account_code = ? AND unit_code = ?", normalizedAccountCode, normalizedUnitCode).
		Order("updated_at desc").
		Find(&requests).Error; err != nil {
		return nil, apperrors.Internal("list resident document requests", err)
	}

	items := make([]DocumentRequestItem, 0, len(requests))
	for _, request := range requests {
		items = append(items, mapDocumentRequest(request))
	}

	return items, nil
}

func (m *Module) CreateRequest(ctx context.Context, input CreateDocumentRequestInput) (*DocumentRequestItem, error) {
	accountCode := strings.TrimSpace(input.AccountCode)
	unitCode := strings.TrimSpace(input.UnitCode)
	requestTypeID := strings.TrimSpace(input.RequestTypeID)
	purpose := strings.TrimSpace(input.Purpose)
	preferredFormatID := strings.TrimSpace(input.PreferredFormatID)
	notes := strings.TrimSpace(input.Notes)

	if accountCode == "" || unitCode == "" || requestTypeID == "" || purpose == "" || preferredFormatID == "" {
		return nil, apperrors.Validation("accountCode, unitCode, typeId, purpose, and preferredFormatId are required")
	}

	requestType, ok := documentRequestTypeByID(requestTypeID)
	if !ok {
		return nil, apperrors.Validationf("unknown typeId: %s", requestTypeID)
	}

	preferredFormat, ok := documentRequestFormatByID(preferredFormatID)
	if !ok {
		return nil, apperrors.Validationf("unknown preferredFormatId: %s", preferredFormatID)
	}

	resident, err := m.lookupRequestResidentAccount(accountCode, unitCode)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	publicID := fmt.Sprintf("document-request-%d", now.UnixMilli())
	record := DocumentRequest{
		PublicID:             publicID,
		Reference:            nextDocumentRequestReference(now),
		AccountCode:          accountCode,
		ResidentCode:         resident.Account.ResidentCode,
		ResidentName:         resident.Account.ResidentName,
		BuildingCode:         resident.Building.Code,
		BuildingName:         resident.Building.Name,
		UnitCode:             resident.Unit.Code,
		RequestTypeID:        requestType.ID,
		RequestTypeLabel:     requestType.Label,
		PreferredFormatID:    preferredFormat.ID,
		PreferredFormatLabel: preferredFormat.Label,
		Purpose:              purpose,
		Notes:                notes,
		Status:               "submitted",
		LatestComment:        "Request submitted and queued for management review.",
	}

	if err := m.db.Create(&record).Error; err != nil {
		return nil, apperrors.Internal("create document request", err)
	}

	initialUpdate := DocumentRequestUpdate{
		PublicID:          fmt.Sprintf("%s-update-1", publicID),
		DocumentRequestID: record.ID,
		Status:            "submitted",
		Title:             "Submitted",
		Comment:           record.LatestComment,
	}
	if err := m.db.Create(&initialUpdate).Error; err != nil {
		return nil, apperrors.Internal("create document request update", err)
	}
	record.Updates = append(record.Updates, initialUpdate)

	for index, header := range input.AttachmentHeaders {
		attachment, err := m.saveDocumentRequestAttachment(ctx, record.ID, publicID, "resident", index+1, header)
		if err != nil {
			return nil, err
		}

		record.Attachments = append(record.Attachments, attachment)
	}

	item := mapDocumentRequest(record)
	return &item, nil
}

func (m *Module) UpdateRequest(ctx context.Context, publicID string, input UpdateDocumentRequestInput) (*DocumentRequestItem, error) {
	normalizedStatus := strings.TrimSpace(input.Status)
	normalizedComment := strings.TrimSpace(input.Comment)

	if !allowedDocumentRequestStatuses[normalizedStatus] {
		return nil, apperrors.Validation("status must be one of: submitted, in_review, fulfilled, rejected")
	}
	if normalizedComment == "" {
		return nil, apperrors.Validation("comment is required")
	}

	var request DocumentRequest
	if err := m.db.
		Preload("Attachments", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc")
		}).
		Preload("Updates", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at desc")
		}).
		Where("public_id = ?", strings.TrimSpace(publicID)).
		First(&request).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("document request not found")
		}
		return nil, apperrors.Internal("load document request", err)
	}

	request.Status = normalizedStatus
	request.LatestComment = normalizedComment
	if err := m.db.Save(&request).Error; err != nil {
		return nil, apperrors.Internal("update document request", err)
	}

	update := DocumentRequestUpdate{
		PublicID:          fmt.Sprintf("%s-update-%d", request.PublicID, time.Now().UnixMilli()),
		DocumentRequestID: request.ID,
		Status:            normalizedStatus,
		Title:             documentRequestTimelineTitle(normalizedStatus),
		Comment:           normalizedComment,
	}
	if err := m.db.Create(&update).Error; err != nil {
		return nil, apperrors.Internal("save document request update", err)
	}
	request.Updates = append([]DocumentRequestUpdate{update}, request.Updates...)

	for index, header := range input.AttachmentHeaders {
		attachment, err := m.saveDocumentRequestAttachment(ctx, request.ID, request.PublicID, "admin", index+1, header)
		if err != nil {
			return nil, err
		}

		request.Attachments = append([]DocumentRequestAttachment{attachment}, request.Attachments...)
	}

	item := mapDocumentRequest(request)
	return &item, nil
}

func (m *Module) saveDocumentRequestAttachment(
	ctx context.Context,
	requestID uint,
	requestPublicID string,
	uploadedBy string,
	index int,
	header *multipart.FileHeader,
) (DocumentRequestAttachment, error) {
	file, err := header.Open()
	if err != nil {
		return DocumentRequestAttachment{}, apperrors.Internal("open document request attachment", err)
	}

	storedFile, err := m.storage.Save(ctx, storage.SaveFileInput{
		Folder:      "document-requests/attachments",
		FileName:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		Reader:      file,
	})
	file.Close()
	if err != nil {
		return DocumentRequestAttachment{}, apperrors.Internal("store document request attachment", err)
	}

	attachmentType := documentRequestAttachmentTypeFor(header.Filename, storedFile.MimeType)
	attachment := DocumentRequestAttachment{
		PublicID:          fmt.Sprintf("%s-attachment-%s-%d-%d", requestPublicID, uploadedBy, time.Now().UnixMilli(), index),
		DocumentRequestID: requestID,
		UploadedBy:        uploadedBy,
		Title:             header.Filename,
		Meta:              fmt.Sprintf("%s • %s", strings.Title(uploadedBy), formatFileSize(storedFile.SizeBytes)),
		Type:              attachmentType,
		ObjectKey:         stringPointer(storedFile.ObjectKey),
		FileURL:           stringPointer(storedFile.PublicURL),
	}
	if err := m.db.Create(&attachment).Error; err != nil {
		return DocumentRequestAttachment{}, apperrors.Internal("save document request attachment", err)
	}

	return attachment, nil
}

func (m *Module) lookupRequestResidentAccount(accountCode, unitCode string) (*requestResidentLookup, error) {
	var account property.ResidentAccount
	if err := m.db.Where("account_code = ?", accountCode).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("resident account not found")
		}
		return nil, apperrors.Internal("find resident account", err)
	}

	var unit property.Unit
	if err := m.db.Where("id = ? AND code = ?", account.UnitID, unitCode).First(&unit).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("resident account not found")
		}
		return nil, apperrors.Internal("match unit for resident account", err)
	}

	var area property.Area
	if err := m.db.Where("id = ?", unit.AreaID).First(&area).Error; err != nil {
		return nil, apperrors.Internal("load area", err)
	}

	var building property.Building
	if err := m.db.Where("id = ?", area.BuildingID).First(&building).Error; err != nil {
		return nil, apperrors.Internal("load building", err)
	}

	return &requestResidentLookup{
		Account:  account,
		Unit:     unit,
		Building: building,
	}, nil
}

func mapDocumentRequest(request DocumentRequest) DocumentRequestItem {
	attachments := make([]DocumentRequestAttachmentView, 0, len(request.Attachments))
	for _, attachment := range request.Attachments {
		attachments = append(attachments, DocumentRequestAttachmentView{
			ID:         attachment.PublicID,
			Title:      attachment.Title,
			Meta:       attachment.Meta,
			Type:       attachment.Type,
			FileURL:    derefString(attachment.FileURL),
			UploadedBy: attachment.UploadedBy,
		})
	}

	timeline := make([]DocumentRequestTimelineItemView, 0, len(request.Updates))
	for index, update := range request.Updates {
		timeline = append(timeline, DocumentRequestTimelineItemView{
			ID:          update.PublicID,
			Title:       update.Title,
			Description: update.Comment,
			Timestamp:   update.CreatedAt.Format("02 Jan 2006, 03:04 PM"),
			IsCurrent:   index == 0,
		})
	}

	return DocumentRequestItem{
		ID:                   request.PublicID,
		Reference:            request.Reference,
		ResidentName:         request.ResidentName,
		ResidentCode:         request.ResidentCode,
		BuildingCode:         request.BuildingCode,
		BuildingName:         request.BuildingName,
		UnitCode:             request.UnitCode,
		RequestTypeID:        request.RequestTypeID,
		RequestTypeLabel:     request.RequestTypeLabel,
		PreferredFormatID:    request.PreferredFormatID,
		PreferredFormatLabel: request.PreferredFormatLabel,
		Purpose:              request.Purpose,
		Notes:                request.Notes,
		Status:               request.Status,
		LatestComment:        request.LatestComment,
		SubmittedAt:          request.CreatedAt.Format("02 Jan 2006, 03:04 PM"),
		UpdatedAt:            request.UpdatedAt.Format("02 Jan 2006, 03:04 PM"),
		Attachments:          attachments,
		Timeline:             timeline,
	}
}

func nextDocumentRequestReference(now time.Time) string {
	return fmt.Sprintf("DR-%s-%03d", now.Format("20060102"), now.UnixMilli()%1000)
}

func documentRequestTypes() []documentRequestOption {
	return []documentRequestOption{
		{ID: "account-statement", Label: "Account Statement"},
		{ID: "residency-letter", Label: "Residence Confirmation Letter"},
		{ID: "access-record", Label: "Access or Visitor Record"},
		{ID: "archived-circular", Label: "Archived Circular or Notice"},
	}
}

func documentRequestTypeByID(id string) (documentRequestOption, bool) {
	for _, option := range documentRequestTypes() {
		if option.ID == id {
			return option, true
		}
	}
	return documentRequestOption{}, false
}

func documentRequestFormats() []documentRequestOption {
	return []documentRequestOption{
		{ID: "digital-copy", Label: "Digital PDF"},
		{ID: "printed-copy", Label: "Printed Copy"},
	}
}

func documentRequestFormatByID(id string) (documentRequestOption, bool) {
	for _, option := range documentRequestFormats() {
		if option.ID == id {
			return option, true
		}
	}
	return documentRequestOption{}, false
}

func documentRequestAttachmentTypeFor(fileName, mimeType string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return "image"
	}
	return "document"
}

func documentRequestTimelineTitle(status string) string {
	switch status {
	case "in_review":
		return "In Review"
	case "fulfilled":
		return "Fulfilled"
	case "rejected":
		return "Rejected"
	default:
		return "Submitted"
	}
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
