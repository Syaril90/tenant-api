package billing

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"modular-api/internal/modules/property"
	"modular-api/internal/platform/apperrors"

	"gorm.io/gorm"
)

type Module struct {
	db      *gorm.DB
	gateway GatewayConfig
}

type GatewayConfig struct {
	BillplzAPIBaseURL      string
	BillplzAPIKey          string
	BillplzXSignatureKey   string
	BillplzCollectionID    string
	BillplzCallbackBaseURL string
}

func NewModule(db *gorm.DB, gateway GatewayConfig) *Module {
	return &Module{db: db, gateway: gateway}
}

type ResidentBillingView struct {
	AccountCode    string        `json:"accountCode"`
	UnitCode       string        `json:"unitCode"`
	BuildingName   string        `json:"buildingName"`
	ResidentCode   string        `json:"residentCode"`
	ResidentName   string        `json:"residentName"`
	Outstanding    float64       `json:"outstanding"`
	Charges        []ChargeView  `json:"charges"`
	RecentPayments []PaymentView `json:"recentPayments"`
}

type ChargeView struct {
	ID          uint    `json:"id"`
	Reference   string  `json:"reference"`
	BillingType string  `json:"billingType"`
	Category    string  `json:"category"`
	PeriodLabel string  `json:"periodLabel"`
	Amount      float64 `json:"amount"`
	DueDate     string  `json:"dueDate"`
	Status      string  `json:"status"`
	Description string  `json:"description"`
}

type PaymentView struct {
	ID          uint    `json:"id"`
	Reference   string  `json:"reference"`
	Amount      float64 `json:"amount"`
	PaidAt      string  `json:"paidAt"`
	MethodLabel string  `json:"methodLabel"`
	Status      string  `json:"status"`
	Description string  `json:"description"`
}

type AdminBillingNode struct {
	BuildingCode string             `json:"buildingCode"`
	BuildingName string             `json:"buildingName"`
	Areas        []AdminBillingArea `json:"areas"`
}

type AdminBillingArea struct {
	AreaCode string             `json:"areaCode"`
	AreaName string             `json:"areaName"`
	Units    []AdminBillingUnit `json:"units"`
}

type AdminBillingUnit struct {
	UnitCode     string   `json:"unitCode"`
	UnitName     string   `json:"unitName"`
	AccountCode  string   `json:"accountCode"`
	ResidentCode string   `json:"residentCode"`
	ResidentName string   `json:"residentName"`
	Outstanding  float64  `json:"outstanding"`
	Status       string   `json:"status"`
	BillingTypes []string `json:"billingTypes"`
}

type CreateChargeInput struct {
	UnitCodes   []string `json:"unitCodes"`
	BillingType string   `json:"billingType"`
	Category    string   `json:"category"`
	PeriodLabel string   `json:"periodLabel"`
	Icon        string   `json:"icon"`
	Amount      float64  `json:"amount"`
	DueDate     string   `json:"dueDate"`
	Reference   string   `json:"reference"`
	Description string   `json:"description"`
	Source      string   `json:"source"`
}

type RecordPaymentInput struct {
	UnitCode    string   `json:"unitCode"`
	ChargeRefs  []string `json:"chargeReferences"`
	Amount      float64  `json:"amount"`
	Reference   string   `json:"reference"`
	Description string   `json:"description"`
	Source      string   `json:"source"`
	MethodID    string   `json:"methodId"`
	MethodLabel string   `json:"methodLabel"`
	Status      string   `json:"status"`
}

