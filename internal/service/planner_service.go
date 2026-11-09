package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"go-be/internal/models"
	"go-be/internal/repository"
)

var ErrPlannerNotFound = errors.New("planner task not found")

var timeHHMM = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

type PlannerTaskDTO struct {
	ID          uuid.UUID `json:"id"`
	PlanDate    string    `json:"plan_date"`
	Title       string    `json:"title"`
	Notes       string    `json:"notes"`
	Status      string    `json:"status"`
	Priority    string    `json:"priority"`
	StartTime   string    `json:"start_time"`
	EndTime     string    `json:"end_time"`
	SortOrder   int       `json:"sort_order"`
	CompletedAt *string   `json:"completed_at,omitempty"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

type PlannerCreateInput struct {
	PlanDate  string `json:"plan_date"`
	Title     string `json:"title" binding:"required,min=1,max=200"`
	Notes     string `json:"notes"`
	Status    string `json:"status"`
	Priority  string `json:"priority"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	SortOrder *int   `json:"sort_order"`
}

type PlannerUpdateInput struct {
	Title     *string `json:"title"`
	Notes     *string `json:"notes"`
	Status    *string `json:"status"`
	Priority  *string `json:"priority"`
	StartTime *string `json:"start_time"`
	EndTime   *string `json:"end_time"`
	SortOrder *int    `json:"sort_order"`
	PlanDate  *string `json:"plan_date"`
}

type PlannerService struct {
	Repo         *repository.PlannerRepository
	Entitlements *EntitlementService
}

func NewPlannerService(repo *repository.PlannerRepository, entitlements *EntitlementService) *PlannerService {
	return &PlannerService{Repo: repo, Entitlements: entitlements}
}

func toPlannerDTO(t models.PlannerTask) PlannerTaskDTO {
	return PlannerTaskDTO{
		ID:          t.ID,
		PlanDate:    t.PlanDate.UTC().Format("2006-01-02"),
		Title:       t.Title,
		Notes:       t.Notes,
		Status:      t.Status,
		Priority:    t.Priority,
		StartTime:   t.StartTime,
		EndTime:     t.EndTime,
		SortOrder:   t.SortOrder,
		CompletedAt: formatTimePtr(t.CompletedAt),
		CreatedAt:   t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   t.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func normalizePlannerStatus(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", models.PlannerStatusTodo:
		return models.PlannerStatusTodo, nil
	case models.PlannerStatusDone, models.PlannerStatusSkipped:
		return s, nil
	default:
		return "", ErrInvalidInput
	}
}

func normalizePlannerPriority(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", models.PlannerPriorityMedium:
		return models.PlannerPriorityMedium, nil
	case models.PlannerPriorityLow, models.PlannerPriorityHigh:
		return s, nil
	default:
		return "", ErrInvalidInput
	}
}

func normalizeClock(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !timeHHMM.MatchString(raw) {
		return "", ErrInvalidInput
	}
	return raw, nil
}

func parsePlanDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return truncateDate(time.Now().UTC()), nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, ErrInvalidInput
	}
	return truncateDate(t), nil
}

func (s *PlannerService) List(ctx context.Context, userID, dateRaw string) ([]PlannerTaskDTO, error) {
	date, err := parsePlanDate(dateRaw)
	if err != nil {
		return nil, err
	}
	items, err := s.Repo.ListByDate(ctx, userID, date)
	if err != nil {
		return nil, err
	}
	out := make([]PlannerTaskDTO, 0, len(items))
	for _, item := range items {
		out = append(out, toPlannerDTO(item))
	}
	return out, nil
}

func (s *PlannerService) Stats(ctx context.Context, userID, dateRaw string) (*models.PlannerDayStats, error) {
	date, err := parsePlanDate(dateRaw)
	if err != nil {
		return nil, err
	}
	return s.Repo.DayStats(ctx, userID, date)
}

func (s *PlannerService) Get(ctx context.Context, userID, id string) (*PlannerTaskDTO, error) {
	t, err := s.Repo.GetByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlannerNotFound
		}
		return nil, err
	}
	dto := toPlannerDTO(*t)
	return &dto, nil
}

