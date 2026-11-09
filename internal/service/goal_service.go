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

var (
	ErrGoalNotFound      = errors.New("goal not found")
	ErrMilestoneNotFound = errors.New("milestone not found")
)

type GoalDTO struct {
	ID           uuid.UUID          `json:"id"`
	Title        string             `json:"title"`
	Description  string             `json:"description"`
	Status       string             `json:"status"`
	Priority     string             `json:"priority"`
	Category     string             `json:"category"`
	Progress     int                `json:"progress"`
	TargetValue  *float64           `json:"target_value,omitempty"`
	CurrentValue float64            `json:"current_value"`
	Unit         string             `json:"unit"`
	StartDate    *string            `json:"start_date,omitempty"`
	TargetDate   *string            `json:"target_date,omitempty"`
	CompletedAt  *string            `json:"completed_at,omitempty"`
	ArchivedAt   *string            `json:"archived_at,omitempty"`
	CreatedAt    string             `json:"created_at"`
	UpdatedAt    string             `json:"updated_at"`
	Milestones   []GoalMilestoneDTO `json:"milestones,omitempty"`
}

type GoalMilestoneDTO struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	IsCompleted bool      `json:"is_completed"`
	SortOrder   int       `json:"sort_order"`
	CompletedAt *string   `json:"completed_at,omitempty"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

type GoalCreateInput struct {
	Title        string   `json:"title" binding:"required,min=1,max=200"`
	Description  string   `json:"description"`
	Status       string   `json:"status"`
	Priority     string   `json:"priority"`
	Category     string   `json:"category"`
	Progress     *int     `json:"progress"`
	TargetValue  *float64 `json:"target_value"`
	CurrentValue *float64 `json:"current_value"`
	Unit         string   `json:"unit"`
	StartDate    *string  `json:"start_date"`
	TargetDate   *string  `json:"target_date"`
}

type GoalUpdateInput struct {
	Title        *string  `json:"title"`
	Description  *string  `json:"description"`
	Status       *string  `json:"status"`
	Priority     *string  `json:"priority"`
	Category     *string  `json:"category"`
	Progress     *int     `json:"progress"`
	TargetValue  *float64 `json:"target_value"`
	ClearTarget  bool     `json:"clear_target_value"`
	CurrentValue *float64 `json:"current_value"`
	Unit         *string  `json:"unit"`
	StartDate    *string  `json:"start_date"`
	ClearStart   bool     `json:"clear_start_date"`
	TargetDate   *string  `json:"target_date"`
	ClearTargetD bool     `json:"clear_target_date"`
}

type MilestoneCreateInput struct {
	Title     string `json:"title" binding:"required,min=1,max=200"`
	SortOrder *int   `json:"sort_order"`
}

type MilestoneUpdateInput struct {
	Title       *string `json:"title"`
	IsCompleted *bool   `json:"is_completed"`
	SortOrder   *int    `json:"sort_order"`
}

type GoalListResult struct {
	Items []GoalDTO `json:"items"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
	Total int       `json:"total"`
}

type GoalService struct {
	Repo         *repository.GoalRepository
	Entitlements *EntitlementService
}

func NewGoalService(repo *repository.GoalRepository, entitlements *EntitlementService) *GoalService {
	return &GoalService{Repo: repo, Entitlements: entitlements}
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func formatDatePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format("2006-01-02")
	return &s
}

func parseDatePtr(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*raw)
	if s == "" {
		return nil, ErrInvalidInput
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, ErrInvalidInput
	}
	return &t, nil
}