func (m *Module) ResidentBilling(unitCode string) (*ResidentBillingView, error) {
	if err := m.syncPendingGatewayTransactionsForUnit(unitCode); err != nil {
		return nil, err
	}

	var unit property.Unit
	if err := m.db.
		Joins("ResidentAccount").
		Joins("JOIN areas ON areas.id = units.area_id").
		Joins("JOIN buildings ON buildings.id = areas.building_id").
		Where("units.code = ?", unitCode).
		Preload("ResidentAccount").
		First(&unit).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFoundf("billing unit %s not found", unitCode)
		}
		return nil, apperrors.Internal("load resident billing unit", err)
	}

	var area property.Area
	if err := m.db.First(&area, unit.AreaID).Error; err != nil {
		return nil, apperrors.Internal("load billing unit area", err)
	}

	var building property.Building
	if err := m.db.First(&building, area.BuildingID).Error; err != nil {
		return nil, apperrors.Internal("load billing unit building", err)
	}

	var charges []Charge
	if err := m.db.Where("unit_code = ?", unitCode).Order("due_date asc").Find(&charges).Error; err != nil {
		return nil, apperrors.Internal("list resident charges", err)
	}

	var payments []Payment
	if err := m.db.Where("unit_code = ?", unitCode).Order("paid_at desc").Find(&payments).Error; err != nil {
		return nil, apperrors.Internal("list resident payments", err)
	}

	paidByCharge, err := m.paidAmountByCharge(charges)
	if err != nil {
		return nil, err
	}

	outstanding := 0.0
	chargeViews := make([]ChargeView, 0, len(charges))
	for _, charge := range charges {
		paidAmount := paidByCharge[charge.ID]
		status := "unpaid"
		if paidAmount >= charge.Amount {
			status = "paid"
		} else if paidAmount > 0 {
			status = "partial"
		} else if dueDatePassed(charge.DueDate) {
			status = "overdue"
		}

		outstanding += maxFloat(charge.Amount-paidAmount, 0)
		chargeViews = append(chargeViews, ChargeView{
			ID:          charge.ID,
			Reference:   charge.Reference,
			BillingType: charge.BillingType,
			Category:    charge.Category,
			PeriodLabel: charge.PeriodLabel,
			Amount:      charge.Amount,
			DueDate:     charge.DueDate,
			Status:      status,
			Description: charge.Description,
		})
	}

	recentPayments := make([]PaymentView, 0, len(payments))
	for _, payment := range payments {
		recentPayments = append(recentPayments, PaymentView{
			ID:          payment.ID,
			Reference:   payment.Reference,
			Amount:      payment.Amount,
			PaidAt:      payment.PaidAt,
			MethodLabel: payment.MethodLabel,
			Status:      payment.Status,
			Description: payment.Description,
		})
	}

	return &ResidentBillingView{
		AccountCode:    unit.ResidentAccount.AccountCode,
		UnitCode:       unit.Code,
		BuildingName:   building.Name,
		ResidentCode:   unit.ResidentAccount.ResidentCode,
		ResidentName:   unit.ResidentAccount.ResidentName,
		Outstanding:    outstanding,
		Charges:        chargeViews,
		RecentPayments: recentPayments,
	}, nil
}

func (m *Module) AdminTree() ([]AdminBillingNode, error) {
	var buildings []property.Building
	if err := m.db.Preload("Areas.Units.ResidentAccount").Order("code asc").Find(&buildings).Error; err != nil {
		return nil, apperrors.Internal("list admin billing tree", err)
	}

	unitCodes := make([]string, 0)
	for _, building := range buildings {
		for _, area := range building.Areas {
			for _, unit := range area.Units {
				unitCodes = append(unitCodes, unit.Code)
			}
		}
	}

	var charges []Charge
	if len(unitCodes) > 0 {
		if err := m.db.Where("unit_code IN ?", unitCodes).Find(&charges).Error; err != nil {
			return nil, apperrors.Internal("list billing charges", err)
		}
	}

	paidByCharge, err := m.paidAmountByCharge(charges)
	if err != nil {
		return nil, err
	}

	chargesByUnit := make(map[string][]Charge)
	for _, charge := range charges {
		chargesByUnit[charge.UnitCode] = append(chargesByUnit[charge.UnitCode], charge)
	}

	nodes := make([]AdminBillingNode, 0, len(buildings))
	for _, building := range buildings {
		buildingNode := AdminBillingNode{
			BuildingCode: building.Code,
			BuildingName: building.Name,
			Areas:        make([]AdminBillingArea, 0, len(building.Areas)),
		}

		for _, area := range building.Areas {
			areaNode := AdminBillingArea{
				AreaCode: area.Code,
				AreaName: area.Name,
				Units:    make([]AdminBillingUnit, 0, len(area.Units)),
			}

			for _, unit := range area.Units {
				unitCharges := chargesByUnit[unit.Code]
				outstanding := 0.0
				typeSet := make(map[string]struct{})

				for _, charge := range unitCharges {
					outstanding += maxFloat(charge.Amount-paidByCharge[charge.ID], 0)
					typeSet[charge.BillingType] = struct{}{}
				}

				billingTypes := make([]string, 0, len(typeSet))
				for billingType := range typeSet {
					billingTypes = append(billingTypes, billingType)
				}

				status := "no_charges"
				if len(unitCharges) > 0 {
					status = "unpaid"
					if outstanding == 0 {
						status = "paid"
					} else if outstanding < sumCharges(unitCharges) {
						status = "partial"
					}
				}

				areaNode.Units = append(areaNode.Units, AdminBillingUnit{
					UnitCode:     unit.Code,
					UnitName:     unit.Name,
					AccountCode:  unit.ResidentAccount.AccountCode,
					ResidentCode: unit.ResidentAccount.ResidentCode,
					ResidentName: unit.ResidentAccount.ResidentName,
					Outstanding:  outstanding,
					Status:       status,
					BillingTypes: billingTypes,
				})
			}

			buildingNode.Areas = append(buildingNode.Areas, areaNode)
		}

		nodes = append(nodes, buildingNode)
	}

	return nodes, nil
}

