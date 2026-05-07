package visitors

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"modular-api/internal/modules/property"
	"modular-api/internal/platform/apperrors"

	"gorm.io/gorm"
)

var allowedStatuses = map[string]bool{
	"pending":  true,
	"approved": true,
	"rejected": true,
}

type Module struct {
	db *gorm.DB
}

func NewModule(db *gorm.DB) *Module {
	return &Module{db: db}
}

type Item struct {
	ID                    string `json:"id"`
	AccountCode           string `json:"accountCode"`
	ResidentCode          string `json:"residentCode"`
	HostName              string `json:"hostName"`
	BuildingCode          string `json:"buildingCode"`
	BuildingName          string `json:"buildingName"`
	UnitCode              string `json:"unitCode"`
	VisitorName           string `json:"visitorName"`
	Purpose               string `json:"purpose"`
	VisitDate             string `json:"visitDate"`
	ArrivalWindow         string `json:"arrivalWindow"`
	VehiclePlate          string `json:"vehiclePlate"`
	ParkingSlotsRequested int    `json:"parkingSlotsRequested"`
	Status                string `json:"status"`
	SubmittedAt           string `json:"submittedAt"`
	UpdatedAt             string `json:"updatedAt"`
}

type BuildingConfig struct {
	BuildingCode string `json:"buildingCode"`
	BuildingName string `json:"buildingName"`
	TotalSlots   int    `json:"totalSlots"`
}

type AdminWorkspace struct {
	Approvals       []Item           `json:"approvals"`
	BuildingConfigs []BuildingConfig `json:"buildingConfigs"`
}

type CreateVisitorInput struct {
	AccountCode           string
	UnitCode              string
	VisitorName           string
	Purpose               string
	VisitDate             string
	ArrivalWindow         string
	VehiclePlate          string
	ParkingSlotsRequested int
}

type AdminApprovalInput struct {
	ID                    string `json:"id"`
	Status                string `json:"status"`
	ParkingSlotsRequested int    `json:"parkingSlotsRequested"`
}

type AdminBuildingConfigInput struct {
	BuildingCode string `json:"buildingCode"`
	TotalSlots   int    `json:"totalSlots"`
}

type residentLookup struct {
	AccountCode  string
	ResidentCode string
	ResidentName string
	Email        string
	Unit         property.Unit
	Building     property.Building
}

func (m *Module) List(unitCode string) ([]Item, error) {
	query := m.db.Order("visit_date desc").Order("updated_at desc").Order("created_at desc")
	if strings.TrimSpace(unitCode) != "" {
		query = query.Where("unit_code = ?", strings.TrimSpace(unitCode))
	}

	var requests []VisitorRequest
	if err := query.Find(&requests).Error; err != nil {
		return nil, apperrors.Internal("failed to list visitor requests", err)
	}

	items := make([]Item, 0, len(requests))
	for _, request := range requests {
		items = append(items, mapRequest(request))
	}

	return items, nil
}

func (m *Module) Create(input CreateVisitorInput) (*Item, error) {
	accountCode := strings.TrimSpace(input.AccountCode)
	unitCode := strings.TrimSpace(input.UnitCode)
	visitorName := strings.TrimSpace(input.VisitorName)
	purpose := strings.TrimSpace(input.Purpose)
	visitDate, err := normalizeVisitDate(input.VisitDate)
	arrivalWindow := strings.TrimSpace(input.ArrivalWindow)
	vehiclePlate := strings.TrimSpace(input.VehiclePlate)
	parkingSlotsRequested := input.ParkingSlotsRequested

	if accountCode == "" || unitCode == "" || visitorName == "" || purpose == "" || visitDate == "" {
		return nil, apperrors.Validation("accountCode, unitCode, visitorName, purpose, and visitDate are required")
	}
	if err != nil {
		return nil, apperrors.Validation("visitDate must be a valid date")
	}
	if arrivalWindow == "" {
		arrivalWindow = "All Day"
	}
	if vehiclePlate == "" {
		vehiclePlate = "No vehicle"
	}
	if parkingSlotsRequested <= 0 {
		parkingSlotsRequested = 1
	}

	resident, err := m.lookupResidentAccount(accountCode, unitCode)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("resident account not found")
		}
		return nil, err
	}

	now := time.Now()
	request := VisitorRequest{
		PublicID:              fmt.Sprintf("visitor-%d", now.UnixMilli()),
		AccountCode:           resident.AccountCode,
		ResidentCode:          resident.ResidentCode,
		ResidentName:          resident.ResidentName,
		BuildingCode:          resident.Building.Code,
		BuildingName:          resident.Building.Name,
		UnitCode:              resident.Unit.Code,
		VisitorName:           visitorName,
		Purpose:               purpose,
		VisitDate:             visitDate,
		ArrivalWindow:         arrivalWindow,
		VehiclePlate:          vehiclePlate,
		ParkingSlotsRequested: parkingSlotsRequested,
		Status:                "pending",
	}
	if err := m.db.Create(&request).Error; err != nil {
		return nil, apperrors.Internal("failed to create visitor request", err)
	}

	item := mapRequest(request)
	return &item, nil
}