func toGoalDTO(g models.Goal, milestones []models.GoalMilestone) GoalDTO {
	dto := GoalDTO{
		ID:           g.ID,
		Title:        g.Title,
		Description:  g.Description,
		Status:       g.Status,
		Priority:     g.Priority,
		Category:     g.Category,
		Progress:     g.Progress,
		TargetValue:  g.TargetValue,
		CurrentValue: g.CurrentValue,
		Unit:         g.Unit,
		StartDate:    formatDatePtr(g.StartDate),
		TargetDate:   formatDatePtr(g.TargetDate),
		CompletedAt:  formatTimePtr(g.CompletedAt),
		ArchivedAt:   formatTimePtr(g.ArchivedAt),
		CreatedAt:    g.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    g.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if milestones != nil {
		dto.Milestones = make([]GoalMilestoneDTO, 0, len(milestones))
		for _, m := range milestones {
			dto.Milestones = append(dto.Milestones, toMilestoneDTO(m))
		}
	}
	return dto
}

func toMilestoneDTO(m models.GoalMilestone) GoalMilestoneDTO {
	return GoalMilestoneDTO{
		ID:          m.ID,
		Title:       m.Title,
		IsCompleted: m.IsCompleted,
		SortOrder:   m.SortOrder,
		CompletedAt: formatTimePtr(m.CompletedAt),
		CreatedAt:   m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   m.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func normalizeStatus(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", models.GoalStatusActive:
		return models.GoalStatusActive, nil
	case models.GoalStatusDraft, models.GoalStatusCompleted, models.GoalStatusCancelled, models.GoalStatusArchived:
		return s, nil
	default:
		return "", ErrInvalidInput
	}
}

func normalizePriority(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", models.GoalPriorityMedium:
		return models.GoalPriorityMedium, nil
	case models.GoalPriorityLow, models.GoalPriorityHigh:
		return s, nil
	default:
		return "", ErrInvalidInput
	}
}

func clampProgress(n int) int {
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}

func computeProgress(current float64, target *float64, manual int, completed, totalMilestones int) int {
	if totalMilestones > 0 {
		return clampProgress(int(float64(completed) / float64(totalMilestones) * 100))
	}
	if target != nil && *target > 0 {
		return clampProgress(int(current / *target * 100))
	}
	return clampProgress(manual)
}

func (s *GoalService) syncProgressAndStatus(ctx context.Context, goal *models.Goal) (*models.Goal, error) {
	completed, total, err := s.Repo.MilestoneProgress(ctx, goal.ID.String(), goal.UserID.String())
	if err != nil {
		return nil, err
	}

	progress := computeProgress(goal.CurrentValue, goal.TargetValue, goal.Progress, completed, total)
	params := repository.GoalUpdateParams{
		Progress: &progress,
	}

	now := time.Now().UTC()
	var completedAt **time.Time
	var status *string

	switch {
	case progress >= 100 && goal.Status != models.GoalStatusCompleted &&
		goal.Status != models.GoalStatusCancelled && goal.Status != models.GoalStatusArchived:
		st := models.GoalStatusCompleted
		status = &st
		t := &now
		completedAt = &t
	case progress < 100 && goal.Status == models.GoalStatusCompleted:
		st := models.GoalStatusActive
		status = &st
		var nilTime *time.Time
		completedAt = &nilTime
	}

	params.Status = status
	params.CompletedAt = completedAt

	updated, err := s.Repo.Update(ctx, goal.ID.String(), goal.UserID.String(), params)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *GoalService) Create(ctx context.Context, userID string, in GoalCreateInput) (*GoalDTO, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > 200 {
		return nil, ErrInvalidInput
	}

	status, err := normalizeStatus(in.Status)
	if err != nil {
		return nil, err
	}
	priority, err := normalizePriority(in.Priority)
	if err != nil {
		return nil, err
	}

	startDate, err := parseDatePtr(in.StartDate)
	if err != nil {
		return nil, err
	}
	targetDate, err := parseDatePtr(in.TargetDate)
	if err != nil {
		return nil, err
	}

	progress := 0
	if in.Progress != nil {
		progress = clampProgress(*in.Progress)
	}
	current := 0.0
	if in.CurrentValue != nil {
		current = *in.CurrentValue
	}
	if in.TargetValue != nil && *in.TargetValue < 0 {
		return nil, ErrInvalidInput
	}

	progress = computeProgress(current, in.TargetValue, progress, 0, 0)

	var completedAt *time.Time
	if progress >= 100 && status == models.GoalStatusActive {
		status = models.GoalStatusCompleted
		now := time.Now().UTC()
		completedAt = &now
	}

	params := repository.GoalCreateParams{
		UserID:       userID,
		Title:        title,
		Description:  strings.TrimSpace(in.Description),
		Status:       status,
		Priority:     priority,
		Category:     strings.TrimSpace(in.Category),
		Progress:     progress,
		TargetValue:  in.TargetValue,
		CurrentValue: current,
		Unit:         strings.TrimSpace(in.Unit),
		StartDate:    startDate,
		TargetDate:   targetDate,
	}

	if err := s.Entitlements.EnsureCanCreate(ctx, userID, models.FeatureLimitGoals); err != nil {
		return nil, err
	}
	goal, err := s.Repo.Create(ctx, params)
	if err != nil {
		return nil, err
	}

	if completedAt != nil {
		ca := &completedAt
		goal, err = s.Repo.Update(ctx, goal.ID.String(), userID, repository.GoalUpdateParams{
			CompletedAt: ca,
		})
		if err != nil {
			return nil, err
		}
	}

	dto := toGoalDTO(*goal, []models.GoalMilestone{})
	return &dto, nil
}

func (s *GoalService) Get(ctx context.Context, userID, id string) (*GoalDTO, error) {
	goal, err := s.Repo.GetByID(ctx, id, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGoalNotFound
	}
	if err != nil {
		return nil, err
	}
	milestones, err := s.Repo.ListMilestones(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	dto := toGoalDTO(*goal, milestones)
	return &dto, nil
}

func (s *GoalService) List(ctx context.Context, userID string, f repository.GoalListFilter) (*GoalListResult, error) {
	if f.Status != "" {
		st, err := normalizeStatus(f.Status)
		if err != nil {
			return nil, err
		}
		f.Status = st
	}
	if f.Priority != "" {
		pr, err := normalizePriority(f.Priority)
		if err != nil {
			return nil, err
		}
		f.Priority = pr
	}

	items, total, err := s.Repo.List(ctx, userID, f)
	if err != nil {
		return nil, err
	}

	out := make([]GoalDTO, 0, len(items))
	for _, g := range items {
		out = append(out, toGoalDTO(g, nil))
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	limit := f.Limit
	if limit < 1 {
		limit = 20
	}
	return &GoalListResult{Items: out, Page: page, Limit: limit, Total: total}, nil
}

func (s *GoalService) Update(ctx context.Context, userID, id string, in GoalUpdateInput) (*GoalDTO, error) {
	existing, err := s.Repo.GetByID(ctx, id, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrGoalNotFound
	}
	if err != nil {
		return nil, err
	}

	params := repository.GoalUpdateParams{}

	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" || len(title) > 200 {
			return nil, ErrInvalidInput
		}
		params.Title = &title
	}
	if in.Description != nil {
		d := strings.TrimSpace(*in.Description)
		params.Description = &d
	}
	if in.Status != nil {
		st, err := normalizeStatus(*in.Status)
		if err != nil {
			return nil, err
		}
		params.Status = &st
		now := time.Now().UTC()
		switch st {
		case models.GoalStatusCompleted:
			if existing.CompletedAt == nil {
				t := &now
				params.CompletedAt = &t
			}
			p := 100
			params.Progress = &p
		case models.GoalStatusArchived:
			t := &now
			params.ArchivedAt = &t
		case models.GoalStatusActive, models.GoalStatusDraft:
			var nilTime *time.Time
			params.CompletedAt = &nilTime
			params.ArchivedAt = &nilTime
		}
	}
	if in.Priority != nil {
		pr, err := normalizePriority(*in.Priority)
		if err != nil {
			return nil, err
		}
		params.Priority = &pr
	}
	if in.Category != nil {
		c := strings.TrimSpace(*in.Category)
		params.Category = &c
	}
	if in.Unit != nil {
		u := strings.TrimSpace(*in.Unit)
		params.Unit = &u
	}
	if in.Progress != nil {
		p := clampProgress(*in.Progress)
		params.Progress = &p
	}
	if in.CurrentValue != nil {
		params.CurrentValue = in.CurrentValue
	}
	if in.ClearTarget {
		var nilVal *float64
		params.TargetValue = &nilVal
	} else if in.TargetValue != nil {
		if *in.TargetValue < 0 {
			return nil, ErrInvalidInput
		}
		v := in.TargetValue
		params.TargetValue = &v
	}
	if in.ClearStart {
		var nilTime *time.Time
		params.StartDate = &nilTime
	} else if in.StartDate != nil {
		t, err := parseDatePtr(in.StartDate)
		if err != nil {
			return nil, err
		}
		params.StartDate = &t
	}
	if in.ClearTargetD {
		var nilTime *time.Time
		params.TargetDate = &nilTime
	} else if in.TargetDate != nil {
		t, err := parseDatePtr(in.TargetDate)
		if err != nil {
			return nil, err
		}
		params.TargetDate = &t
	}

	goal, err := s.Repo.Update(ctx, id, userID, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGoalNotFound
		}
		return nil, err
	}

	// Recompute from milestones / target unless status was forced to completed/cancelled/archived
	if goal.Status == models.GoalStatusActive || goal.Status == models.GoalStatusDraft {
		goal, err = s.syncProgressAndStatus(ctx, goal)
		if err != nil {
			return nil, err
		}
	}

	milestones, err := s.Repo.ListMilestones(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	dto := toGoalDTO(*goal, milestones)
	return &dto, nil
}

func (s *GoalService) Delete(ctx context.Context, userID, id string) error {
	err := s.Repo.Delete(ctx, id, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrGoalNotFound
	}
	return err
}

func (s *GoalService) Stats(ctx context.Context, userID string) (*models.GoalStats, error) {
	return s.Repo.Stats(ctx, userID)
}

func (s *GoalService) Dashboard(ctx context.Context, userID string) (*models.GoalDashboard, error) {
	stats, err := s.Repo.Stats(ctx, userID)
	if err != nil {
		return nil, err
	}
	byStatus, err := s.Repo.CountsByStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	byPriority, err := s.Repo.CountsByPriority(ctx, userID)
	if err != nil {
		return nil, err
	}
	byCategory, err := s.Repo.CountsByCategory(ctx, userID, 6)
	if err != nil {
		return nil, err
	}
	trend, err := s.Repo.MonthlyTrend(ctx, userID, 6)
	if err != nil {
		return nil, err
	}
	buckets, err := s.Repo.ProgressBuckets(ctx, userID)
	if err != nil {
		return nil, err
	}

	allBuckets := []string{"0-20", "21-40", "41-60", "61-80", "81-100"}
	filled := make([]models.ProgressBucket, 0, len(allBuckets))
	lookup := make(map[string]int, len(buckets))
	for _, b := range buckets {
		lookup[b.Bucket] = b.Count
	}
	for _, key := range allBuckets {
		filled = append(filled, models.ProgressBucket{Bucket: key, Count: lookup[key]})
	}

	return &models.GoalDashboard{
		Stats:           *stats,
		ByStatus:        byStatus,
		ByPriority:      byPriority,
		ByCategory:      byCategory,
		CreatedTrend:    trend,
		ProgressBuckets: filled,
	}, nil
}

func (s *GoalService) ExportDashboardPDF(ctx context.Context, userID, locale string) ([]byte, string, error) {
	dashboard, err := s.Dashboard(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	return s.BuildDashboardPDF(dashboard, locale)
}

func (s *GoalService) AddMilestone(ctx context.Context, userID, goalID string, in MilestoneCreateInput) (*GoalDTO, error) {
	if _, err := s.Repo.GetByID(ctx, goalID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGoalNotFound
		}
		return nil, err
	}

	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > 200 {
		return nil, ErrInvalidInput
	}

	sortOrder := 0
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	} else {
		next, err := s.Repo.NextMilestoneSortOrder(ctx, goalID, userID)
		if err != nil {
			return nil, err
		}
		sortOrder = next
	}

	if _, err := s.Repo.CreateMilestone(ctx, goalID, userID, title, sortOrder); err != nil {
		return nil, err
	}

	goal, err := s.Repo.GetByID(ctx, goalID, userID)
	if err != nil {
		return nil, err
	}
	goal, err = s.syncProgressAndStatus(ctx, goal)
	if err != nil {
		return nil, err
	}
	milestones, err := s.Repo.ListMilestones(ctx, goalID, userID)
	if err != nil {
		return nil, err
	}
	dto := toGoalDTO(*goal, milestones)
	return &dto, nil
}

