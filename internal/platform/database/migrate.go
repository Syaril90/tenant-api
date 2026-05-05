package database

import (
	"modular-api/internal/modules/announcements"
	"modular-api/internal/modules/billing"
	"modular-api/internal/modules/complaints"
	"modular-api/internal/modules/documents"
	"modular-api/internal/modules/feedback"
	"modular-api/internal/modules/hub"
	"modular-api/internal/modules/property"
	"modular-api/internal/modules/visitors"

	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&property.Building{},
		&property.Area{},
		&property.Unit{},
		&property.ResidentAccount{},
		&billing.Charge{},
		&billing.Payment{},
		&billing.PaymentAllocation{},
		&billing.GatewayTransaction{},
		&announcements.MediaAsset{},
		&announcements.Announcement{},
		&announcements.Attachment{},
		&documents.Document{},
		&documents.DocumentRequest{},
		&documents.DocumentRequestAttachment{},
		&documents.DocumentRequestUpdate{},
		&complaints.Complaint{},
		&complaints.ComplaintAttachment{},
		&complaints.ComplaintUpdate{},
		&feedback.Feedback{},
		&feedback.FeedbackAttachment{},
		&hub.Post{},
		&hub.Reply{},
		&hub.PostLike{},
		&visitors.VisitorRequest{},
	)
}
