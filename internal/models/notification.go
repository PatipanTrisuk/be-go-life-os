package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	NotificationTypeExportQueued = "export_queued"
	NotificationTypeExportReady  = "export_ready"
	NotificationTypeExportFailed = "export_failed"
	NotificationTypeReminderDue  = "reminder_due"
	NotificationRefExportJob     = "export_job"
	NotificationRefReminder      = "reminder"
)

type Notification struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	Type      string     `json:"type"`
	RefType   *string    `json:"ref_type,omitempty"`
	RefID     *uuid.UUID `json:"ref_id,omitempty"`
	MetaJSON  string     `json:"meta"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