func (m *Module) CreateCharge(input CreateChargeInput) error {
	if len(input.UnitCodes) == 0 {
		return apperrors.Validation("unitCodes is required")
	}
	if input.Amount <= 0 {
		return apperrors.Validation("amount must be positive")
	}
	if input.Reference == "" {
		return apperrors.Validation("reference is required")
	}

	return m.db.Transaction(func(tx *gorm.DB) error {
		for index, unitCode := range input.UnitCodes {
			var unit property.Unit
			if err := tx.Preload("ResidentAccount").Where("code = ?", unitCode).First(&unit).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return apperrors.NotFoundf("billing unit %s not found", unitCode)
				}
				return apperrors.Internal("load charge billing unit", err)
			}

			reference := input.Reference
			if len(input.UnitCodes) > 1 {
				reference = fmt.Sprintf("%s-%s", input.Reference, sanitizeRefSuffix(unitCode))
			}

			charge := Charge{
				UnitCode:    unitCode,
				AccountCode: unit.ResidentAccount.AccountCode,
				Category:    defaultString(input.Category, "Management"),
				BillingType: input.BillingType,
				PeriodLabel: defaultString(input.PeriodLabel, monthPeriodLabel()),
				Icon:        defaultString(input.Icon, "wallet-outline"),
				Amount:      input.Amount,
				DueDate:     input.DueDate,
				PostedAt:    nowLabel(),
				Reference:   reference,
				Description: input.Description,
				Source:      defaultString(input.Source, "building_admin"),
			}

			if err := tx.Create(&charge).Error; err != nil {
				return apperrors.Internal(fmt.Sprintf("create charge %d", index), err)
			}
		}

		return nil
	})
}

func (m *Module) RecordPayment(input RecordPaymentInput) error {
	if input.UnitCode == "" {
		return apperrors.Validation("unitCode is required")
	}
	if input.Amount <= 0 {
		return apperrors.Validation("amount must be positive")
	}
	if input.Reference == "" {
		return apperrors.Validation("reference is required")
	}

	return m.db.Transaction(func(tx *gorm.DB) error {
		var unit property.Unit
		if err := tx.Preload("ResidentAccount").Where("code = ?", input.UnitCode).First(&unit).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.NotFoundf("billing unit %s not found", input.UnitCode)
			}
			return apperrors.Internal("load payment billing unit", err)
		}

		payment := Payment{
			AccountCode: unit.ResidentAccount.AccountCode,
			UnitCode:    input.UnitCode,
			Amount:      input.Amount,
			PaidAt:      nowLabel(),
			Reference:   input.Reference,
			Description: input.Description,
			Source:      defaultString(input.Source, "building_admin"),
			MethodID:    defaultString(input.MethodID, "offline_bank_transfer"),
			MethodLabel: defaultString(input.MethodLabel, "Bank Transfer"),
			Status:      defaultString(input.Status, "successful"),
		}

		if err := tx.Create(&payment).Error; err != nil {
			return apperrors.Internal("create payment", err)
		}

		if len(input.ChargeRefs) == 0 {
			return nil
		}

		var charges []Charge
		if err := tx.Where("reference IN ? AND unit_code = ?", input.ChargeRefs, input.UnitCode).Order("due_date asc, reference asc").Find(&charges).Error; err != nil {
			return apperrors.Internal("list payment charges", err)
		}
		if len(charges) != len(input.ChargeRefs) {
			return apperrors.NotFound("one or more charges were not found for the selected unit")
		}

		paidByCharge, err := m.paidAmountByChargeWithDB(tx, charges)
		if err != nil {
			return err
		}

		for _, allocation := range buildPaymentAllocations(payment.ID, input.Amount, charges, paidByCharge) {
			if allocation.Amount <= 0 {
				continue
			}

			if err := tx.Create(&allocation).Error; err != nil {
				return apperrors.Internal("create payment allocation", err)
			}
		}

		return nil
	})
}

func (m *Module) paidAmountByCharge(charges []Charge) (map[uint]float64, error) {
	return m.paidAmountByChargeWithDB(m.db, charges)
}

