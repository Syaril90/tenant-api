package property

import (
	"strings"

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

	return items, nil
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
