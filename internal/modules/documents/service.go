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

type Module struct {
	db      *gorm.DB
	storage storage.FileStorage
}

func NewModule(db *gorm.DB, fileStorage storage.FileStorage) *Module {
	return &Module{db: db, storage: fileStorage}
}

type Category struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Featured    bool   `json:"featured"`
}

type Item struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	SizeLabel      string `json:"sizeLabel"`
	Description    string `json:"description"`
	CategoryID     string `json:"categoryId"`
	CategoryLabel  string `json:"categoryLabel"`
	FileTypeLabel  string `json:"fileTypeLabel"`
	Tone           string `json:"tone"`
	UpdatedAtLabel string `json:"updatedAtLabel"`
	PreviewTitle   string `json:"previewTitle"`
	PreviewBody    string `json:"previewBody"`
	FileURL        string `json:"fileUrl"`
	BuildingCode   string `json:"buildingCode,omitempty"`
	BuildingLabel  string `json:"buildingLabel,omitempty"`
}

type ListPayload struct {
	Categories []Category `json:"categories"`
	Items      []Item     `json:"items"`
}

type CreateDocumentInput struct {
	Title        string
	Description  string
	CategoryID   string
	BuildingCode string
	File         multipart.File
	FileHeader   *multipart.FileHeader
}

type buildingDirectory struct {
	Code string
	Name string
}

func (m *Module) ListAdmin() (*ListPayload, error) {
	var records []Document
	if err := m.db.Where("status = ?", "published").Order("updated_at desc").Order("created_at desc").Find(&records).Error; err != nil {
		return nil, apperrors.Internal("list admin documents", err)
	}

	return &ListPayload{
		Categories: categories(),
		Items:      mapDocuments(records),
	}, nil
}

func (m *Module) ListResident(unitCode string) (*ListPayload, error) {
	normalizedUnitCode := strings.TrimSpace(unitCode)
	if normalizedUnitCode == "" {
		return nil, apperrors.Validation("unitCode query parameter is required")
	}

	building, err := m.lookupBuildingByUnitCode(normalizedUnitCode)
	if err != nil {
		return nil, err
	}

	var records []Document
	if err := m.db.
		Where("status = ?", "published").
		Where("(audience_scope = ? OR (audience_scope = ? AND building_code = ?))", "all_residents", "building", building.Code).
		Order("updated_at desc").
		Order("created_at desc").
		Find(&records).Error; err != nil {
		return nil, apperrors.Internal("list resident documents", err)
	}

	return &ListPayload{
		Categories: categories(),
		Items:      mapDocuments(records),
	}, nil
}

func (m *Module) Get(publicID string) (*Item, error) {
	normalizedID := strings.TrimSpace(publicID)
	if normalizedID == "" {
		return nil, apperrors.Validation("document id is required")
	}

	var record Document
	if err := m.db.Where("public_id = ? AND status = ?", normalizedID, "published").First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("document not found")
		}
		return nil, apperrors.Internal("load document", err)
	}

	item := mapDocument(record)
	return &item, nil
}

func (m *Module) Create(ctx context.Context, input CreateDocumentInput) (*Item, error) {
	title := strings.TrimSpace(input.Title)
	description := strings.TrimSpace(input.Description)
	categoryID := strings.TrimSpace(input.CategoryID)
	buildingCode := strings.TrimSpace(input.BuildingCode)

	if title == "" || description == "" || categoryID == "" {
		return nil, apperrors.Validation("title, description, and categoryId are required")
	}

	if input.File == nil || input.FileHeader == nil {
		return nil, apperrors.Validation("file is required")
	}

	category, ok := categoryByID(categoryID)
	if !ok {
		return nil, apperrors.Validationf("unknown categoryId: %s", categoryID)
	}

	audienceScope := "all_residents"
	buildingName := ""
	if buildingCode != "" {
		building, err := m.lookupBuildingByCode(buildingCode)
		if err != nil {
			return nil, err
		}
		audienceScope = "building"
		buildingCode = building.Code
		buildingName = building.Name
	}

	storedFile, err := m.storage.Save(ctx, storage.SaveFileInput{
		Folder:      "documents",
		FileName:    input.FileHeader.Filename,
		ContentType: input.FileHeader.Header.Get("Content-Type"),
		Reader:      input.File,
	})
	if err != nil {
		return nil, apperrors.Internal("store document file", err)
	}

	now := time.Now()
	record := Document{
		PublicID:        fmt.Sprintf("document-%d", now.UnixMilli()),
		Status:          "published",
		AudienceScope:   audienceScope,
		BuildingCode:    buildingCode,
		BuildingName:    buildingName,
		CategoryID:      category.ID,
		CategoryLabel:   category.Title,
		Title:           storedFile.OriginalName,
		Description:     description,
		PreviewTitle:    title,
		PreviewBody:     description,
		StorageProvider: storedFile.StorageProvider,
		ObjectKey:       storedFile.ObjectKey,
		OriginalName:    storedFile.OriginalName,
		MimeType:        storedFile.MimeType,
		SizeBytes:       storedFile.SizeBytes,
		PublicURL:       storedFile.PublicURL,
		FileTypeLabel:   fileTypeLabelForName(storedFile.OriginalName),
		Tone:            toneForFileType(fileTypeLabelForName(storedFile.OriginalName)),
	}

	if err := m.db.Create(&record).Error; err != nil {
		return nil, apperrors.Internal("create document", err)
	}

	item := mapDocument(record)
	return &item, nil
}

