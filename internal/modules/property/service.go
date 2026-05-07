package property

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"modular-api/internal/platform/apperrors"

	"gorm.io/gorm"
)

type Module struct {
	db *gorm.DB
}

func NewModule(db *gorm.DB) *Module {
	return &Module{db: db}
}

type TreeNode struct {
	Type              string     `json:"type"`
	Code              string     `json:"code"`
	Name              string     `json:"name"`
	Status            string     `json:"status"`
	DocumentsExpected []string   `json:"documentsExpected"`
	ResidentCode      string     `json:"residentCode,omitempty"`
	ResidentName      string     `json:"residentName,omitempty"`
	Children          []TreeNode `json:"children,omitempty"`
}

type UnitDirectoryItem struct {
	BuildingCode string `json:"buildingCode"`
	BuildingName string `json:"buildingName"`
	AreaCode     string `json:"areaCode"`
	AreaName     string `json:"areaName"`
	UnitCode     string `json:"unitCode"`
	UnitName     string `json:"unitName"`
	UnitStatus   string `json:"unitStatus"`
	AccountCode  string `json:"accountCode"`
	ResidentCode string `json:"residentCode"`
	ResidentName string `json:"residentName"`
	Email        string `json:"email"`
}

type ResidentAccountDirectoryItem struct {
	AccountCode  string `json:"accountCode"`
	ResidentCode string `json:"residentCode"`
	ResidentName string `json:"residentName"`
	Email        string `json:"email"`
	BuildingCode string `json:"buildingCode"`
	BuildingName string `json:"buildingName"`
	AreaCode     string `json:"areaCode"`
	AreaName     string `json:"areaName"`
	UnitCode     string `json:"unitCode"`
	UnitName     string `json:"unitName"`
	ResidentRole string `json:"residentRole"`
}

type OwnerTenantRegistrationItem struct {
	ID                string `json:"id"`
	OwnerAccountCode  string `json:"ownerAccountCode"`
	OwnerResidentCode string `json:"ownerResidentCode,omitempty"`
	OwnerName         string `json:"ownerName"`
	PropertyName      string `json:"propertyName"`
	UnitNumber        string `json:"unitNumber"`
	TenantName        string `json:"tenantName"`
	TenantEmail       string `json:"tenantEmail"`
	TenantPhone       string `json:"tenantPhone"`
	MoveInDate        string `json:"moveInDate"`
	Notes             string `json:"notes,omitempty"`
	Status            string `json:"status"`
	CreatedAt         string `json:"createdAt"`
}

type RegisterOwnerTenantInput struct {
	OwnerAccountCode  string
	OwnerResidentCode string
	OwnerName         string
	PropertyName      string
	UnitNumber        string
	TenantName        string
	TenantEmail       string
	TenantPhone       string
	MoveInDate        string
	Notes             string
}

type AccessProfile struct {
	AccountCode  string
	ResidentCode string
	ResidentName string
	Email        string
	ResidentRole string
	Unit         Unit
	Area         Area
	Building     Building
}

func (m *Module) Tree() ([]TreeNode, error) {
	var buildings []Building
	if err := m.db.Preload("Areas.Units.ResidentAccount").Order("code asc").Find(&buildings).Error; err != nil {
		return nil, apperrors.Internal("list property tree", err)
	}

	nodes := make([]TreeNode, 0, len(buildings))
	for _, building := range buildings {
		buildingNode := TreeNode{
			Type:              "building",
			Code:              building.Code,
			Name:              building.Name,
			Status:            building.Status,
			DocumentsExpected: splitDocs(building.DocumentsExpected),
			Children:          make([]TreeNode, 0, len(building.Areas)),
		}

		for _, area := range building.Areas {
			areaNode := TreeNode{
				Type:              "area",
				Code:              area.Code,
				Name:              area.Name,
				Status:            area.Status,
				DocumentsExpected: splitDocs(area.DocumentsExpected),
				Children:          make([]TreeNode, 0, len(area.Units)),
			}

			for _, unit := range area.Units {
				unitNode := TreeNode{
					Type:              "unit",
					Code:              unit.Code,
					Name:              unit.Name,
					Status:            unit.Status,
					DocumentsExpected: splitDocs(unit.DocumentsExpected),
					ResidentCode:      unit.ResidentAccount.ResidentCode,
					ResidentName:      unit.ResidentAccount.ResidentName,
				}
				areaNode.Children = append(areaNode.Children, unitNode)
			}

			buildingNode.Children = append(buildingNode.Children, areaNode)
		}

		nodes = append(nodes, buildingNode)
	}

	return nodes, nil
}

