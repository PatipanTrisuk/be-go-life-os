package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"go-be/internal/models"
	"go-be/internal/repository"
)

var ErrFinanceNotFound = errors.New("finance transaction not found")

type FinanceTransactionDTO struct {
	ID         uuid.UUID `json:"id"`
	Kind       string    `json:"kind"`
	Amount     float64   `json:"amount"`
	Currency   string    `json:"currency"`
	Category   string    `json:"category"`
	Note       string    `json:"note"`
	OccurredOn string    `json:"occurred_on"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

type FinanceCreateInput struct {
	Kind       string  `json:"kind" binding:"required"`
	Amount     float64 `json:"amount" binding:"required"`
	Currency   string  `json:"currency"`
	Category   string  `json:"category"`
	Note       string  `json:"note"`
	OccurredOn string  `json:"occurred_on"`
}

type FinanceUpdateInput struct {
	Kind       *string  `json:"kind"`
	Amount     *float64 `json:"amount"`
	Currency   *string  `json:"currency"`
	Category   *string  `json:"category"`
	Note       *string  `json:"note"`
	OccurredOn *string  `json:"occurred_on"`
}

type FinanceListResult struct {
	Items []FinanceTransactionDTO `json:"items"`
	Page  int                     `json:"page"`
	Limit int                     `json:"limit"`
	Total int                     `json:"total"`
}

type FinanceListQuery struct {
	Kind     string
	Category string
	From     string
	To       string
	Page     int
	Limit    int
}

type FinanceService struct {
	Repo         *repository.FinanceRepository
	Entitlements *EntitlementService
}

func NewFinanceService(repo *repository.FinanceRepository, entitlements *EntitlementService) *FinanceService {
	return &FinanceService{Repo: repo, Entitlements: entitlements}
}

func toFinanceDTO(t models.FinanceTransaction) FinanceTransactionDTO {
	return FinanceTransactionDTO{
		ID:         t.ID,
		Kind:       t.Kind,
		Amount:     t.Amount,
		Currency:   t.Currency,
		Category:   t.Category,
		Note:       t.Note,
		OccurredOn: t.OccurredOn.UTC().Format("2006-01-02"),
		CreatedAt:  t.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  t.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func normalizeFinanceKind(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case models.FinanceKindIncome, models.FinanceKindExpense:
		return s, nil
	default:
		return "", ErrInvalidInput
	}
}

func normalizeCurrency(s string) (string, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return "THB", nil
	}
	if len(s) > 10 {
		return "", ErrInvalidInput
	}
	return s, nil
}

func validateAmount(a float64) error {
	if math.IsNaN(a) || math.IsInf(a, 0) || a <= 0 {
		return ErrInvalidInput
	}
	return nil
}

// parseOptionalDate parses YYYY-MM-DD; empty string yields nil.
func parseOptionalDate(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, ErrInvalidInput
	}
	t = truncateDate(t)
	return &t, nil
}

func financeRange(fromRaw, toRaw string) (*time.Time, *time.Time, error) {
	from, err := parseOptionalDate(fromRaw)
	if err != nil {
		return nil, nil, err
	}
	to, err := parseOptionalDate(toRaw)
	if err != nil {
		return nil, nil, err
	}
	if from != nil && to != nil && to.Before(*from) {
		return nil, nil, ErrInvalidInput
	}
	return from, to, nil
}

func (s *FinanceService) List(ctx context.Context, userID string, q FinanceListQuery) (*FinanceListResult, error) {
	f := repository.FinanceListFilter{
		Category: strings.TrimSpace(q.Category),
		Page:     q.Page,
		Limit:    q.Limit,
	}
	if strings.TrimSpace(q.Kind) != "" {
		k, err := normalizeFinanceKind(q.Kind)
		if err != nil {
			return nil, err
		}
		f.Kind = k
	}
	from, to, err := financeRange(q.From, q.To)
	if err != nil {
		return nil, err
	}
	f.From, f.To = from, to

	items, total, err := s.Repo.List(ctx, userID, f)
	if err != nil {
		return nil, err
	}
	out := make([]FinanceTransactionDTO, 0, len(items))
	for _, t := range items {
		out = append(out, toFinanceDTO(t))
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
	return &FinanceListResult{Items: out, Page: page, Limit: limit, Total: total}, nil
}

func (s *FinanceService) Stats(ctx context.Context, userID, fromRaw, toRaw string) (*models.FinanceStats, error) {
	from, to, err := financeRange(fromRaw, toRaw)
	if err != nil {
		return nil, err
	}
	return s.Repo.Stats(ctx, userID, from, to)
}

func (s *FinanceService) Get(ctx context.Context, userID, id string) (*FinanceTransactionDTO, error) {
	t, err := s.Repo.GetByID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFinanceNotFound
		}
		return nil, err
	}
	dto := toFinanceDTO(*t)
	return &dto, nil
}

func (s *FinanceService) Create(ctx context.Context, userID string, in FinanceCreateInput) (*FinanceTransactionDTO, error) {
	kind, err := normalizeFinanceKind(in.Kind)
	if err != nil {
		return nil, err
	}
	if err := validateAmount(in.Amount); err != nil {
		return nil, err
	}
	currency, err := normalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	category, err := validateShortText(in.Category, 100)
	if err != nil {
		return nil, err
	}
	occurredOn := truncateDate(time.Now().UTC())
	if d, err := parseOptionalDate(in.OccurredOn); err != nil {
		return nil, err
	} else if d != nil {
		occurredOn = *d
	}

	if err := s.Entitlements.EnsureCanCreate(ctx, userID, models.FeatureLimitFinance); err != nil {
		return nil, err
	}
	t, err := s.Repo.Create(ctx, repository.FinanceCreateParams{
		UserID:     userID,
		Kind:       kind,
		Amount:     in.Amount,
		Currency:   currency,
		Category:   category,
		Note:       strings.TrimSpace(in.Note),
		OccurredOn: occurredOn,
	})
	if err != nil {
		return nil, err
	}
	dto := toFinanceDTO(*t)
	return &dto, nil
}

func (s *FinanceService) Update(ctx context.Context, userID, id string, in FinanceUpdateInput) (*FinanceTransactionDTO, error) {
	params := repository.FinanceUpdateParams{}
	if in.Kind != nil {
		k, err := normalizeFinanceKind(*in.Kind)
		if err != nil {
			return nil, err
		}
		params.Kind = &k
	}
	if in.Amount != nil {
		if err := validateAmount(*in.Amount); err != nil {
			return nil, err
		}
		params.Amount = in.Amount
	}
	if in.Currency != nil {
		c, err := normalizeCurrency(*in.Currency)
		if err != nil {
			return nil, err
		}
		params.Currency = &c
	}
	if in.Category != nil {
		c, err := validateShortText(*in.Category, 100)
		if err != nil {
			return nil, err
		}
		params.Category = &c
	}
	if in.Note != nil {
		n := strings.TrimSpace(*in.Note)
		params.Note = &n
	}
	if in.OccurredOn != nil {
		d, err := parseDatePtr(in.OccurredOn)
		if err != nil {
			return nil, err
		}
		params.OccurredOn = d
	}

	t, err := s.Repo.Update(ctx, id, userID, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFinanceNotFound
		}
		return nil, err
	}
	dto := toFinanceDTO(*t)
	return &dto, nil
}

func (s *FinanceService) Delete(ctx context.Context, userID, id string) error {
	if err := s.Repo.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFinanceNotFound
		}
		return err
	}
	return nil
}
