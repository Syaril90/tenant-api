package seed

import (
	"fmt"
	"strings"
	"time"

	"modular-api/internal/modules/announcements"
	"modular-api/internal/modules/billing"
	"modular-api/internal/modules/complaints"
	"modular-api/internal/modules/feedback"
	"modular-api/internal/modules/property"
	"modular-api/internal/modules/visitors"

	"gorm.io/gorm"
)

type seedBuilding struct {
	Code              string
	Name              string
	Status            string
	DocumentsExpected string
	VisitorSlots      int
	Areas             []seedArea
}

type seedArea struct {
	Code              string
	Name              string
	Status            string
	DocumentsExpected string
	Units             []seedUnit
}

type seedUnit struct {
	Code              string
	Name              string
	Status            string
	DocumentsExpected string
	AccountCode       string
	ResidentCode      string
	ResidentName      string
	Email             string
}

type seedCharge struct {
	UnitCode    string
	BillingType string
	Category    string
	PeriodLabel string
	Icon        string
	Amount      float64
	DueDate     string
	PostedAt    string
	Reference   string
	Description string
	Source      string
}

type seedPayment struct {
	UnitCode    string
	ChargeRefs  []string
	Amount      float64
	PaidAt      string
	Reference   string
	Description string
	Source      string
	MethodID    string
	MethodLabel string
	Status      string
}

func Run(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := seedProperty(tx); err != nil {
			return err
		}
		if err := seedBilling(tx); err != nil {
			return err
		}
		if err := seedVisitors(tx); err != nil {
			return err
		}
		if err := seedAnnouncements(tx); err != nil {
			return err
		}
		if err := seedComplaints(tx); err != nil {
			return err
		}
		if err := seedFeedback(tx); err != nil {
			return err
		}
		return nil
	})
}