func (m *Module) lookupBuildingByCode(buildingCode string) (*buildingDirectory, error) {
	var building property.Building
	if err := m.db.Where("code = ?", buildingCode).First(&building).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("building not found")
		}
		return nil, apperrors.Internal("load building", err)
	}

	return &buildingDirectory{Code: building.Code, Name: building.Name}, nil
}

func (m *Module) lookupBuildingByUnitCode(unitCode string) (*buildingDirectory, error) {
	var unit property.Unit
	if err := m.db.
		Joins("JOIN areas ON areas.id = units.area_id").
		Joins("JOIN buildings ON buildings.id = areas.building_id").
		Select("units.id").
		Where("units.code = ?", unitCode).
		First(&unit).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("unit not found")
		}
		return nil, apperrors.Internal("load unit", err)
	}

	var result buildingDirectory
	if err := m.db.Table("units").
		Joins("JOIN areas ON areas.id = units.area_id").
		Joins("JOIN buildings ON buildings.id = areas.building_id").
		Select("buildings.code AS code, buildings.name AS name").
		Where("units.code = ?", unitCode).
		Scan(&result).Error; err != nil {
		return nil, apperrors.Internal("load unit building", err)
	}

	if result.Code == "" {
		return nil, apperrors.NotFound("unit building not found")
	}

	return &result, nil
}

func mapDocuments(records []Document) []Item {
	items := make([]Item, 0, len(records))
	for _, record := range records {
		items = append(items, mapDocument(record))
	}
	return items
}

func mapDocument(record Document) Item {
	return Item{
		ID:             record.PublicID,
		Title:          record.OriginalName,
		SizeLabel:      formatFileSize(record.SizeBytes),
		Description:    record.Description,
		CategoryID:     record.CategoryID,
		CategoryLabel:  record.CategoryLabel,
		FileTypeLabel:  record.FileTypeLabel,
		Tone:           record.Tone,
		UpdatedAtLabel: fmt.Sprintf("Updated %s", record.UpdatedAt.Format("02 Jan 2006")),
		PreviewTitle:   record.PreviewTitle,
		PreviewBody:    record.PreviewBody,
		FileURL:        record.PublicURL,
		BuildingCode:   record.BuildingCode,
		BuildingLabel:  record.BuildingName,
	}
}

func categories() []Category {
	return []Category{
		{ID: "house-rules", Title: "By-Laws", Description: "House Rules", Icon: "hammer-outline", Featured: true},
		{ID: "agm-minutes", Title: "AGM Minutes", Description: "Meeting Logs", Icon: "people-outline", Featured: false},
		{ID: "circulars", Title: "Circulars", Description: "Announcements", Icon: "megaphone-outline", Featured: false},
		{ID: "financials", Title: "Statements", Description: "Accounts & Finance", Icon: "wallet-outline", Featured: false},
	}
}

func categoryByID(categoryID string) (Category, bool) {
	for _, category := range categories() {
		if category.ID == categoryID {
			return category, true
		}
	}
	return Category{}, false
}

func fileTypeLabelForName(fileName string) string {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".doc", ".docx":
		return "DOCX"
	case ".xls", ".xlsx":
		return "XLSX"
	case ".jpg", ".jpeg":
		return "JPG"
	case ".png":
		return "PNG"
	default:
		return "PDF"
	}
}

func toneForFileType(fileTypeLabel string) string {
	switch fileTypeLabel {
	case "PDF":
		return "danger"
	case "XLSX":
		return "success"
	case "DOCX", "JPG", "PNG":
		return "info"
	default:
		return "neutral"
	}
}

func formatFileSize(sizeBytes int64) string {
	if sizeBytes < 1024*1024 {
		sizeKB := sizeBytes / 1024
		if sizeKB < 1 {
			sizeKB = 1
		}
		return fmt.Sprintf("%d KB", sizeKB)
	}

	return fmt.Sprintf("%.1f MB", float64(sizeBytes)/(1024*1024))
}
