package visitors

import "time"

type VisitorRequest struct {
	ID                    uint   `gorm:"primaryKey"`
	PublicID              string `gorm:"uniqueIndex;size:64;not null"`
	AccountCode           string `gorm:"index;size:64;not null"`
	ResidentCode          string `gorm:"size:64;not null"`
	ResidentName          string `gorm:"size:255;not null"`
	BuildingCode          string `gorm:"index;size:64;not null"`
	BuildingName          string `gorm:"size:255;not null"`
	UnitCode              string `gorm:"index;size:64;not null"`
	VisitorName           string `gorm:"size:255;not null"`
	Purpose               string `gorm:"type:text;not null"`
	VisitDate             string `gorm:"index;size:16;not null"`
	ArrivalWindow         string `gorm:"size:64;not null"`
	VehiclePlate          string `gorm:"size:64;not null"`
	ParkingSlotsRequested int    `gorm:"not null"`
	Status                string `gorm:"size:32;not null"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}