func (s *GoalService) UpdateMilestone(ctx context.Context, userID, goalID, milestoneID string, in MilestoneUpdateInput) (*GoalDTO, error) {
	if _, err := s.Repo.GetByID(ctx, goalID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGoalNotFound
		}
		return nil, err
	}

	var title *string
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" || len(t) > 200 {
			return nil, ErrInvalidInput
		}
		title = &t
	}

	if _, err := s.Repo.UpdateMilestone(ctx, milestoneID, goalID, userID, title, in.IsCompleted, in.SortOrder); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMilestoneNotFound
		}
		return nil, err
	}

	goal, err := s.Repo.GetByID(ctx, goalID, userID)
	if err != nil {
		return nil, err
	}
	goal, err = s.syncProgressAndStatus(ctx, goal)
	if err != nil {
		return nil, err
	}
	milestones, err := s.Repo.ListMilestones(ctx, goalID, userID)
	if err != nil {
		return nil, err
	}
	dto := toGoalDTO(*goal, milestones)
	return &dto, nil
}

func (s *GoalService) DeleteMilestone(ctx context.Context, userID, goalID, milestoneID string) (*GoalDTO, error) {
	if _, err := s.Repo.GetByID(ctx, goalID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGoalNotFound
		}
		return nil, err
	}

	if err := s.Repo.DeleteMilestone(ctx, milestoneID, goalID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMilestoneNotFound
		}
		return nil, err
	}

	goal, err := s.Repo.GetByID(ctx, goalID, userID)
	if err != nil {
		return nil, err
	}
	goal, err = s.syncProgressAndStatus(ctx, goal)
	if err != nil {
		return nil, err
	}
	milestones, err := s.Repo.ListMilestones(ctx, goalID, userID)
	if err != nil {
		return nil, err
	}
	dto := toGoalDTO(*goal, milestones)
	return &dto, nil
}