func (m *Module) Units() ([]UnitDirectoryItem, error) {
	var buildings []Building
	if err := m.db.Preload("Areas.Units.ResidentAccount").Order("code asc").Find(&buildings).Error; err != nil {
		return nil, apperrors.Internal("list property units", err)
	}

	items := make([]UnitDirectoryItem, 0)
	for _, building := range buildings {
		for _, area := range building.Areas {
			for _, unit := range area.Units {
				items = append(items, UnitDirectoryItem{
					BuildingCode: building.Code,
					BuildingName: building.Name,
					AreaCode:     area.Code,
					AreaName:     area.Name,
					UnitCode:     unit.Code,
					UnitName:     unit.Name,
					UnitStatus:   unit.Status,
					AccountCode:  unit.ResidentAccount.AccountCode,
					ResidentCode: unit.ResidentAccount.ResidentCode,
					ResidentName: unit.ResidentAccount.ResidentName,
					Email:        unit.ResidentAccount.Email,
				})
			}
		}
	}

	return items, nil
}

func (m *Module) ResidentAccountsByEmail(email string) ([]ResidentAccountDirectoryItem, error) {
	normalizedEmail := strings.TrimSpace(strings.ToLower(email))
	if normalizedEmail == "" {
		return nil, apperrors.Validation("email query parameter is required")
	}

	var buildings []Building
	if err := m.db.Preload("Areas.Units.ResidentAccount").Order("code asc").Find(&buildings).Error; err != nil {
		return nil, apperrors.Internal("list resident accounts", err)
	}

	items := make([]ResidentAccountDirectoryItem, 0)

	for _, building := range buildings {
		for _, area := range building.Areas {
			for _, unit := range area.Units {
				account := unit.ResidentAccount
				if strings.TrimSpace(strings.ToLower(account.Email)) != normalizedEmail {
					continue
				}

				items = append(items, ResidentAccountDirectoryItem{
					AccountCode:  account.AccountCode,
					ResidentCode: account.ResidentCode,
					ResidentName: account.ResidentName,
					Email:        account.Email,
					BuildingCode: building.Code,
					BuildingName: building.Name,
					AreaCode:     area.Code,
					AreaName:     area.Name,
					UnitCode:     unit.Code,
					UnitName:     unit.Name,
					ResidentRole: residentRoleForUnit(unit.Status),
				})
			}
		}
	}

	var ownerTenantRecords []OwnerTenantRegistration
	if err := m.db.Order("created_at desc").Find(&ownerTenantRecords).Error; err != nil {
		return nil, apperrors.Internal("list owner tenant registrations", err)
	}

	for _, record := range ownerTenantRecords {
		if strings.TrimSpace(strings.ToLower(record.TenantEmail)) != normalizedEmail {
			continue
		}

		accessProfile, err := ResolveAccessProfile(m.db, record.PublicID, record.UnitCode)
		if err != nil {
			return nil, err
		}

		items = append(items, ResidentAccountDirectoryItem{
			AccountCode:  accessProfile.AccountCode,
			ResidentCode: accessProfile.ResidentCode,
			ResidentName: accessProfile.ResidentName,
			Email:        accessProfile.Email,
			BuildingCode: accessProfile.Building.Code,
			BuildingName: accessProfile.Building.Name,
			AreaCode:     accessProfile.Area.Code,
			AreaName:     accessProfile.Area.Name,
			UnitCode:     accessProfile.Unit.Code,
			UnitName:     accessProfile.Unit.Name,
			ResidentRole: accessProfile.ResidentRole,
		})
	}

	return items, nil
}

func (m *Module) OwnerTenantRegistrations(ownerAccountCodes []string, unitCode string) ([]OwnerTenantRegistrationItem, error) {
	normalized := normalizeCodes(ownerAccountCodes)
	if len(normalized) == 0 {
		return nil, apperrors.Validation("ownerAccountCodes query parameter is required")
	}

	var records []OwnerTenantRegistration
	query := m.db.Where("owner_account_code IN ?", normalized)
	if normalizedUnitCode := strings.TrimSpace(unitCode); normalizedUnitCode != "" {
		query = query.Where("unit_code = ?", normalizedUnitCode)
	}
	if err := query.
		Order("updated_at desc").
		Order("created_at desc").
		Find(&records).Error; err != nil {
		return nil, apperrors.Internal("list owner tenant registrations", err)
	}

	items := make([]OwnerTenantRegistrationItem, 0, len(records))
	for _, record := range records {
		items = append(items, mapOwnerTenantRegistration(record))
	}

	return items, nil
}

