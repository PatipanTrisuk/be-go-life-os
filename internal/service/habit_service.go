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

var ErrHabitNotFound = errors.New("habit not found")
var ErrHabitLogNotFound = errors.New("habit log not found")

type HabitDTO struct {
	ID             uuid.UUID     `json:"id"`
	Title          string        `json:"title"`
	Description    string        `json:"description"`
	Status         string        `json:"status"`
	Frequency      string        `json:"frequency"`
	TargetCount    int           `json:"target_count"`
	Category       string        `json:"category"`
	Color          string        `json:"color"`
	CurrentStreak  int           `json:"current_streak"`
	BestStreak     int           `json:"best_streak"`
	CompletedToday bool          `json:"completed_today"`
	TodayCount     int           `json:"today_count"`
	ArchivedAt     *string       `json:"archived_at,omitempty"`
	CreatedAt      string        `json:"created_at"`
	UpdatedAt      string        `json:"updated_at"`
	Logs           []HabitLogDTO `json:"logs,omitempty"`
}

type HabitLogDTO struct {
	ID        uuid.UUID `json:"id"`
	LogDate   string    `json:"log_date"`
	Count     int       `json:"count"`
	Note      string    `json:"note"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

type HabitCreateInput struct {
	Title       string `json:"title" binding:"required,min=1,max=200"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Frequency   string `json:"frequency"`
	TargetCount *int   `json:"target_count"`
	Category    string `json:"category"`
	Color       string `json:"color"`
}

type HabitUpdateInput struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
	Frequency   *string `json:"frequency"`
	TargetCount *int    `json:"target_count"`
	Category    *string `json:"category"`
	Color       *string `json:"color"`
}

type HabitCheckInInput struct {
	Date  *string `json:"date"`
	Count *int    `json:"count"`
	Note  *string `json:"note"`
	Done  *bool   `json:"done"`
}

type HabitListResult struct {
	Items []HabitDTO `json:"items"`
	Page  int        `json:"page"`
	Limit int        `json:"limit"`
	Total int        `json:"total"`
}

type HabitService struct {
	Repo         *repository.HabitRepository
	Entitlements *EntitlementService
}

func NewHabitService(repo *repository.HabitRepository, entitlements *EntitlementService) *HabitService {
	return &HabitService{Repo: repo, Entitlements: entitlements}
}