func (m *Module) paidAmountByChargeWithDB(db *gorm.DB, charges []Charge) (map[uint]float64, error) {
	chargeIDs := make([]uint, 0, len(charges))
	for _, charge := range charges {
		chargeIDs = append(chargeIDs, charge.ID)
	}

	paidByCharge := make(map[uint]float64)
	if len(chargeIDs) == 0 {
		return paidByCharge, nil
	}

	var allocations []PaymentAllocation
	if err := db.Where("charge_id IN ?", chargeIDs).Find(&allocations).Error; err != nil {
		return nil, apperrors.Internal("list payment allocations", err)
	}

	paymentIDs := make([]uint, 0, len(allocations))
	for _, allocation := range allocations {
		paymentIDs = append(paymentIDs, allocation.PaymentID)
	}

	var payments []Payment
	if len(paymentIDs) > 0 {
		if err := db.Where("id IN ? AND status = ?", paymentIDs, "successful").Find(&payments).Error; err != nil {
			return nil, apperrors.Internal("list payments for allocations", err)
		}
	}

	paymentByID := make(map[uint]Payment)
	for _, payment := range payments {
		paymentByID[payment.ID] = payment
	}

	for _, allocation := range allocations {
		payment, ok := paymentByID[allocation.PaymentID]
		if !ok {
			continue
		}

		if allocation.Amount > 0 {
			paidByCharge[allocation.ChargeID] += allocation.Amount
			continue
		}

		paidByCharge[allocation.ChargeID] += payment.Amount / maxFloat(float64(linkCountForPayment(allocation.PaymentID, allocations)), 1)
	}

	return paidByCharge, nil
}

func (m *Module) syncPendingGatewayTransactionsForUnit(unitCode string) error {
	if strings.TrimSpace(m.gateway.BillplzAPIKey) == "" || strings.TrimSpace(m.gateway.BillplzAPIBaseURL) == "" {
		return nil
	}

	var transactions []GatewayTransaction
	if err := m.db.Where("gateway = ? AND unit_code = ? AND status = ?", billplzGatewayName, unitCode, "pending").Order("created_at desc").Find(&transactions).Error; err != nil {
		return apperrors.Internal("list pending gateway transactions", err)
	}

	for _, transaction := range transactions {
		bill, err := m.getBillplzBill(transaction.ExternalID)
		if err != nil {
			return err
		}

		if !bill.Paid && strings.TrimSpace(strings.ToLower(bill.State)) == "due" {
			continue
		}

		payload, err := jsonMarshalGatewayBill(transaction.ExternalID, bill)
		if err != nil {
			return err
		}

		result := billplzCallbackResult{
			BillID:     bill.ID,
			Paid:       bill.Paid,
			State:      bill.State,
			PaidAt:     bill.PaidAt,
			RawPayload: payload,
		}

		if err := m.db.Transaction(func(tx *gorm.DB) error {
			current := transaction
			return m.reconcileGatewayTransaction(tx, &current, result, payload)
		}); err != nil {
			return err
		}
	}

	return nil
}

func buildPaymentAllocations(paymentID uint, paymentAmount float64, charges []Charge, paidByCharge map[uint]float64) []PaymentAllocation {
	orderedCharges := append([]Charge(nil), charges...)
	sort.Slice(orderedCharges, func(left, right int) bool {
		if orderedCharges[left].DueDate == orderedCharges[right].DueDate {
			return orderedCharges[left].Reference < orderedCharges[right].Reference
		}
		return orderedCharges[left].DueDate < orderedCharges[right].DueDate
	})

	remaining := paymentAmount
	allocations := make([]PaymentAllocation, 0, len(orderedCharges))

	for _, charge := range orderedCharges {
		if remaining <= 0 {
			break
		}

		balance := maxFloat(charge.Amount-paidByCharge[charge.ID], 0)
		if balance <= 0 {
			continue
		}

		allocated := minFloat(balance, remaining)
		allocations = append(allocations, PaymentAllocation{
			PaymentID: paymentID,
			ChargeID:  charge.ID,
			Amount:    allocated,
		})
		remaining -= allocated
	}

	return allocations
}

func jsonMarshalGatewayBill(externalID string, bill *billplzGetBillResponse) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"id":          externalID,
		"paid":        bill.Paid,
		"state":       bill.State,
		"paid_at":     bill.PaidAt,
		"paid_amount": bill.PaidAmount,
	})
	if err != nil {
		return "", apperrors.Internal("marshal gateway bill payload", err)
	}
	return string(payload), nil
}

func linkCountForPayment(paymentID uint, allocations []PaymentAllocation) float64 {
	count := 0.0
	for _, allocation := range allocations {
		if allocation.PaymentID == paymentID {
			count++
		}
	}
	return count
}

func sanitizeRefSuffix(unitCode string) string {
	return strings.ReplaceAll(unitCode, "-", "")
}

func dueDatePassed(dueDate string) bool {
	value, err := time.Parse("2006-01-02", dueDate)
	if err != nil {
		return false
	}
	return value.Before(time.Now())
}

func nowLabel() string {
	return time.Now().Format("02 Jan 2006 • 03:04 PM")
}

func monthPeriodLabel() string {
	return time.Now().Format("January 2006")
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func sumCharges(charges []Charge) float64 {
	total := 0.0
	for _, charge := range charges {
		total += charge.Amount
	}
	return total
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}