func seedProperty(db *gorm.DB) error {
	for _, buildingSeed := range propertySeeds() {
		buildingModel := property.Building{
			Code:              buildingSeed.Code,
			Name:              buildingSeed.Name,
			Status:            buildingSeed.Status,
			DocumentsExpected: buildingSeed.DocumentsExpected,
			VisitorSlots:      buildingSeed.VisitorSlots,
		}
		if err := db.Where(property.Building{Code: buildingSeed.Code}).FirstOrCreate(&buildingModel).Error; err != nil {
			return err
		}

		for _, areaSeed := range buildingSeed.Areas {
			areaModel := property.Area{
				BuildingID:        buildingModel.ID,
				Code:              areaSeed.Code,
				Name:              areaSeed.Name,
				Status:            areaSeed.Status,
				DocumentsExpected: areaSeed.DocumentsExpected,
			}
			if err := db.Where(property.Area{Code: areaSeed.Code}).FirstOrCreate(&areaModel).Error; err != nil {
				return err
			}

			for _, unitSeed := range areaSeed.Units {
				unitModel := property.Unit{
					AreaID:            areaModel.ID,
					Code:              unitSeed.Code,
					Name:              unitSeed.Name,
					Status:            unitSeed.Status,
					DocumentsExpected: unitSeed.DocumentsExpected,
				}
				if err := db.Where(property.Unit{Code: unitSeed.Code}).FirstOrCreate(&unitModel).Error; err != nil {
					return err
				}

				account := property.ResidentAccount{
					UnitID:       unitModel.ID,
					AccountCode:  unitSeed.AccountCode,
					ResidentCode: unitSeed.ResidentCode,
					ResidentName: unitSeed.ResidentName,
					Email:        unitSeed.Email,
				}
				if err := db.Where(property.ResidentAccount{AccountCode: unitSeed.AccountCode}).Assign(account).FirstOrCreate(&account).Error; err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func seedBilling(db *gorm.DB) error {
	accountByUnit := make(map[string]property.ResidentAccount)
	var accounts []property.ResidentAccount
	if err := db.Find(&accounts).Error; err != nil {
		return err
	}
	for _, account := range accounts {
		accountByUnit[account.ResidentCode] = account
	}

	unitToAccount := make(map[string]property.ResidentAccount)
	var units []property.Unit
	if err := db.Preload("ResidentAccount").Find(&units).Error; err != nil {
		return err
	}
	for _, unit := range units {
		unitToAccount[unit.Code] = unit.ResidentAccount
	}

	for _, chargeSeed := range billingChargeSeeds() {
		account, ok := unitToAccount[chargeSeed.UnitCode]
		if !ok {
			return fmt.Errorf("missing account for unit %s", chargeSeed.UnitCode)
		}

		model := billing.Charge{
			UnitCode:    chargeSeed.UnitCode,
			AccountCode: account.AccountCode,
			Category:    chargeSeed.Category,
			BillingType: chargeSeed.BillingType,
			PeriodLabel: chargeSeed.PeriodLabel,
			Icon:        chargeSeed.Icon,
			Amount:      chargeSeed.Amount,
			DueDate:     chargeSeed.DueDate,
			PostedAt:    chargeSeed.PostedAt,
			Reference:   chargeSeed.Reference,
			Description: chargeSeed.Description,
			Source:      chargeSeed.Source,
		}
		if err := db.Where(billing.Charge{Reference: chargeSeed.Reference}).FirstOrCreate(&model).Error; err != nil {
			return err
		}
	}

	for _, paymentSeed := range billingPaymentSeeds() {
		account, ok := unitToAccount[paymentSeed.UnitCode]
		if !ok {
			return fmt.Errorf("missing account for unit %s", paymentSeed.UnitCode)
		}

		model := billing.Payment{
			AccountCode: account.AccountCode,
			UnitCode:    paymentSeed.UnitCode,
			Amount:      paymentSeed.Amount,
			PaidAt:      paymentSeed.PaidAt,
			Reference:   paymentSeed.Reference,
			Description: paymentSeed.Description,
			Source:      paymentSeed.Source,
			MethodID:    paymentSeed.MethodID,
			MethodLabel: paymentSeed.MethodLabel,
			Status:      paymentSeed.Status,
		}
		if err := db.Where(billing.Payment{Reference: paymentSeed.Reference}).FirstOrCreate(&model).Error; err != nil {
			return err
		}

		for _, chargeRef := range paymentSeed.ChargeRefs {
			var charge billing.Charge
			if err := db.Where("reference = ?", chargeRef).First(&charge).Error; err != nil {
				return err
			}

			allocation := billing.PaymentAllocation{
				PaymentID: model.ID,
				ChargeID:  charge.ID,
			}
			if err := db.Where(billing.PaymentAllocation{PaymentID: model.ID, ChargeID: charge.ID}).FirstOrCreate(&allocation).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func seedVisitors(db *gorm.DB) error {
	type visitorSeed struct {
		PublicID              string
		AccountCode           string
		VisitorName           string
		Purpose               string
		VisitDate             string
		ArrivalWindow         string
		VehiclePlate          string
		ParkingSlotsRequested int
		Status                string
		CreatedAt             time.Time
		UpdatedAt             time.Time
	}

	lookup := map[string]property.ResidentAccount{}
	var accounts []property.ResidentAccount
	if err := db.Find(&accounts).Error; err != nil {
		return err
	}
	for _, account := range accounts {
		lookup[account.AccountCode] = account
	}

	unitLookup := map[uint]property.Unit{}
	var units []property.Unit
	if err := db.Find(&units).Error; err != nil {
		return err
	}
	for _, unit := range units {
		unitLookup[unit.ID] = unit
	}

	areaLookup := map[uint]property.Area{}
	var areas []property.Area
	if err := db.Find(&areas).Error; err != nil {
		return err
	}
	for _, area := range areas {
		areaLookup[area.ID] = area
	}

	buildingLookup := map[uint]property.Building{}
	var buildings []property.Building
	if err := db.Find(&buildings).Error; err != nil {
		return err
	}
	for _, building := range buildings {
		buildingLookup[building.ID] = building
	}

	seeds := []visitorSeed{
		{
			PublicID:              "visitor-seed-1",
			AccountCode:           "acct-a1208",
			VisitorName:           "Melissa Tan",
			Purpose:               "Weekend family visit",
			VisitDate:             "2026-05-03",
			ArrivalWindow:         "02:00 PM - 05:00 PM",
			VehiclePlate:          "WVG 2210",
			ParkingSlotsRequested: 1,
			Status:                "approved",
			CreatedAt:             time.Date(2026, 4, 26, 8, 0, 0, 0, time.UTC),
			UpdatedAt:             time.Date(2026, 4, 26, 9, 15, 0, 0, time.UTC),
		},
		{
			PublicID:              "visitor-seed-2",
			AccountCode:           "acct-a1208",
			VisitorName:           "Hannah Lee",
			Purpose:               "Dinner guest visit",
			VisitDate:             "2026-04-28",
			ArrivalWindow:         "07:30 PM - 10:30 PM",
			VehiclePlate:          "WXY 8821",
			ParkingSlotsRequested: 1,
			Status:                "pending",
			CreatedAt:             time.Date(2026, 4, 27, 12, 10, 0, 0, time.UTC),
			UpdatedAt:             time.Date(2026, 4, 27, 12, 10, 0, 0, time.UTC),
		},
		{
			PublicID:              "visitor-seed-3",
			AccountCode:           "acct-b0411",
			VisitorName:           "Zen Movers",
			Purpose:               "Move-in support team",
			VisitDate:             "2026-04-29",
			ArrivalWindow:         "10:00 AM - 12:00 PM",
			VehiclePlate:          "VAN 3312",
			ParkingSlotsRequested: 2,
			Status:                "pending",
			CreatedAt:             time.Date(2026, 4, 28, 7, 45, 0, 0, time.UTC),
			UpdatedAt:             time.Date(2026, 4, 28, 7, 45, 0, 0, time.UTC),
		},
		{
			PublicID:              "visitor-seed-4",
			AccountCode:           "acct-pg03",
			VisitorName:           "Kelvin Ong",
			Purpose:               "Supplier drop-off",
			VisitDate:             "2026-04-27",
			ArrivalWindow:         "03:00 PM - 05:00 PM",
			VehiclePlate:          "BMQ 7190",
			ParkingSlotsRequested: 1,
			Status:                "approved",
			CreatedAt:             time.Date(2026, 4, 26, 6, 30, 0, 0, time.UTC),
			UpdatedAt:             time.Date(2026, 4, 26, 7, 20, 0, 0, time.UTC),
		},
		{
			PublicID:              "visitor-seed-5",
			AccountCode:           "acct-a1208",
			VisitorName:           "Aaron Goh",
			Purpose:               "Dinner gathering",
			VisitDate:             "2026-05-07",
			ArrivalWindow:         "07:00 PM - 09:00 PM",
			VehiclePlate:          "VJQ 5518",
			ParkingSlotsRequested: 1,
			Status:                "pending",
			CreatedAt:             time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
			UpdatedAt:             time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		},
	}

	for _, seed := range seeds {
		account, ok := lookup[seed.AccountCode]
		if !ok {
			return fmt.Errorf("missing resident account for visitor seed %s", seed.PublicID)
		}

		unit, ok := unitLookup[account.UnitID]
		if !ok {
			return fmt.Errorf("missing unit for visitor seed %s", seed.PublicID)
		}

		area, ok := areaLookup[unit.AreaID]
		if !ok {
			return fmt.Errorf("missing area for visitor seed %s", seed.PublicID)
		}

		building, ok := buildingLookup[area.BuildingID]
		if !ok {
			return fmt.Errorf("missing building for visitor seed %s", seed.PublicID)
		}

		model := visitors.VisitorRequest{
			PublicID:              seed.PublicID,
			AccountCode:           account.AccountCode,
			ResidentCode:          account.ResidentCode,
			ResidentName:          account.ResidentName,
			BuildingCode:          building.Code,
			BuildingName:          building.Name,
			UnitCode:              unit.Code,
			VisitorName:           seed.VisitorName,
			Purpose:               seed.Purpose,
			VisitDate:             seed.VisitDate,
			ArrivalWindow:         seed.ArrivalWindow,
			VehiclePlate:          seed.VehiclePlate,
			ParkingSlotsRequested: seed.ParkingSlotsRequested,
			Status:                seed.Status,
			CreatedAt:             seed.CreatedAt,
			UpdatedAt:             seed.UpdatedAt,
		}

		if err := db.Where(visitors.VisitorRequest{PublicID: seed.PublicID}).Assign(model).FirstOrCreate(&model).Error; err != nil {
			return err
		}
	}

	return nil
}

func seedComplaints(db *gorm.DB) error {
	type complaintSeed struct {
		PublicID    string
		Reference   string
		AccountCode string
		UnitCode    string
		Category    string
		Title       string
		Description string
		Location    string
		Priority    string
		Status      string
		CreatedAt   time.Time
		UpdatedAt   time.Time
		Updates     []complaints.ComplaintUpdate
	}

	lookup := map[string]property.ResidentAccount{}
	var accounts []property.ResidentAccount
	if err := db.Find(&accounts).Error; err != nil {
		return err
	}
	for _, account := range accounts {
		lookup[account.AccountCode] = account
	}

	seeds := []complaintSeed{
		{
			PublicID:    "complaint-seed-1",
			Reference:   "CMP-240381",
			AccountCode: "acct-a1208",
			UnitCode:    "A-12-08",
			Category:    "Facilities",
			Title:       "Water seepage from common corridor ceiling",
			Description: "There is recurring water seepage outside the unit near the common corridor ceiling and the drip becomes heavier during rain.",
			Location:    "Tower A, Level 12 common corridor",
			Priority:    "High",
			Status:      "in_progress",
			CreatedAt:   time.Date(2026, 4, 27, 9, 10, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 4, 29, 2, 15, 0, 0, time.UTC),
			Updates: []complaints.ComplaintUpdate{
				{PublicID: "complaint-seed-1-update-1", Status: "received", Title: "Received", Comment: "Complaint received and routed to the building operations queue.", CreatedAt: time.Date(2026, 4, 27, 9, 10, 0, 0, time.UTC)},
				{PublicID: "complaint-seed-1-update-2", Status: "in_progress", Title: "In Progress", Comment: "Site inspection arranged for the corridor ceiling leak and maintenance team is checking the source.", CreatedAt: time.Date(2026, 4, 29, 2, 15, 0, 0, time.UTC)},
			},
		},
		{
			PublicID:    "complaint-seed-2",
			Reference:   "CMP-238914",
			AccountCode: "acct-b0411",
			UnitCode:    "B-04-11",
			Category:    "Electrical",
			Title:       "Basement car park lighting not working",
			Description: "Several car park lights near Bay B-17 are not turning on at night and the area is too dim for safe walking.",
			Location:    "Tower B basement car park",
			Priority:    "Medium",
			Status:      "done",
			CreatedAt:   time.Date(2026, 4, 24, 11, 45, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 4, 27, 8, 0, 0, 0, time.UTC),
			Updates: []complaints.ComplaintUpdate{
				{PublicID: "complaint-seed-2-update-1", Status: "received", Title: "Received", Comment: "Complaint received and assigned for lighting inspection.", CreatedAt: time.Date(2026, 4, 24, 11, 45, 0, 0, time.UTC)},
				{PublicID: "complaint-seed-2-update-2", Status: "done", Title: "Done", Comment: "Lighting circuit was restored and the affected car park fittings are working again.", CreatedAt: time.Date(2026, 4, 27, 8, 0, 0, 0, time.UTC)},
			},
		},
		{
			PublicID:    "complaint-seed-3",
			Reference:   "CMP-240622",
			AccountCode: "acct-a1209",
			UnitCode:    "A-12-09",
			Category:    "Security",
			Title:       "Visitor intercom audio static",
			Description: "The guard house intercom audio is breaking up badly during visitor verification and guests cannot hear unit responses clearly.",
			Location:    "Tower A guard house intercom line",
			Priority:    "Medium",
			Status:      "received",
			CreatedAt:   time.Date(2026, 4, 28, 13, 25, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 4, 28, 13, 25, 0, 0, time.UTC),
			Updates: []complaints.ComplaintUpdate{
				{PublicID: "complaint-seed-3-update-1", Status: "received", Title: "Received", Comment: "Complaint received. Management will verify the intercom issue with the guard house team.", CreatedAt: time.Date(2026, 4, 28, 13, 25, 0, 0, time.UTC)},
			},
		},
	}

	for _, seed := range seeds {
		account, ok := lookup[seed.AccountCode]
		if !ok {
			return fmt.Errorf("missing account for complaint seed %s", seed.AccountCode)
		}

		var unit property.Unit
		if err := db.Where("id = ?", account.UnitID).First(&unit).Error; err != nil {
			return err
		}
		var area property.Area
		if err := db.Where("id = ?", unit.AreaID).First(&area).Error; err != nil {
			return err
		}
		var building property.Building
		if err := db.Where("id = ?", area.BuildingID).First(&building).Error; err != nil {
			return err
		}

		record := complaints.Complaint{
			PublicID:     seed.PublicID,
			Reference:    seed.Reference,
			AccountCode:  seed.AccountCode,
			ResidentCode: account.ResidentCode,
			ResidentName: account.ResidentName,
			BuildingName: building.Name,
			UnitCode:     seed.UnitCode,
			Category:     seed.Category,
			Title:        seed.Title,
			Description:  seed.Description,
			Location:     seed.Location,
			Priority:     seed.Priority,
			Status:       seed.Status,
			CreatedAt:    seed.CreatedAt,
			UpdatedAt:    seed.UpdatedAt,
		}

		if err := db.Where(complaints.Complaint{Reference: seed.Reference}).Assign(record).FirstOrCreate(&record).Error; err != nil {
			return err
		}

		for _, updateSeed := range seed.Updates {
			updateModel := updateSeed
			updateModel.ComplaintID = record.ID
			if err := db.Where(complaints.ComplaintUpdate{PublicID: updateSeed.PublicID}).Assign(updateModel).FirstOrCreate(&updateModel).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func seedFeedback(db *gorm.DB) error {
	type feedbackSeed struct {
		PublicID    string
		AccountCode string
		UnitCode    string
		Type        string
		Rating      string
		Details     string
		Status      string
		CreatedAt   time.Time
		UpdatedAt   time.Time
	}

	lookup := map[string]property.ResidentAccount{}
	var accounts []property.ResidentAccount
	if err := db.Find(&accounts).Error; err != nil {
		return err
	}
	for _, account := range accounts {
		lookup[account.AccountCode] = account
	}

	unitLookup := map[string]property.Unit{}
	var units []property.Unit
	if err := db.Find(&units).Error; err != nil {
		return err
	}
	for _, unit := range units {
		unitLookup[unit.Code] = unit
	}

	buildingNameByUnit := map[string]string{}
	for _, unit := range units {
		var area property.Area
		if err := db.Where("id = ?", unit.AreaID).First(&area).Error; err != nil {
			return err
		}

		var building property.Building
		if err := db.Where("id = ?", area.BuildingID).First(&building).Error; err != nil {
			return err
		}

		buildingNameByUnit[unit.Code] = building.Name
	}

	now := time.Now()
	seeds := []feedbackSeed{
		{
			PublicID:    "feedback-seed-1",
			AccountCode: "acct-a1208",
			UnitCode:    "A-12-08",
			Type:        "Suggestion",
			Rating:      "Good",
			Details:     "The lobby parcel shelves are helpful, but adding clearer collection labels would reduce confusion during peak hours.",
			Status:      "submitted",
			CreatedAt:   now.Add(-72 * time.Hour),
			UpdatedAt:   now.Add(-72 * time.Hour),
		},
		{
			PublicID:    "feedback-seed-2",
			AccountCode: "acct-b0411",
			UnitCode:    "B-04-11",
			Type:        "Praise",
			Rating:      "Excellent",
			Details:     "Weekend security response was fast and professional when the lift access card stopped working.",
			Status:      "submitted",
			CreatedAt:   now.Add(-36 * time.Hour),
			UpdatedAt:   now.Add(-36 * time.Hour),
		},
	}

	for _, seed := range seeds {
		account, ok := lookup[seed.AccountCode]
		if !ok {
			return fmt.Errorf("missing resident account for feedback seed %s", seed.PublicID)
		}

		if _, ok := unitLookup[seed.UnitCode]; !ok {
			return fmt.Errorf("missing unit for feedback seed %s", seed.PublicID)
		}

		model := feedback.Feedback{
			PublicID:     seed.PublicID,
			AccountCode:  seed.AccountCode,
			ResidentCode: account.ResidentCode,
			ResidentName: account.ResidentName,
			BuildingName: buildingNameByUnit[seed.UnitCode],
			UnitCode:     seed.UnitCode,
			Type:         seed.Type,
			Rating:       seed.Rating,
			Details:      seed.Details,
			Status:       seed.Status,
			CreatedAt:    seed.CreatedAt,
			UpdatedAt:    seed.UpdatedAt,
		}

		if err := db.Where(feedback.Feedback{PublicID: seed.PublicID}).Assign(model).FirstOrCreate(&model).Error; err != nil {
			return err
		}
	}

	return nil
}

func propertySeeds() []seedBuilding {
	return []seedBuilding{
		{
			Code:              "BLD-A",
			Name:              "Serene Heights Tower A",
			Status:            "Active building",
			DocumentsExpected: "Master plan|Fire compliance file",
			VisitorSlots:      8,
			Areas: []seedArea{
				{
					Code:              "AREA-A-LZ",
					Name:              "Low Zone",
					Status:            "Shared facilities zone",
					DocumentsExpected: "Area schematic|Maintenance zone report",
					Units: []seedUnit{
						{Code: "A-01-01", Name: "Unit A-01-01", Status: "Owner occupied", DocumentsExpected: "SPA / title|Resident registration form", AccountCode: "acct-a0101", ResidentCode: "RES-A0101-2026", ResidentName: "Amirul Faiz", Email: "amirul.faiz@example.com"},
						{Code: "A-01-02", Name: "Unit A-01-02", Status: "Tenanted", DocumentsExpected: "Tenancy agreement|Resident registration form", AccountCode: "acct-a0102", ResidentCode: "RES-A0102-2026", ResidentName: "Mei Ling Tan", Email: "mei.ling.tan@example.com"},
					},
				},
				{
					Code:              "AREA-A-MZ",
					Name:              "Mid Zone",
					Status:            "Shared facilities zone",
					DocumentsExpected: "Area schematic|Renovation notice register",
					Units: []seedUnit{
						{Code: "A-12-08", Name: "Unit A-12-08", Status: "Owner occupied", DocumentsExpected: "SPA / title|Resident registration form", AccountCode: "acct-a1208", ResidentCode: "RES-A1208-2026", ResidentName: "Syaril Nazirul", Email: "syarilnazirul.sazali@gmail.com"},
						{Code: "A-12-09", Name: "Unit A-12-09", Status: "Tenanted", DocumentsExpected: "Tenancy agreement|Resident registration form", AccountCode: "acct-a1209", ResidentCode: "RES-A1209-2026", ResidentName: "Daniel Wong", Email: "daniel.wong@example.com"},
					},
				},
				{
					Code:              "AREA-A-SZ",
					Name:              "Sky Zone",
					Status:            "Premium residential zone",
					DocumentsExpected: "Area schematic|Access control plan",
					Units: []seedUnit{
						{Code: "A-20-01", Name: "Unit A-20-01", Status: "Reserved", DocumentsExpected: "Booking form|Purchaser registration pack", AccountCode: "acct-a2001", ResidentCode: "", ResidentName: "Pending Occupant", Email: "pending.occupant@example.com"},
					},
				},
			},
		},
		{
			Code:              "BLD-B",
			Name:              "Serene Heights Tower B",
			Status:            "Active building",
			DocumentsExpected: "Master plan|CCC certificate",
			VisitorSlots:      6,
			Areas: []seedArea{
				{
					Code:              "AREA-B-E",
					Name:              "East Wing",
					Status:            "Shared facilities zone",
					DocumentsExpected: "Area schematic|Mechanical services drawing",
					Units: []seedUnit{
						{Code: "B-04-11", Name: "Unit B-04-11", Status: "Owner occupied", DocumentsExpected: "SPA / title|Resident registration form", AccountCode: "acct-b0411", ResidentCode: "RES-B0411-2026", ResidentName: "Syaril Nazirul", Email: "syarilnazirul.sazali@gmail.com"},
						{Code: "B-04-12", Name: "Unit B-04-12", Status: "Tenanted", DocumentsExpected: "Tenancy agreement|Resident registration form", AccountCode: "acct-b0412", ResidentCode: "RES-B0412-2026", ResidentName: "Kelvin Yap", Email: "kelvin.yap@example.com"},
					},
				},
				{
					Code:              "AREA-B-W",
					Name:              "West Wing",
					Status:            "Shared facilities zone",
					DocumentsExpected: "Area schematic|Renovation control checklist",
					Units: []seedUnit{
						{Code: "B-15-01", Name: "Unit B-15-01", Status: "Vacant", DocumentsExpected: "Vacancy checklist|Meter handover form", AccountCode: "acct-b1501", ResidentCode: "", ResidentName: "Vacant Unit", Email: "vacant.unit@example.com"},
						{Code: "B-15-02", Name: "Unit B-15-02", Status: "Tenanted", DocumentsExpected: "Tenancy agreement|Resident registration form", AccountCode: "acct-b1502", ResidentCode: "RES-B1502-2026", ResidentName: "Siti Kamilah", Email: "siti.kamilah@example.com"},
					},
				},
			},
		},
		{
			Code:              "BLD-C",
			Name:              "Crescent Bay Tower C",
			Status:            "New residential tower",
			DocumentsExpected: "Master plan|Fire compliance certificate",
			VisitorSlots:      5,
			Areas: []seedArea{
				{
					Code:              "AREA-C-E",
					Name:              "East Wing",
					Status:            "Floors 1 to 10",
					DocumentsExpected: "Area layout plan|Mechanical services drawing",
					Units: []seedUnit{
						{Code: "C-02-01", Name: "Unit C-02-01", Status: "Owner occupied", DocumentsExpected: "SPA / title|Resident registration form", AccountCode: "acct-c0201", ResidentCode: "RES-C0201-2026", ResidentName: "Harith Iskandar", Email: "harith.iskandar@example.com"},
						{Code: "C-02-02", Name: "Unit C-02-02", Status: "Tenanted", DocumentsExpected: "Tenancy agreement|Resident registration form", AccountCode: "acct-c0202", ResidentCode: "RES-C0202-2026", ResidentName: "Chloe Lim", Email: "chloe.lim@example.com"},
					},
				},
			},
		},
		{
			Code:              "BLD-P",
			Name:              "Retail Podium",
			Status:            "Retail block",
			DocumentsExpected: "Retail master plan|Trade license file",
			VisitorSlots:      4,
			Areas: []seedArea{
				{
					Code:              "AREA-P-G",
					Name:              "Ground Retail",
					Status:            "Street-facing lots",
					DocumentsExpected: "Retail fit-out guide|Trade access schedule",
					Units: []seedUnit{
						{Code: "P-G-03", Name: "Lot P-G-03", Status: "Trading", DocumentsExpected: "Tenancy agreement|Business registration", AccountCode: "acct-pg03", ResidentCode: "SHOP-PG03-2026", ResidentName: "Brew Yard Cafe", Email: "hello@brewyard.example.com"},
						{Code: "P-G-05", Name: "Lot P-G-05", Status: "Trading", DocumentsExpected: "Tenancy agreement|Business registration", AccountCode: "acct-pg05", ResidentCode: "SHOP-PG05-2026", ResidentName: "Common Ground Pharmacy", Email: "hello@commongroundpharmacy.example.com"},
					},
				},
			},
		},
	}
}

func billingChargeSeeds() []seedCharge {
	return []seedCharge{
		{UnitCode: "A-12-08", BillingType: "Maintenance", Category: "Management", PeriodLabel: "April 2026", Icon: "home-outline", Amount: 220, DueDate: "2026-05-05", PostedAt: "01 Apr 2026 • 09:00 AM", Reference: "APR-MTN-A1208", Description: "April 2026 maintenance charges", Source: "system"},
		{UnitCode: "A-12-08", BillingType: "Sinking Fund", Category: "Management", PeriodLabel: "April 2026", Icon: "wallet-outline", Amount: 220, DueDate: "2026-05-05", PostedAt: "01 Apr 2026 • 09:05 AM", Reference: "APR-SNK-A1208", Description: "April 2026 sinking fund contribution", Source: "system"},
		{UnitCode: "A-12-09", BillingType: "Maintenance", Category: "Management", PeriodLabel: "April 2026", Icon: "home-outline", Amount: 220, DueDate: "2026-05-05", PostedAt: "01 Apr 2026 • 09:10 AM", Reference: "APR-MTN-A1209", Description: "April 2026 maintenance charges", Source: "system"},
		{UnitCode: "A-12-09", BillingType: "Water Bill", Category: "Utility", PeriodLabel: "March 2026", Icon: "water-outline", Amount: 68, DueDate: "2026-05-05", PostedAt: "03 Apr 2026 • 10:30 AM", Reference: "APR-WTR-A1209", Description: "March 2026 metered water bill", Source: "system"},
		{UnitCode: "B-04-11", BillingType: "Maintenance", Category: "Management", PeriodLabel: "April 2026", Icon: "home-outline", Amount: 240, DueDate: "2026-04-20", PostedAt: "01 Apr 2026 • 09:12 AM", Reference: "APR-MTN-B0411", Description: "April 2026 maintenance charges", Source: "system"},
		{UnitCode: "B-04-11", BillingType: "Water Bill", Category: "Utility", PeriodLabel: "March 2026", Icon: "water-outline", Amount: 72, DueDate: "2026-04-20", PostedAt: "03 Apr 2026 • 10:40 AM", Reference: "APR-WTR-B0411", Description: "March 2026 metered water bill", Source: "system"},
		{UnitCode: "B-04-12", BillingType: "Maintenance", Category: "Management", PeriodLabel: "April 2026", Icon: "home-outline", Amount: 240, DueDate: "2026-05-10", PostedAt: "01 Apr 2026 • 09:18 AM", Reference: "APR-MTN-B0412", Description: "April 2026 maintenance charges", Source: "system"},
		{UnitCode: "C-02-01", BillingType: "Maintenance", Category: "Management", PeriodLabel: "April 2026", Icon: "home-outline", Amount: 210, DueDate: "2026-05-12", PostedAt: "01 Apr 2026 • 09:25 AM", Reference: "APR-MTN-C0201", Description: "April 2026 maintenance charges", Source: "system"},
		{UnitCode: "P-G-03", BillingType: "Utilities", Category: "Utility", PeriodLabel: "April 2026", Icon: "flash-outline", Amount: 180, DueDate: "2026-05-10", PostedAt: "02 Apr 2026 • 02:00 PM", Reference: "APR-UTIL-PG03", Description: "April 2026 shared utilities billing", Source: "system"},
		{UnitCode: "P-G-05", BillingType: "Utilities", Category: "Utility", PeriodLabel: "April 2026", Icon: "flash-outline", Amount: 200, DueDate: "2026-05-10", PostedAt: "02 Apr 2026 • 02:10 PM", Reference: "APR-UTIL-PG05", Description: "April 2026 shared utilities billing", Source: "system"},
		{UnitCode: "P-G-05", BillingType: "Access Card Replacement", Category: "Access", PeriodLabel: "April 2026", Icon: "card-outline", Amount: 50, DueDate: "2026-05-10", PostedAt: "12 Apr 2026 • 09:20 AM", Reference: "APR-AC-PG05", Description: "Access card replacement fee", Source: "building_admin"},
	}
}

func billingPaymentSeeds() []seedPayment {
	return []seedPayment{
		{UnitCode: "A-12-08", ChargeRefs: []string{"APR-MTN-A1208"}, Amount: 220, PaidAt: "24 Apr 2026 • 11:20 AM", Reference: "FPX-884201", Description: "Resident online settlement received", Source: "system", MethodID: "fpx-cimb", MethodLabel: "FPX via CIMB", Status: "successful"},
		{UnitCode: "B-04-12", ChargeRefs: []string{"APR-MTN-B0412"}, Amount: 120, PaidAt: "18 Apr 2026 • 02:45 PM", Reference: "DQN-410821", Description: "Partial settlement received online", Source: "resident_app", MethodID: "duitnow-qr", MethodLabel: "DuitNow QR", Status: "successful"},
		{UnitCode: "P-G-03", ChargeRefs: []string{"APR-UTIL-PG03"}, Amount: 180, PaidAt: "22 Apr 2026 • 03:08 PM", Reference: "OFFLINE-CHK-1009", Description: "Cheque payment recorded by management office", Source: "building_admin", MethodID: "offline_cheque", MethodLabel: "Cheque", Status: "successful"},
	}
}

type seedAnnouncement struct {
	PublicID             string
	Badge                string
	BadgeTone            string
	Title                string
	Description          string
	Icon                 string
	AccentColor          string
	ImageURI             string
	AffectedArea         string
	Schedule             string
	Contact              string
	SummaryTitle         string
	SummaryParagraphs    []string
	HighlightedAreaTitle string
	HighlightedAreaItems []string
	TimelineTitle        string
	TimelineParagraphs   []string
	ETALabel             string
	ETAValue             string
	TeamLabel            string
	TeamValue            string
	SupportTitle         string
	SupportDescription   string
	PublishedAt          time.Time
	Attachments          []announcements.AttachmentView
}

func seedAnnouncements(db *gorm.DB) error {
	for _, item := range announcementSeeds() {
		model := announcements.Announcement{
			PublicID:              item.PublicID,
			Status:                "published",
			AudienceScope:         "all_residents",
			Badge:                 item.Badge,
			BadgeTone:             item.BadgeTone,
			Title:                 item.Title,
			Description:           item.Description,
			Icon:                  item.Icon,
			AccentColor:           item.AccentColor,
			AffectedArea:          item.AffectedArea,
			Schedule:              item.Schedule,
			Contact:               item.Contact,
			SummaryTitle:          item.SummaryTitle,
			SummaryParagraphsRaw:  strings.Join(item.SummaryParagraphs, "\n"),
			HighlightTitle:        item.HighlightedAreaTitle,
			HighlightItemsRaw:     strings.Join(item.HighlightedAreaItems, "\n"),
			TimelineTitle:         item.TimelineTitle,
			TimelineParagraphsRaw: strings.Join(item.TimelineParagraphs, "\n"),
			ETALabel:              item.ETALabel,
			ETAValue:              item.ETAValue,
			TeamLabel:             item.TeamLabel,
			TeamValue:             item.TeamValue,
			SupportTitle:          item.SupportTitle,
			SupportDescription:    item.SupportDescription,
			ImageURL:              item.ImageURI,
			PublishedAt:           item.PublishedAt,
		}

		if err := db.Where(announcements.Announcement{PublicID: item.PublicID}).Assign(model).FirstOrCreate(&model).Error; err != nil {
			return err
		}

		for _, attachment := range item.Attachments {
			attachmentModel := announcements.Attachment{
				PublicID:       attachment.ID,
				AnnouncementID: model.ID,
				Title:          attachment.Title,
				Meta:           attachment.Meta,
				Type:           attachment.Type,
				ObjectKey:      nil,
				FileURL:        nil,
			}
			if err := db.Where(announcements.Attachment{PublicID: attachment.ID}).Assign(attachmentModel).FirstOrCreate(&attachmentModel).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func announcementSeeds() []seedAnnouncement {
	return []seedAnnouncement{
		{
			PublicID:             "water-main-repair",
			Badge:                "URGENT MAINTENANCE",
			BadgeTone:            "danger",
			Title:                "Domestic Water Pipe Rectification",
			Description:          "Notice of temporary water disruption affecting selected towers due to urgent rectification works at the pump room.",
			Icon:                 "construct-outline",
			AccentColor:          "#E77A34",
			ImageURI:             "https://images.unsplash.com/photo-1621905252507-b35492cc74b4?auto=format&fit=crop&w=1200&q=80",
			AffectedArea:         "Tower A, Tower B, podium toilets, and common facilities water points",
			Schedule:             "Estimated completion by 4:00 PM today",
			Contact:              "Management Office • Guard House Hotline",
			SummaryTitle:         "The Situation",
			SummaryParagraphs:    []string{"At approximately 8:45 AM today, the on-site team detected a leak at the domestic water pipe near the pump room. To allow urgent rectification works, water supply has been temporarily isolated for the affected towers and common areas below."},
			HighlightedAreaTitle: "Affected Areas",
			HighlightedAreaItems: []string{
				"Tower A and Tower B residential units",
				"Podium toilets and cleaner rooms",
				"Surau and multipurpose hall water points",
			},
			TimelineTitle:      "Resolution Timeline",
			TimelineParagraphs: []string{"The appointed contractor is currently rectifying the affected pipe section. Works are expected to complete by 4:00 PM today, subject to site conditions. When supply resumes, residents may notice temporary air or slight discoloration in the water. Please run the tap for one to two minutes before use."},
			ETALabel:           "Estimated Time",
			ETAValue:           "Until 4:00 PM today",
			TeamLabel:          "Team Assigned",
			TeamValue:          "Aqua Mech Engineering",
			SupportTitle:       "Still have questions?",
			SupportDescription: "Contact the management office or guard house if you need urgent assistance during the interruption.",
			PublishedAt:        mustParseSeedTime("2026-04-23T09:15:00+08:00"),
			Attachments: []announcements.AttachmentView{
				{ID: "official-notice", Title: "Water_Interruption_Notice_23042026.pdf", Meta: "2.4 MB • PDF Document", Type: "pdf"},
				{ID: "affected-zone-map", Title: "Affected_Towers_Map.jpg", Meta: "1.1 MB • Image", Type: "image"},
			},
		},
		{
			PublicID:             "elevator-modernization",
			Badge:                "COMMUNITY UPDATE",
			BadgeTone:            "brand",
			Title:                "Lift Interior Refurbishment",
			Description:          "Lift refurbishment works will begin next week at Tower C with phased closures to reduce inconvenience.",
			Icon:                 "business-outline",
			AccentColor:          "#50636F",
			ImageURI:             "https://images.unsplash.com/photo-1518005020951-eccb494ad742?auto=format&fit=crop&w=1200&q=80",
			AffectedArea:         "Tower C passenger lifts and ground floor lobby",
			Schedule:             "Works scheduled from 29 Apr to 10 May 2026",
			Contact:              "Management Office",
			SummaryTitle:         "The Situation",
			SummaryParagraphs:    []string{"The lift refurbishment project will upgrade cabin finishes, control panels, and lighting for Tower C. One lift will remain operational throughout the work period to maintain resident access."},
			HighlightedAreaTitle: "Impacted Zones",
			HighlightedAreaItems: []string{
				"Tower C passenger lifts",
				"Ground floor drop-off and lift lobby",
				"Move-in, move-out, and contractor access scheduling",
			},
			TimelineTitle:      "Resolution Timeline",
			TimelineParagraphs: []string{"Contractors will take one lift offline at a time to minimise disruption. Waiting time may be longer during peak periods, especially before office hours and after dinner. Notices and temporary directional signage will be placed at the lobby during each phase."},
			ETALabel:           "Project Duration",
			ETAValue:           "12 days scheduled",
			TeamLabel:          "Team Assigned",
			TeamValue:          "Kone Elevators Malaysia",
			SupportTitle:       "Need access coordination?",
			SupportDescription: "Contact management if you need lift booking support for movers, deliveries, or elderly residents.",
			PublishedAt:        mustParseSeedTime("2026-04-20T16:30:00+08:00"),
			Attachments: []announcements.AttachmentView{
				{ID: "upgrade-schedule", Title: "Lift_Upgrade_Schedule.pdf", Meta: "1.8 MB • PDF Document", Type: "pdf"},
			},
		},
	}
}

func mustParseSeedTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}
