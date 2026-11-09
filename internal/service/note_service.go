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

var ErrNoteNotFound = errors.New("note not found")

type NoteDTO struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Kind      string    `json:"kind"`
	Mood      string    `json:"mood"`
	EntryDate string    `json:"entry_date"`
	Tags      string    `json:"tags"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

type NoteCreateInput struct {
	Title     string `json:"title" binding:"required,min=1,max=200"`
	Body      string `json:"body"`
	Kind      string `json:"kind"`
	Mood      string `json:"mood"`
	EntryDate string `json:"entry_date"`
	Tags      string `json:"tags"`
}

type NoteUpdateInput struct {
	Title     *string `json:"title"`
	Body      *string `json:"body"`
	Kind      *string `json:"kind"`
	Mood      *string `json:"mood"`
	EntryDate *string `json:"entry_date"`
	Tags      *string `json:"tags"`
}

type NoteListResult struct {
	Items []NoteDTO `json:"items"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
	Total int       `json:"total"`
}

type NoteService struct {
	Repo         *repository.NoteRepository
	Entitlements *EntitlementService
}

func NewNoteService(repo *repository.NoteRepository, entitlements *EntitlementService) *NoteService {
	return &NoteService{Repo: repo, Entitlements: entitlements}
}

func toNoteDTO(n models.Note) NoteDTO {
	return NoteDTO{
		ID:        n.ID,
		Title:     n.Title,
		Body:      n.Body,
		Kind:      n.Kind,
		Mood:      n.Mood,
		EntryDate: n.EntryDate.UTC().Format("2006-01-02"),
		Tags:      n.Tags,
		CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: n.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func normalizeNoteKind(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", models.NoteKindNote:
		return models.NoteKindNote, nil
	case models.NoteKindJournal:
		return s, nil
	default:
		return "", ErrInvalidInput
	}
}

func (s *NoteService) List(ctx context.Context, userID string, f repository.NoteListFilter) (*NoteListResult, error) {
	if f.Kind != "" {
		k, err := normalizeNoteKind(f.Kind)
		if err != nil {
			return nil, err
		}
		f.Kind = k
	}
	f.Search = strings.TrimSpace(f.Search)

	items, total, err := s.Repo.List(ctx, userID, f)
	if err != nil {
		return nil, err
	}
	out := make([]NoteDTO, 0, len(items))
	for _, n := range items {
		out = append(out, toNoteDTO(n))
	}
	page := f.Page
	if page < 1 {
		page = 1
	}
	limit := f.Limit
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return &NoteListResult{Items: out, Page: page, Limit: limit, Total: total}, nil
}

func (s *NoteService) Stats(ctx context.Context, userID string) (*models.NoteStats, error) {
	return s.Repo.Stats(ctx, userID)
}

func (s *NoteService) Get(ctx context.Context, userID, id string) (*NoteDTO, error) {
	n, err := s.Repo.GetByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoteNotFound
		}
		return nil, err
	}
	dto := toNoteDTO(*n)
	return &dto, nil
}

func (s *NoteService) Create(ctx context.Context, userID string, in NoteCreateInput) (*NoteDTO, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > 200 {
		return nil, ErrInvalidInput
	}
	kind, err := normalizeNoteKind(in.Kind)
	if err != nil {
		return nil, err
	}
	mood, err := validateShortText(in.Mood, 50)
	if err != nil {
		return nil, err
	}
	entryDate := truncateDate(time.Now().UTC())
	if strings.TrimSpace(in.EntryDate) != "" {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(in.EntryDate))
		if err != nil {
			return nil, ErrInvalidInput
		}
		entryDate = truncateDate(t)
	}

	if err := s.Entitlements.EnsureCanCreate(ctx, userID, models.FeatureLimitNotes); err != nil {
		return nil, err
	}
	n, err := s.Repo.Create(ctx, repository.NoteCreateParams{
		UserID:    userID,
		Title:     title,
		Body:      strings.TrimSpace(in.Body),
		Kind:      kind,
		Mood:      mood,
		EntryDate: entryDate,
		Tags:      strings.TrimSpace(in.Tags),
	})
	if err != nil {
		return nil, err
	}
	dto := toNoteDTO(*n)
	return &dto, nil
}

func (s *NoteService) Update(ctx context.Context, userID, id string, in NoteUpdateInput) (*NoteDTO, error) {
	params := repository.NoteUpdateParams{}
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
	if in.Kind != nil {
		k, err := normalizeNoteKind(*in.Kind)
		if err != nil {
			return nil, err
		}
		params.Kind = &k
	}
	if in.Mood != nil {
		m, err := validateShortText(*in.Mood, 50)
		if err != nil {
			return nil, err
		}
		params.Mood = &m
	}
	if in.EntryDate != nil {
		t, err := parseDatePtr(in.EntryDate)
		if err != nil {
			return nil, err
		}
		params.EntryDate = t
	}
	if in.Tags != nil {
		tg := strings.TrimSpace(*in.Tags)
		params.Tags = &tg
	}

	n, err := s.Repo.Update(ctx, id, userID, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoteNotFound
		}
		return nil, err
	}
	dto := toNoteDTO(*n)
	return &dto, nil
}

func (s *NoteService) Delete(ctx context.Context, userID, id string) error {
	if err := s.Repo.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoteNotFound
		}
		return err
	}
	return nil
}