func (m *Module) RegisterOwnerTenant(input RegisterOwnerTenantInput) (*OwnerTenantRegistrationItem, error) {
	ownerAccountCode := strings.TrimSpace(input.OwnerAccountCode)
	ownerResidentCode := strings.TrimSpace(input.OwnerResidentCode)
	ownerName := strings.TrimSpace(input.OwnerName)
	propertyName := strings.TrimSpace(input.PropertyName)
	unitCode := strings.TrimSpace(input.UnitNumber)
	tenantName := strings.TrimSpace(input.TenantName)
	tenantEmail := strings.ToLower(strings.TrimSpace(input.TenantEmail))
	tenantPhone := strings.TrimSpace(input.TenantPhone)
	moveInDate, err := normalizeMoveInDate(input.MoveInDate)
	notes := strings.TrimSpace(input.Notes)

	if ownerAccountCode == "" || unitCode == "" || tenantName == "" || tenantEmail == "" || moveInDate == "" {
		return nil, apperrors.Validation("ownerAccountCode, unitNumber, tenantName, tenantEmail, and moveInDate are required")
	}
	if err != nil {
		return nil, apperrors.Validation("moveInDate must be a valid date")
	}

	lookup, err := m.lookupOwnerResidentAccount(ownerAccountCode, unitCode)
	if err != nil {
		return nil, err
	}

	if ownerResidentCode == "" {
		ownerResidentCode = lookup.ResidentAccount.ResidentCode
	}
	if ownerName == "" {
		ownerName = lookup.ResidentAccount.ResidentName
	}
	if propertyName == "" {
		propertyName = lookup.Building.Name
	}

	now := time.Now()
	record := OwnerTenantRegistration{
		PublicID:          fmt.Sprintf("owner-tenant-%d", now.UnixMilli()),
		OwnerAccountCode:  lookup.ResidentAccount.AccountCode,
		OwnerResidentCode: ownerResidentCode,
		OwnerName:         ownerName,
		PropertyName:      propertyName,
		UnitCode:          lookup.Unit.Code,
		TenantName:        tenantName,
		TenantEmail:       tenantEmail,
		TenantPhone:       tenantPhone,
		MoveInDate:        moveInDate,
		Notes:             notes,
		Status:            "pending_activation",
	}

	if err := m.db.Create(&record).Error; err != nil {
		return nil, apperrors.Internal("create owner tenant registration", err)
	}

	item := mapOwnerTenantRegistration(record)
	return &item, nil
}

func splitDocs(value string) []string {
	if value == "" {
		return []string{}
	}
	return strings.Split(value, "|")
}

func residentRoleForUnit(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "tenanted":
		return "Tenant"
	case "owner occupied":
		return "Primary occupant"
	default:
		return "Resident"
	}
}

type ownerLookup struct {
	ResidentAccount ResidentAccount
	Unit            Unit
	Building        Building
}

func (m *Module) lookupOwnerResidentAccount(accountCode, unitCode string) (*ownerLookup, error) {
	var account ResidentAccount
	if err := m.db.Where("account_code = ?", accountCode).First(&account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("owner resident account not found")
		}
		return nil, apperrors.Internal("find owner resident account", err)
	}

	var unit Unit
	if err := m.db.Where("id = ? AND code = ?", account.UnitID, unitCode).First(&unit).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("owner unit not found")
		}
		return nil, apperrors.Internal("find owner unit", err)
	}

	if strings.ToLower(strings.TrimSpace(unit.Status)) != "owner occupied" {
		return nil, apperrors.Validation("tenant registration is only available for owner-occupied units")
	}

	var area Area
	if err := m.db.Where("id = ?", unit.AreaID).First(&area).Error; err != nil {
		return nil, apperrors.Internal("find owner area", err)
	}

	var building Building
	if err := m.db.Where("id = ?", area.BuildingID).First(&building).Error; err != nil {
		return nil, apperrors.Internal("find owner building", err)
	}

	return &ownerLookup{
		ResidentAccount: account,
		Unit:            unit,
		Building:        building,
	}, nil
}