func (m *Module) AdminVisitorRequests() ([]Item, error) {
	var requests []VisitorRequest
	if err := m.db.Order("visit_date desc").Order("updated_at desc").Order("created_at desc").Find(&requests).Error; err != nil {
		return nil, apperrors.Internal("failed to list admin visitor requests", err)
	}

	items := make([]Item, 0, len(requests))
	for _, request := range requests {
		items = append(items, mapRequest(request))
	}

	return items, nil
}

func (m *Module) AdminParkingConfigs() ([]BuildingConfig, error) {
	var buildings []property.Building
	if err := m.db.Order("code asc").Find(&buildings).Error; err != nil {
		return nil, apperrors.Internal("failed to list visitor parking configs", err)
	}

	items := make([]BuildingConfig, 0, len(buildings))
	for _, building := range buildings {
		items = append(items, BuildingConfig{
			BuildingCode: building.Code,
			BuildingName: building.Name,
			TotalSlots:   building.VisitorSlots,
		})
	}

	return items, nil
}

func (m *Module) AdminWorkspace() (*AdminWorkspace, error) {
	approvals, err := m.AdminVisitorRequests()
	if err != nil {
		return nil, err
	}
	buildingConfigs, err := m.AdminParkingConfigs()
	if err != nil {
		return nil, err
	}

	return &AdminWorkspace{
		Approvals:       approvals,
		BuildingConfigs: buildingConfigs,
	}, nil
}

func (m *Module) ReplaceAdminWorkspace(approvals []AdminApprovalInput, buildingConfigs []AdminBuildingConfigInput) (*AdminWorkspace, error) {
	returnValue := &AdminWorkspace{}

	err := m.db.Transaction(func(tx *gorm.DB) error {
		var requests []VisitorRequest
		if err := tx.Find(&requests).Error; err != nil {
			return err
		}
		requestByID := make(map[string]VisitorRequest, len(requests))
		for _, request := range requests {
			requestByID[request.PublicID] = request
		}

		var buildings []property.Building
		if err := tx.Order("code asc").Find(&buildings).Error; err != nil {
			return err
		}
		buildingByCode := make(map[string]property.Building, len(buildings))
		for _, building := range buildings {
			buildingByCode[building.Code] = building
		}

		for _, config := range buildingConfigs {
			building, ok := buildingByCode[strings.TrimSpace(config.BuildingCode)]
			if !ok {
				return apperrors.NotFoundf("building not found for code %s", config.BuildingCode)
			}
			if config.TotalSlots < 0 {
				return apperrors.Validation("totalSlots must be zero or greater")
			}
			building.VisitorSlots = config.TotalSlots
			if err := tx.Save(&building).Error; err != nil {
				return apperrors.Internal("failed to save parking config", err)
			}
			buildingByCode[building.Code] = building
		}

		approvedSlotsByBuildingDate := make(map[string]int)
		for _, request := range requests {
			approvedSlotsByBuildingDate[quotaKey(request.BuildingCode, request.VisitDate)] = approvedSlotsByBuildingDate[quotaKey(request.BuildingCode, request.VisitDate)]
			if request.Status == "approved" {
				approvedSlotsByBuildingDate[quotaKey(request.BuildingCode, request.VisitDate)] += request.ParkingSlotsRequested
			}
		}

		sortedApprovals := make([]AdminApprovalInput, len(approvals))
		copy(sortedApprovals, approvals)
		sort.Slice(sortedApprovals, func(i, j int) bool {
			currentI, okI := requestByID[sortedApprovals[i].ID]
			currentJ, okJ := requestByID[sortedApprovals[j].ID]
			if !okI || !okJ {
				return sortedApprovals[i].ID < sortedApprovals[j].ID
			}
			return currentI.CreatedAt.Before(currentJ.CreatedAt)
		})

		for _, approval := range sortedApprovals {
			request, ok := requestByID[strings.TrimSpace(approval.ID)]
			if !ok {
				return apperrors.NotFoundf("visitor request not found for id %s", approval.ID)
			}

			nextStatus := strings.TrimSpace(strings.ToLower(approval.Status))
			if !allowedStatuses[nextStatus] {
				return apperrors.Validation("status must be one of: pending, approved, rejected")
			}

			nextParkingSlots := approval.ParkingSlotsRequested
			if nextParkingSlots <= 0 {
				nextParkingSlots = request.ParkingSlotsRequested
			}

			key := quotaKey(request.BuildingCode, request.VisitDate)
			if request.Status == "approved" {
				approvedSlotsByBuildingDate[key] -= request.ParkingSlotsRequested
			}
			if nextStatus == "approved" {
				building, ok := buildingByCode[request.BuildingCode]
				if !ok {
					return apperrors.NotFoundf("building not found for code %s", request.BuildingCode)
				}
				if approvedSlotsByBuildingDate[key]+nextParkingSlots > building.VisitorSlots {
					return apperrors.Conflictf("approved visitor parking exceeds quota for %s on %s", request.BuildingCode, request.VisitDate)
				}
				approvedSlotsByBuildingDate[key] += nextParkingSlots
			}

			request.Status = nextStatus
			request.ParkingSlotsRequested = nextParkingSlots
			if err := tx.Save(&request).Error; err != nil {
				return apperrors.Internal("failed to save visitor request", err)
			}
			requestByID[request.PublicID] = request
		}

		workspace, err := m.workspaceFromTransaction(tx)
		if err != nil {
			return err
		}
		*returnValue = *workspace
		return nil
	})
	if err != nil {
		return nil, err
	}

	return returnValue, nil
}

