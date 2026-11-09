package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"go-be/internal/models"
	"go-be/internal/repository"
)

var ErrReminderNotFound = errors.New("reminder not found")

type ReminderDTO struct {
	ID         uuid.UUID `json:"id"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	DueAt      string    `json:"due_at"`
	Status     string    `json:"status"`
	NotifiedAt *string   `json:"notified_at,omitempty"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

type ReminderCreateInput struct {
	Title  string `json:"title" binding:"required,min=1,max=200"`
	Body   string `json:"body"`
	DueAt  string `json:"due_at" binding:"required"`
	Status string `json:"status"`
}

type ReminderUpdateInput struct {
	Title  *string `json:"title"`
	Body   *string `json:"body"`
	DueAt  *string `json:"due_at"`
	Status *string `json:"status"`
}

type ReminderCheckResult struct {
	Created int           `json:"created"`
	Items   []ReminderDTO `json:"items"`
}

type ReminderService struct {
	Repo                *repository.ReminderRepository
	NotificationService *NotificationService
	Entitlements        *EntitlementService
}

func NewReminderService(repo *repository.ReminderRepository, notificationService *NotificationService, entitlements *EntitlementService) *ReminderService {
	return &ReminderService{Repo: repo, NotificationService: notificationService, Entitlements: entitlements}
}

func toReminderDTO(r models.Reminder) ReminderDTO {
	return ReminderDTO{
		ID:         r.ID,
		Title:      r.Title,
		Body:       r.Body,
		DueAt:      r.DueAt.UTC().Format(time.RFC3339),
		Status:     r.Status,
		NotifiedAt: formatTimePtr(r.NotifiedAt),
		CreatedAt:  r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func normalizeReminderStatus(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", models.ReminderStatusPending:
		return models.ReminderStatusPending, nil
	case models.ReminderStatusDone, models.ReminderStatusDismissed:
		return s, nil
	default:
		return "", ErrInvalidInput
	}
}

func (s *ReminderService) List(ctx context.Context, userID, statusRaw string) ([]ReminderDTO, error) {
	status := ""
	if strings.TrimSpace(statusRaw) != "" {
		st, err := normalizeReminderStatus(statusRaw)
		if err != nil {
			return nil, err
		}
		status = st
	}
	items, err := s.Repo.List(ctx, userID, status)
	if err != nil {
		return nil, err
	}
	out := make([]ReminderDTO, 0, len(items))
	for _, item := range items {
		out = append(out, toReminderDTO(item))
	}
	return out, nil
}

func (s *ReminderService) Get(ctx context.Context, userID, id string) (*ReminderDTO, error) {
	r, err := s.Repo.GetByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReminderNotFound
		}
		return nil, err
	}
	dto := toReminderDTO(*r)
	return &dto, nil
}

func (s *ReminderService) Create(ctx context.Context, userID string, in ReminderCreateInput) (*ReminderDTO, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > 200 {
		return nil, ErrInvalidInput
	}
	dueAt, err := parseDateTimeInput(in.DueAt, false)
	if err != nil {
		return nil, err
	}
	status, err := normalizeReminderStatus(in.Status)
	if err != nil {
		return nil, err
	}

	if err := s.Entitlements.EnsureCanCreate(ctx, userID, models.FeatureLimitReminders); err != nil {
		return nil, err
	}
	r, err := s.Repo.Create(ctx, repository.ReminderCreateParams{
		UserID: userID,
		Title:  title,
		Body:   strings.TrimSpace(in.Body),
		DueAt:  dueAt,
		Status: status,
	})
	if err != nil {
		return nil, err
	}
	dto := toReminderDTO(*r)
	return &dto, nil
}

func (s *ReminderService) Update(ctx context.Context, userID, id string, in ReminderUpdateInput) (*ReminderDTO, error) {
	existing, err := s.Repo.GetByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReminderNotFound
		}
		return nil, err
	}

	params := repository.ReminderUpdateParams{}
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" || len(title) > 200 {
			return nil, ErrInvalidInput
		}
		params.Title = &title
	}
	if in.Body != nil {
		b := strings.TrimSpace(*in.Body)
		params.Body = &b
	}
	if in.DueAt != nil {
		t, err := parseDateTimeInput(*in.DueAt, false)
		if err != nil {
			return nil, err
		}
		params.DueAt = &t
		if !t.Equal(existing.DueAt) {
			params.ResetNotified = true
		}
	}
	if in.Status != nil {
		st, err := normalizeReminderStatus(*in.Status)
		if err != nil {
			return nil, err
		}
		params.Status = &st
		if st == models.ReminderStatusPending && existing.Status != models.ReminderStatusPending {
			params.ResetNotified = true
		}
	}

	r, err := s.Repo.Update(ctx, id, userID, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReminderNotFound
		}
		return nil, err
	}
	dto := toReminderDTO(*r)
	return &dto, nil
}

func (s *ReminderService) MarkDone(ctx context.Context, userID, id string) (*ReminderDTO, error) {
	done := models.ReminderStatusDone
	return s.Update(ctx, userID, id, ReminderUpdateInput{Status: &done})
}

func (s *ReminderService) Delete(ctx context.Context, userID, id string) error {
	if err := s.Repo.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrReminderNotFound
		}
		return err
	}
	return nil
}

// Check finds due, un-notified pending reminders, creates a notification for each,
// and marks them notified.
func (s *ReminderService) Check(ctx context.Context, userID string) (*ReminderCheckResult, error) {
	now := time.Now().UTC()
	due, err := s.Repo.ListDue(ctx, userID, now)
	if err != nil {
		return nil, err
	}

	result := &ReminderCheckResult{Items: make([]ReminderDTO, 0, len(due))}
	refType := models.NotificationRefReminder
	for _, rm := range due {
		// Claim first so concurrent checks do not double-notify.
		claimed, err := s.Repo.MarkNotified(ctx, rm.ID.String(), userID, now)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return nil, err
		}

		refID := rm.ID
		if s.NotificationService != nil {
			err := s.NotificationService.Create(
				ctx,
				userID,
				models.NotificationTypeReminderDue,
				&refType,
				&refID,
				map[string]any{
					"title":  rm.Title,
					"due_at": rm.DueAt.UTC().Format(time.RFC3339),
				},
			)
			if err != nil {
				// Release the claim so a later check can retry.
				_, _ = s.Repo.Update(ctx, rm.ID.String(), userID, repository.ReminderUpdateParams{ResetNotified: true})
				return nil, err
			}
		}
		result.Items = append(result.Items, toReminderDTO(*claimed))
		result.Created++
	}
	return result, nil
}
