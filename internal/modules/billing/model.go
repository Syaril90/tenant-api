package billing

import "time"

type Charge struct {
	ID          uint    `gorm:"primaryKey"`
	UnitCode    string  `gorm:"index;size:64;not null"`
	AccountCode string  `gorm:"index;size:64;not null"`
	Category    string  `gorm:"size:64;not null"`
	BillingType string  `gorm:"size:128;not null"`
	PeriodLabel string  `gorm:"size:128;not null"`
	Icon        string  `gorm:"size:128;not null"`
	Amount      float64 `gorm:"not null"`
	DueDate     string  `gorm:"size:32;not null"`
	PostedAt    string  `gorm:"size:64;not null"`
	Reference   string  `gorm:"uniqueIndex;size:128;not null"`
	Description string  `gorm:"type:text;not null"`
	Source      string  `gorm:"size:32;not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Allocations []PaymentAllocation
}

type Payment struct {
	ID          uint    `gorm:"primaryKey"`
	AccountCode string  `gorm:"index;size:64;not null"`
	UnitCode    string  `gorm:"index;size:64;not null"`
	Amount      float64 `gorm:"not null"`
	PaidAt      string  `gorm:"size:64;not null"`
	Reference   string  `gorm:"uniqueIndex;size:128;not null"`
	Description string  `gorm:"type:text;not null"`
	Source      string  `gorm:"size:32;not null"`
	MethodID    string  `gorm:"size:64;not null"`
	MethodLabel string  `gorm:"size:128;not null"`
	Status      string  `gorm:"size:32;not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Allocations []PaymentAllocation
}

type PaymentAllocation struct {
	ID        uint    `gorm:"primaryKey"`
	PaymentID uint    `gorm:"index;not null"`
	ChargeID  uint    `gorm:"index;not null"`
	Amount    float64 `gorm:"not null;default:0"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type GatewayTransaction struct {
	ID               uint    `gorm:"primaryKey"`
	Gateway          string  `gorm:"index;size:64;not null"`
	ExternalID       string  `gorm:"index;size:128;not null"`
	AccountCode      string  `gorm:"index;size:64;not null"`
	UnitCode         string  `gorm:"index;size:64;not null"`
	Reference        string  `gorm:"uniqueIndex;size:128;not null"`
	Amount           float64 `gorm:"not null"`
	Currency         string  `gorm:"size:8;not null"`
	Status           string  `gorm:"size:32;not null"`
	PayerName        string  `gorm:"size:255;not null"`
	PayerEmail       string  `gorm:"size:255;not null"`
	ChargeReferences string  `gorm:"type:text;not null"`
	CheckoutURL      string  `gorm:"type:text;not null"`
	RedirectURL      string  `gorm:"type:text;not null"`
	CallbackPayload  string  `gorm:"type:text;not null"`
	SettledPaymentID *uint
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