func (m *Module) ReplaceAdminVisitorRequests(input []AdminApprovalInput) ([]Item, error) {
	workspace, err := m.ReplaceAdminWorkspace(input, nil)
	if err != nil {
		return nil, err
	}

	return workspace.Approvals, nil
}

func (m *Module) ReplaceAdminParkingConfigs(input []AdminBuildingConfigInput) ([]BuildingConfig, error) {
	workspace, err := m.ReplaceAdminWorkspace(nil, input)
	if err != nil {
		return nil, err
	}

	return workspace.BuildingConfigs, nil
}

func (m *Module) workspaceFromTransaction(tx *gorm.DB) (*AdminWorkspace, error) {
	var requests []VisitorRequest
	if err := tx.Order("visit_date desc").Order("updated_at desc").Order("created_at desc").Find(&requests).Error; err != nil {
		return nil, err
	}

	var buildings []property.Building
	if err := tx.Order("code asc").Find(&buildings).Error; err != nil {
		return nil, err
	}

	approvals := make([]Item, 0, len(requests))
	for _, request := range requests {
		approvals = append(approvals, mapRequest(request))
	}

	buildingConfigs := make([]BuildingConfig, 0, len(buildings))
	for _, building := range buildings {
		buildingConfigs = append(buildingConfigs, BuildingConfig{
			BuildingCode: building.Code,
			BuildingName: building.Name,
			TotalSlots:   building.VisitorSlots,
		})
	}

	return &AdminWorkspace{
		Approvals:       approvals,
		BuildingConfigs: buildingConfigs,
	}, nil
}

func (m *Module) lookupResidentAccount(accountCode, unitCode string) (*residentLookup, error) {
	accessProfile, err := property.ResolveAccessProfile(m.db, accountCode, unitCode)
	if err != nil {
		return nil, err
	}

	return &residentLookup{
		AccountCode:  accessProfile.AccountCode,
		ResidentCode: accessProfile.ResidentCode,
		ResidentName: accessProfile.ResidentName,
		Email:        accessProfile.Email,
		Unit:         accessProfile.Unit,
		Building:     accessProfile.Building,
	}, nil
}

func mapRequest(request VisitorRequest) Item {
	visitDate, err := normalizeVisitDate(request.VisitDate)
	if err != nil {
		visitDate = strings.TrimSpace(request.VisitDate)
	}

	return Item{
		ID:                    request.PublicID,
		AccountCode:           request.AccountCode,
		ResidentCode:          request.ResidentCode,
		HostName:              request.ResidentName,
		BuildingCode:          request.BuildingCode,
		BuildingName:          request.BuildingName,
		UnitCode:              request.UnitCode,
		VisitorName:           request.VisitorName,
		Purpose:               request.Purpose,
		VisitDate:             visitDate,
		ArrivalWindow:         request.ArrivalWindow,
		VehiclePlate:          request.VehiclePlate,
		ParkingSlotsRequested: request.ParkingSlotsRequested,
		Status:                request.Status,
		SubmittedAt:           request.CreatedAt.Format("02 Jan 2006 • 03:04 PM"),
		UpdatedAt:             request.UpdatedAt.Format("02 Jan 2006 • 03:04 PM"),
	}
}

func quotaKey(buildingCode, visitDate string) string {
	return buildingCode + "::" + visitDate
}

func normalizeVisitDate(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}

	layouts := []string{
		"2006-01-02",
		"Mon 2 Jan 2006",
		"Mon 02 Jan 2006",
		"2 Jan 2006",
		"02 Jan 2006",
	}

	for _, layout := range layouts {
		parsed, err := time.Parse(layout, trimmed)
		if err == nil {
			return parsed.Format("2006-01-02"), nil
		}
	}

	return "", fmt.Errorf("visitDate must be a valid date")
}
