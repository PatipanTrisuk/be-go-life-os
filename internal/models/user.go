package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID              uuid.UUID  `json:"id"`
	Username        string     `json:"username"`
	Email           string     `json:"email"`
	Password        string     `json:"-"`
	RoleID          uuid.UUID  `json:"role_id"`
	Role            string     `json:"role"` // role name (joined), for display
	Permissions     []string   `json:"permissions,omitempty"`
	PlanCode        string     `json:"plan_code"`
	AuthProvider    string     `json:"auth_provider"`
	DiscordID       *string    `json:"discord_id,omitempty"`
	GoogleID        *string    `json:"google_id,omitempty"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