func toHabitDTO(h models.Habit, todayCount int) HabitDTO {
	return HabitDTO{
		ID:             h.ID,
		Title:          h.Title,
		Description:    h.Description,
		Status:         h.Status,
		Frequency:      h.Frequency,
		TargetCount:    h.TargetCount,
		Category:       h.Category,
		Color:          h.Color,
		CurrentStreak:  h.CurrentStreak,
		BestStreak:     h.BestStreak,
		CompletedToday: todayCount >= h.TargetCount,
		TodayCount:     todayCount,
		ArchivedAt:     formatTimePtr(h.ArchivedAt),
		CreatedAt:      h.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      h.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toHabitLogDTO(log models.HabitLog) HabitLogDTO {
	return HabitLogDTO{
		ID:        log.ID,
		LogDate:   log.LogDate.UTC().Format("2006-01-02"),
		Count:     log.Count,
		Note:      log.Note,
		CreatedAt: log.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: log.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func normalizeHabitStatus(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", models.HabitStatusActive:
		return models.HabitStatusActive, nil
	case models.HabitStatusPaused, models.HabitStatusArchived:
		return s, nil
	default:
		return "", ErrInvalidInput
	}
}

func normalizeHabitFrequency(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", models.HabitFrequencyDaily:
		return models.HabitFrequencyDaily, nil
	case models.HabitFrequencyWeekly:
		return s, nil
	default:
		return "", ErrInvalidInput
	}
}

func clampTargetCount(n int) int {
	if n < 1 {
		return 1
	}
	if n > 100 {
		return 100
	}
	return n
}

func truncateDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func computeDailyStreak(completed []time.Time, today time.Time) (current, best int) {
	today = truncateDate(today)
	set := make(map[string]struct{}, len(completed))
	dates := make([]time.Time, 0, len(completed))
	for _, d := range completed {
		td := truncateDate(d)
		key := td.Format("2006-01-02")
		if _, ok := set[key]; ok {
			continue
		}
		set[key] = struct{}{}
		dates = append(dates, td)
	}

	// best streak
	best = 0
	run := 0
	var prev *time.Time
	// dates are DESC from repo — reverse for ascending scan
	for i := len(dates) - 1; i >= 0; i-- {
		d := dates[i]
		if prev == nil || d.Equal(prev.AddDate(0, 0, 1)) {
			run++
		} else {
			run = 1
		}
		if run > best {
			best = run
		}
		prev = &d
	}

	// current streak: allow starting from today or yesterday
	start := today
	if _, ok := set[today.Format("2006-01-02")]; !ok {
		start = today.AddDate(0, 0, -1)
		if _, ok := set[start.Format("2006-01-02")]; !ok {
			return 0, best
		}
	}
	current = 0
	for d := start; ; d = d.AddDate(0, 0, -1) {
		if _, ok := set[d.Format("2006-01-02")]; !ok {
			break
		}
		current++
	}
	if current > best {
		best = current
	}
	return current, best
}

func (s *HabitService) refreshStreaks(ctx context.Context, habit *models.Habit) (*models.Habit, error) {
	dates, err := s.Repo.CompletedDates(ctx, habit.ID.String(), habit.UserID.String(), habit.TargetCount, 400)
	if err != nil {
		return nil, err
	}
	current, best := computeDailyStreak(dates, time.Now().UTC())
	if best < habit.BestStreak {
		best = habit.BestStreak
	}
	return s.Repo.UpdateStreaks(ctx, habit.ID.String(), habit.UserID.String(), current, best)
}

func (s *HabitService) Create(ctx context.Context, userID string, in HabitCreateInput) (*HabitDTO, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > 200 {
		return nil, ErrInvalidInput
	}
	status, err := normalizeHabitStatus(in.Status)
	if err != nil {
		return nil, err
	}
	freq, err := normalizeHabitFrequency(in.Frequency)
	if err != nil {
		return nil, err
	}
	target := 1
	if in.TargetCount != nil {
		target = clampTargetCount(*in.TargetCount)
	}

	if err := s.Entitlements.EnsureCanCreate(ctx, userID, models.FeatureLimitHabits); err != nil {
		return nil, err
	}
	h, err := s.Repo.Create(ctx, repository.HabitCreateParams{
		UserID:      userID,
		Title:       title,
		Description: strings.TrimSpace(in.Description),
		Status:      status,
		Frequency:   freq,
		TargetCount: target,
		Category:    strings.TrimSpace(in.Category),
		Color:       strings.TrimSpace(in.Color),
	})
	if err != nil {
		return nil, err
	}
	dto := toHabitDTO(*h, 0)
	return &dto, nil
}

func (s *HabitService) Get(ctx context.Context, userID, id string) (*HabitDTO, error) {
	h, err := s.Repo.GetByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrHabitNotFound
		}
		return nil, err
	}
	todayCount := 0
	if log, err := s.Repo.GetLogByDate(ctx, id, userID, time.Now().UTC()); err == nil {
		todayCount = log.Count
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -41)
	logs, err := s.Repo.ListLogs(ctx, id, userID, from, to)
	if err != nil {
		return nil, err
	}
	dto := toHabitDTO(*h, todayCount)
	dto.Logs = make([]HabitLogDTO, 0, len(logs))
	for _, log := range logs {
		dto.Logs = append(dto.Logs, toHabitLogDTO(log))
	}
	return &dto, nil
}

func (s *HabitService) List(ctx context.Context, userID string, f repository.HabitListFilter) (*HabitListResult, error) {
	items, total, err := s.Repo.List(ctx, userID, f)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(items))
	for _, h := range items {
		ids = append(ids, h.ID.String())
	}
	todayCounts, err := s.Repo.TodayCounts(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]HabitDTO, 0, len(items))
	for _, h := range items {
		out = append(out, toHabitDTO(h, todayCounts[h.ID.String()]))
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	limit := f.Limit
	if limit < 1 {
		limit = 20
	}
	return &HabitListResult{Items: out, Page: page, Limit: limit, Total: total}, nil
}

func (s *HabitService) Update(ctx context.Context, userID, id string, in HabitUpdateInput) (*HabitDTO, error) {
	existing, err := s.Repo.GetByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrHabitNotFound
		}
		return nil, err
	}

	params := repository.HabitUpdateParams{}
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
		status, err := normalizeHabitStatus(*in.Status)
		if err != nil {
			return nil, err
		}
		params.Status = &status
		if status == models.HabitStatusArchived {
			now := time.Now().UTC()
			archivedAt := &now
			params.ArchivedAt = &archivedAt
		} else if existing.Status == models.HabitStatusArchived {
			var nilTime *time.Time
			params.ArchivedAt = &nilTime
		}
	}
	if in.Frequency != nil {
		freq, err := normalizeHabitFrequency(*in.Frequency)
		if err != nil {
			return nil, err
		}
		params.Frequency = &freq
	}
	if in.TargetCount != nil {
		n := clampTargetCount(*in.TargetCount)
		params.TargetCount = &n
	}
	if in.Category != nil {
		c := strings.TrimSpace(*in.Category)
		params.Category = &c
	}
	if in.Color != nil {
		c := strings.TrimSpace(*in.Color)
		params.Color = &c
	}

	h, err := s.Repo.Update(ctx, id, userID, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrHabitNotFound
		}
		return nil, err
	}

	if in.TargetCount != nil {
		h, err = s.refreshStreaks(ctx, h)
		if err != nil {
			return nil, err
		}
	}

	todayCount := 0
	if log, err := s.Repo.GetLogByDate(ctx, id, userID, time.Now().UTC()); err == nil {
		todayCount = log.Count
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	dto := toHabitDTO(*h, todayCount)
	return &dto, nil
}

