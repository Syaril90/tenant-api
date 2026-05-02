package property

import "time"

type Building struct {
	ID                uint   `gorm:"primaryKey"`
	Code              string `gorm:"uniqueIndex;size:64;not null"`
	Name              string `gorm:"size:255;not null"`
	Status            string `gorm:"size:255;not null"`
	DocumentsExpected string `gorm:"type:text;not null"`
	VisitorSlots      int    `gorm:"not null"`
	Areas             []Area
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Area struct {
	ID                uint   `gorm:"primaryKey"`
	BuildingID        uint   `gorm:"index;not null"`
	Code              string `gorm:"uniqueIndex;size:64;not null"`
	Name              string `gorm:"size:255;not null"`
	Status            string `gorm:"size:255;not null"`
	DocumentsExpected string `gorm:"type:text;not null"`
	Units             []Unit
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Unit struct {
	ID                uint   `gorm:"primaryKey"`
	AreaID            uint   `gorm:"index;not null"`
	Code              string `gorm:"uniqueIndex;size:64;not null"`
	Name              string `gorm:"size:255;not null"`
	Status            string `gorm:"size:255;not null"`
	DocumentsExpected string `gorm:"type:text;not null"`
	ResidentAccount   ResidentAccount
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ResidentAccount struct {
	ID           uint   `gorm:"primaryKey"`
	UnitID       uint   `gorm:"uniqueIndex;not null"`
	AccountCode  string `gorm:"uniqueIndex;size:64;not null"`
	ResidentCode string `gorm:"size:64;not null"`
	ResidentName string `gorm:"size:255;not null"`
	Email        string `gorm:"index;size:255;not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