func mapOwnerTenantRegistration(record OwnerTenantRegistration) OwnerTenantRegistrationItem {
	return OwnerTenantRegistrationItem{
		ID:                record.PublicID,
		OwnerAccountCode:  record.OwnerAccountCode,
		OwnerResidentCode: record.OwnerResidentCode,
		OwnerName:         record.OwnerName,
		PropertyName:      record.PropertyName,
		UnitNumber:        record.UnitCode,
		TenantName:        record.TenantName,
		TenantEmail:       record.TenantEmail,
		TenantPhone:       record.TenantPhone,
		MoveInDate:        record.MoveInDate,
		Notes:             record.Notes,
		Status:            record.Status,
		CreatedAt:         record.CreatedAt.Format(time.RFC3339),
	}
}

func ResolveAccessProfile(db *gorm.DB, accountCode, unitCode string) (*AccessProfile, error) {
	normalizedAccountCode := strings.TrimSpace(accountCode)
	normalizedUnitCode := strings.TrimSpace(unitCode)
	if normalizedAccountCode == "" || normalizedUnitCode == "" {
		return nil, apperrors.Validation("accountCode and unitCode are required")
	}

	var account ResidentAccount
	if err := db.Where("account_code = ?", normalizedAccountCode).First(&account).Error; err == nil {
		return resolveResidentAccountProfile(db, account, normalizedUnitCode)
	} else if err != gorm.ErrRecordNotFound {
		return nil, apperrors.Internal("find resident account", err)
	}

	var registration OwnerTenantRegistration
	if err := db.Where("public_id = ? AND unit_code = ?", normalizedAccountCode, normalizedUnitCode).First(&registration).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("resident account not found")
		}
		return nil, apperrors.Internal("find owner tenant registration", err)
	}

	unit, area, building, err := loadUnitHierarchy(db, normalizedUnitCode)
	if err != nil {
		return nil, err
	}

	return &AccessProfile{
		AccountCode:  registration.PublicID,
		ResidentCode: registration.PublicID,
		ResidentName: registration.TenantName,
		Email:        registration.TenantEmail,
		ResidentRole: "Tenant",
		Unit:         unit,
		Area:         area,
		Building:     building,
	}, nil
}

func normalizeMoveInDate(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}

	parsed, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return "", err
	}

	return parsed.Format("2006-01-02"), nil
}

func normalizeCodes(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		code := strings.TrimSpace(value)
		if code == "" {
			continue
		}
		if !slices.Contains(normalized, code) {
			normalized = append(normalized, code)
		}
	}
	return normalized
}

func resolveResidentAccountProfile(db *gorm.DB, account ResidentAccount, unitCode string) (*AccessProfile, error) {
	unit, area, building, err := loadUnitHierarchyByID(db, account.UnitID, unitCode)
	if err != nil {
		return nil, err
	}

	return &AccessProfile{
		AccountCode:  account.AccountCode,
		ResidentCode: account.ResidentCode,
		ResidentName: account.ResidentName,
		Email:        account.Email,
		ResidentRole: residentRoleForUnit(unit.Status),
		Unit:         unit,
		Area:         area,
		Building:     building,
	}, nil
}

func loadUnitHierarchy(db *gorm.DB, unitCode string) (Unit, Area, Building, error) {
	var unit Unit
	if err := db.Where("code = ?", unitCode).First(&unit).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return Unit{}, Area{}, Building{}, apperrors.NotFound("resident account not found")
		}
		return Unit{}, Area{}, Building{}, apperrors.Internal("find unit", err)
	}

	return loadUnitHierarchyByID(db, unit.ID, unitCode)
}

func loadUnitHierarchyByID(db *gorm.DB, unitID uint, unitCode string) (Unit, Area, Building, error) {
	var unit Unit
	if err := db.Where("id = ? AND code = ?", unitID, unitCode).First(&unit).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return Unit{}, Area{}, Building{}, apperrors.NotFound("resident account not found")
		}
		return Unit{}, Area{}, Building{}, apperrors.Internal("match unit for resident account", err)
	}

	var area Area
	if err := db.Where("id = ?", unit.AreaID).First(&area).Error; err != nil {
		return Unit{}, Area{}, Building{}, apperrors.Internal("load area", err)
	}

	var building Building
	if err := db.Where("id = ?", area.BuildingID).First(&building).Error; err != nil {
		return Unit{}, Area{}, Building{}, apperrors.Internal("load building", err)
	}

	return unit, area, building, nil
}
