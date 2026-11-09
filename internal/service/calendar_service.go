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

var ErrCalendarNotFound = errors.New("calendar event not found")

type CalendarEventDTO struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	Color       string    `json:"color"`
	AllDay      bool      `json:"all_day"`
	StartsAt    string    `json:"starts_at"`
	EndsAt      string    `json:"ends_at"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

type CalendarCreateInput struct {
	Title       string `json:"title" binding:"required,min=1,max=200"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Color       string `json:"color"`
	AllDay      bool   `json:"all_day"`
	StartsAt    string `json:"starts_at"`
	EndsAt      string `json:"ends_at"`
}

type CalendarUpdateInput struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Location    *string `json:"location"`
	Color       *string `json:"color"`
	AllDay      *bool   `json:"all_day"`
	StartsAt    *string `json:"starts_at"`
	EndsAt      *string `json:"ends_at"`
}

type CalendarService struct {
	Repo         *repository.CalendarRepository
	Entitlements *EntitlementService
}

func NewCalendarService(repo *repository.CalendarRepository, entitlements *EntitlementService) *CalendarService {
	return &CalendarService{Repo: repo, Entitlements: entitlements}
}

func toCalendarDTO(e models.CalendarEvent) CalendarEventDTO {
	return CalendarEventDTO{
		ID:          e.ID,
		Title:       e.Title,
		Description: e.Description,
		Location:    e.Location,
		Color:       e.Color,
		AllDay:      e.AllDay,
		StartsAt:    e.StartsAt.UTC().Format(time.RFC3339),
		EndsAt:      e.EndsAt.UTC().Format(time.RFC3339),
		CreatedAt:   e.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   e.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// parseDateTimeInput parses RFC3339 or YYYY-MM-DD. For date-only input,
// endOfDay selects 23:59:59 instead of 00:00:00 (UTC).
func parseDateTimeInput(raw string, endOfDay bool) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, ErrInvalidInput
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, ErrInvalidInput
	}
	d = d.UTC()
	if endOfDay {
		return d.Add(24*time.Hour - time.Second), nil
	}
	return d, nil
}

func calendarRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	fromRaw = strings.TrimSpace(fromRaw)
	toRaw = strings.TrimSpace(toRaw)

	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0).Add(-time.Second)

	from, to := monthStart, monthEnd
	var err error
	if fromRaw != "" {
		from, err = parseDateTimeInput(fromRaw, false)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if toRaw == "" {
			to = from.AddDate(0, 1, 0).Add(-time.Second)
		}
	}
	if toRaw != "" {
		to, err = parseDateTimeInput(toRaw, true)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if fromRaw == "" {
			from = time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, time.UTC)
		}
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, ErrInvalidInput
	}
	return from, to, nil
}

func validateShortText(s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return "", ErrInvalidInput
	}
	return s, nil
}

func (s *CalendarService) List(ctx context.Context, userID, fromRaw, toRaw string) ([]CalendarEventDTO, error) {
	from, to, err := calendarRange(fromRaw, toRaw)
	if err != nil {
		return nil, err
	}
	items, err := s.Repo.ListRange(ctx, userID, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]CalendarEventDTO, 0, len(items))
	for _, item := range items {
		out = append(out, toCalendarDTO(item))
	}
	return out, nil
}

func (s *CalendarService) Stats(ctx context.Context, userID, fromRaw, toRaw string) (*models.CalendarStats, error) {
	from, to, err := calendarRange(fromRaw, toRaw)
	if err != nil {
		return nil, err
	}
	return s.Repo.Stats(ctx, userID, from, to, time.Now().UTC())
}

func (s *CalendarService) Get(ctx context.Context, userID, id string) (*CalendarEventDTO, error) {
	e, err := s.Repo.GetByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}
	dto := toCalendarDTO(*e)
	return &dto, nil
}

func (s *CalendarService) Create(ctx context.Context, userID string, in CalendarCreateInput) (*CalendarEventDTO, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > 200 {
		return nil, ErrInvalidInput
	}
	location, err := validateShortText(in.Location, 200)
	if err != nil {
		return nil, err
	}
	color, err := validateShortText(in.Color, 30)
	if err != nil {
		return nil, err
	}
	startsAt, err := parseDateTimeInput(in.StartsAt, false)
	if err != nil {
		return nil, err
	}
	endsAt := startsAt
	if strings.TrimSpace(in.EndsAt) != "" {
		endsAt, err = parseDateTimeInput(in.EndsAt, true)
		if err != nil {
			return nil, err
		}
	}
	if endsAt.Before(startsAt) {
		return nil, ErrInvalidInput
	}

	if err := s.Entitlements.EnsureCanCreate(ctx, userID, models.FeatureLimitCalendar); err != nil {
		return nil, err
	}
	e, err := s.Repo.Create(ctx, repository.CalendarCreateParams{
		UserID:      userID,
		Title:       title,
		Description: strings.TrimSpace(in.Description),
		Location:    location,
		Color:       color,
		AllDay:      in.AllDay,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
	})
	if err != nil {
		return nil, err
	}
	dto := toCalendarDTO(*e)
	return &dto, nil
}

func (s *CalendarService) Update(ctx context.Context, userID, id string, in CalendarUpdateInput) (*CalendarEventDTO, error) {
	existing, err := s.Repo.GetByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}

	params := repository.CalendarUpdateParams{}
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
	if in.Location != nil {
		v, err := validateShortText(*in.Location, 200)
		if err != nil {
			return nil, err
		}
		params.Location = &v
	}
	if in.Color != nil {
		v, err := validateShortText(*in.Color, 30)
		if err != nil {
			return nil, err
		}
		params.Color = &v
	}
	if in.AllDay != nil {
		params.AllDay = in.AllDay
	}

	startsAt, endsAt := existing.StartsAt.UTC(), existing.EndsAt.UTC()
	if in.StartsAt != nil {
		t, err := parseDateTimeInput(*in.StartsAt, false)
		if err != nil {
			return nil, err
		}
		startsAt = t
		params.StartsAt = &t
	}
	if in.EndsAt != nil {
		t, err := parseDateTimeInput(*in.EndsAt, true)
		if err != nil {
			return nil, err
		}
		endsAt = t
		params.EndsAt = &t
	}
	if endsAt.Before(startsAt) {
		return nil, ErrInvalidInput
	}

	e, err := s.Repo.Update(ctx, id, userID, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}
	dto := toCalendarDTO(*e)
	return &dto, nil
}

func (s *CalendarService) Delete(ctx context.Context, userID, id string) error {
	if err := s.Repo.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCalendarNotFound
		}
		return err
	}
	return nil
}