func (s *HabitService) Delete(ctx context.Context, userID, id string) error {
	if err := s.Repo.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrHabitNotFound
		}
		return err
	}
	return nil
}

func (s *HabitService) Stats(ctx context.Context, userID string) (*models.HabitStats, error) {
	return s.Repo.Stats(ctx, userID)
}

func (s *HabitService) Dashboard(ctx context.Context, userID string) (*models.HabitDashboard, error) {
	stats, err := s.Repo.Stats(ctx, userID)
	if err != nil {
		return nil, err
	}
	byStatus, err := s.Repo.CountsByStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	byFrequency, err := s.Repo.CountsByFrequency(ctx, userID)
	if err != nil {
		return nil, err
	}
	byCategory, err := s.Repo.CountsByCategory(ctx, userID, 6)
	if err != nil {
		return nil, err
	}
	trend, err := s.Repo.CheckinTrend(ctx, userID, 14)
	if err != nil {
		return nil, err
	}
	buckets, err := s.Repo.StreakBuckets(ctx, userID)
	if err != nil {
		return nil, err
	}

	allBuckets := []string{"0", "1-3", "4-7", "8-14", "15+"}
	filled := make([]models.ProgressBucket, 0, len(allBuckets))
	lookup := make(map[string]int, len(buckets))
	for _, b := range buckets {
		lookup[b.Bucket] = b.Count
	}
	for _, key := range allBuckets {
		filled = append(filled, models.ProgressBucket{Bucket: key, Count: lookup[key]})
	}

	return &models.HabitDashboard{
		Stats:         *stats,
		ByStatus:      byStatus,
		ByFrequency:   byFrequency,
		ByCategory:    byCategory,
		CheckinTrend:  trend,
		StreakBuckets: filled,
	}, nil
}

func (s *HabitService) CheckIn(ctx context.Context, userID, id string, in HabitCheckInInput) (*HabitDTO, error) {
	h, err := s.Repo.GetByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrHabitNotFound
		}
		return nil, err
	}

	day := time.Now().UTC()
	if in.Date != nil {
		parsed, err := parseDatePtr(in.Date)
		if err != nil || parsed == nil {
			return nil, ErrInvalidInput
		}
		day = *parsed
	}
	day = truncateDate(day)

	done := true
	if in.Done != nil {
		done = *in.Done
	}

	note := ""
	if in.Note != nil {
		note = strings.TrimSpace(*in.Note)
	}

	todayCount := 0
	if !done {
		if err := s.Repo.DeleteLog(ctx, id, userID, day); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	} else {
		count := h.TargetCount
		if in.Count != nil {
			count = clampTargetCount(*in.Count)
		} else if existing, err := s.Repo.GetLogByDate(ctx, id, userID, day); err == nil {
			// toggle-friendly: if already complete, leave as-is unless count provided
			count = existing.Count
			if existing.Note != "" && note == "" {
				note = existing.Note
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		log, err := s.Repo.UpsertLog(ctx, id, userID, day, count, note)
		if err != nil {
			return nil, err
		}
		todayCount = log.Count
		if !truncateDate(time.Now().UTC()).Equal(day) {
			todayCount = 0
			if todayLog, err := s.Repo.GetLogByDate(ctx, id, userID, time.Now().UTC()); err == nil {
				todayCount = todayLog.Count
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
		}
	}

	h, err = s.refreshStreaks(ctx, h)
	if err != nil {
		return nil, err
	}

	// refresh today count after uncheck
	if !done {
		if todayLog, err := s.Repo.GetLogByDate(ctx, id, userID, time.Now().UTC()); err == nil {
			todayCount = todayLog.Count
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		} else {
			todayCount = 0
		}
	}

	dto := toHabitDTO(*h, todayCount)
	return &dto, nil
}

func (s *HabitService) ListLogs(ctx context.Context, userID, id, fromRaw, toRaw string) ([]HabitLogDTO, error) {
	if _, err := s.Repo.GetByID(ctx, id, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrHabitNotFound
		}
		return nil, err
	}

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -41)
	if strings.TrimSpace(fromRaw) != "" {
		parsed, err := parseDatePtr(&fromRaw)
		if err != nil || parsed == nil {
			return nil, ErrInvalidInput
		}
		from = *parsed
	}
	if strings.TrimSpace(toRaw) != "" {
		parsed, err := parseDatePtr(&toRaw)
		if err != nil || parsed == nil {
			return nil, ErrInvalidInput
		}
		to = *parsed
	}
	if to.Before(from) {
		return nil, ErrInvalidInput
	}

	logs, err := s.Repo.ListLogs(ctx, id, userID, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]HabitLogDTO, 0, len(logs))
	for _, log := range logs {
		out = append(out, toHabitLogDTO(log))
	}
	return out, nil
}