func (s *PlannerService) Create(ctx context.Context, userID string, in PlannerCreateInput) (*PlannerTaskDTO, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > 200 {
		return nil, ErrInvalidInput
	}
	date, err := parsePlanDate(in.PlanDate)
	if err != nil {
		return nil, err
	}
	status, err := normalizePlannerStatus(in.Status)
	if err != nil {
		return nil, err
	}
	priority, err := normalizePlannerPriority(in.Priority)
	if err != nil {
		return nil, err
	}
	start, err := normalizeClock(in.StartTime)
	if err != nil {
		return nil, err
	}
	end, err := normalizeClock(in.EndTime)
	if err != nil {
		return nil, err
	}
	sortOrder := 0
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	} else {
		next, err := s.Repo.NextSortOrder(ctx, userID, date)
		if err != nil {
			return nil, err
		}
		sortOrder = next
	}

	if err := s.Entitlements.EnsureCanCreatePlanner(ctx, userID, date); err != nil {
		return nil, err
	}
	t, err := s.Repo.Create(ctx, repository.PlannerCreateParams{
		UserID:    userID,
		PlanDate:  date,
		Title:     title,
		Notes:     strings.TrimSpace(in.Notes),
		Status:    status,
		Priority:  priority,
		StartTime: start,
		EndTime:   end,
		SortOrder: sortOrder,
	})
	if err != nil {
		return nil, err
	}
	if status == models.PlannerStatusDone {
		now := time.Now().UTC()
		completed := &now
		t, err = s.Repo.Update(ctx, t.ID.String(), userID, repository.PlannerUpdateParams{
			CompletedAt: &completed,
		})
		if err != nil {
			return nil, err
		}
	}
	dto := toPlannerDTO(*t)
	return &dto, nil
}

func (s *PlannerService) Update(ctx context.Context, userID, id string, in PlannerUpdateInput) (*PlannerTaskDTO, error) {
	existing, err := s.Repo.GetByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlannerNotFound
		}
		return nil, err
	}

	params := repository.PlannerUpdateParams{}
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" || len(title) > 200 {
			return nil, ErrInvalidInput
		}
		params.Title = &title
	}
	if in.Notes != nil {
		n := strings.TrimSpace(*in.Notes)
		params.Notes = &n
	}
	if in.Status != nil {
		status, err := normalizePlannerStatus(*in.Status)
		if err != nil {
			return nil, err
		}
		params.Status = &status
		if status == models.PlannerStatusDone {
			now := time.Now().UTC()
			completed := &now
			params.CompletedAt = &completed
		} else if existing.Status == models.PlannerStatusDone {
			var nilTime *time.Time
			params.CompletedAt = &nilTime
		}
	}
	if in.Priority != nil {
		priority, err := normalizePlannerPriority(*in.Priority)
		if err != nil {
			return nil, err
		}
		params.Priority = &priority
	}
	if in.StartTime != nil {
		start, err := normalizeClock(*in.StartTime)
		if err != nil {
			return nil, err
		}
		params.StartTime = &start
	}
	if in.EndTime != nil {
		end, err := normalizeClock(*in.EndTime)
		if err != nil {
			return nil, err
		}
		params.EndTime = &end
	}
	if in.SortOrder != nil {
		params.SortOrder = in.SortOrder
	}

	// plan_date change via recreate fields not supported in UpdateParams — skip for simplicity
	_ = in.PlanDate

	t, err := s.Repo.Update(ctx, id, userID, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlannerNotFound
		}
		return nil, err
	}
	dto := toPlannerDTO(*t)
	return &dto, nil
}

func (s *PlannerService) Toggle(ctx context.Context, userID, id string) (*PlannerTaskDTO, error) {
	existing, err := s.Repo.GetByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlannerNotFound
		}
		return nil, err
	}
	next := models.PlannerStatusDone
	if existing.Status == models.PlannerStatusDone {
		next = models.PlannerStatusTodo
	}
	return s.Update(ctx, userID, id, PlannerUpdateInput{Status: &next})
}

func (s *PlannerService) Delete(ctx context.Context, userID, id string) error {
	if err := s.Repo.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPlannerNotFound
		}
		return err
	}
	return nil
}
