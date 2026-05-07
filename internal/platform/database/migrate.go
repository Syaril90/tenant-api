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
	if err := db.AutoMigrate(
		&property.Building{},
		&property.Area{},
		&property.Unit{},
		&property.ResidentAccount{},
		&property.OwnerTenantRegistration{},
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
	); err != nil {
		return err
	}

	// AutoMigrate does not remove obsolete unique indexes, so clean up the
	// pre-multi-tenant constraint if this database was created before the change.
	if db.Migrator().HasIndex(&property.OwnerTenantRegistration{}, "idx_owner_tenant_unit") {
		if err := db.Migrator().DropIndex(&property.OwnerTenantRegistration{}, "idx_owner_tenant_unit"); err != nil {
			return err
		}
	}

	return nil
}
