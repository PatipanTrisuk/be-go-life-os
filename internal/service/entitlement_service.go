package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go-be/internal/models"
	"go-be/internal/repository"
)

var (
	ErrPlanLimitReached = errors.New("plan limit reached")
	ErrInvalidPlan      = errors.New("invalid plan")
)

// PlanLimitError is returned when a create would exceed the user's plan entitlement.
type PlanLimitError struct {
	Feature string
	Limit   int
	Usage   int
}

func (e *PlanLimitError) Error() string { return ErrPlanLimitReached.Error() }
func (e *PlanLimitError) Unwrap() error { return ErrPlanLimitReached }

type EntitlementDTO struct {
	FeatureKey string `json:"feature_key"`
	LimitValue int    `json:"limit_value"`
}

type PlanDTO struct {
	Code         string           `json:"code"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	IsActive     bool             `json:"is_active"`
	Entitlements []EntitlementDTO `json:"entitlements"`
}

type SubscriptionSnapshot struct {
	PlanCode     string             `json:"plan_code"`
	PlanName     string             `json:"plan_name"`
	Status       string             `json:"status"`
	StartedAt    string             `json:"started_at"`
	Entitlements []EntitlementDTO   `json:"entitlements"`
	Usage        []models.UsageItem `json:"usage"`
}

type EntitlementService struct {
	BillingRepo *repository.BillingRepository
}

func NewEntitlementService(billingRepo *repository.BillingRepository) *EntitlementService {
	return &EntitlementService{BillingRepo: billingRepo}
}

// billingFeatureKeys is the ordered list of features reported in usage snapshots.
var billingFeatureKeys = []string{
	models.FeatureLimitGoals,
	models.FeatureLimitHabits,
	models.FeatureLimitPlannerPerDay,
	models.FeatureLimitCalendar,
	models.FeatureLimitNotes,
	models.FeatureLimitFinance,
	models.FeatureLimitReminders,
}

func isValidPlanCode(code string) bool {
	return code == models.PlanCodeFree || code == models.PlanCodePro
}

func toEntitlementDTOs(items []models.PlanEntitlement) []EntitlementDTO {
	out := make([]EntitlementDTO, 0, len(items))
	for _, e := range items {
		out = append(out, EntitlementDTO{FeatureKey: e.FeatureKey, LimitValue: e.LimitValue})
	}
	return out
}

// GetPlans returns all plans with their entitlements.
func (s *EntitlementService) GetPlans(ctx context.Context) ([]PlanDTO, error) {
	plans, err := s.BillingRepo.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	ents, err := s.BillingRepo.ListAllEntitlements(ctx)
	if err != nil {
		return nil, err
	}
	byPlan := make(map[string][]EntitlementDTO, len(plans))
	for _, e := range ents {
		byPlan[e.PlanCode] = append(byPlan[e.PlanCode], EntitlementDTO{FeatureKey: e.FeatureKey, LimitValue: e.LimitValue})
	}
	out := make([]PlanDTO, 0, len(plans))
	for _, p := range plans {
		list := byPlan[p.Code]
		if list == nil {
			list = []EntitlementDTO{}
		}
		out = append(out, PlanDTO{
			Code:         p.Code,
			Name:         p.Name,
			Description:  p.Description,
			IsActive:     p.IsActive,
			Entitlements: list,
		})
	}
	return out, nil
}

// EnsureSubscription creates a free subscription if the user has none.
func (s *EntitlementService) EnsureSubscription(ctx context.Context, userID string) error {
	return s.BillingRepo.EnsureFreeSubscription(ctx, userID)
}

// loadSubscription returns the subscription, auto-creating a free one if missing.
func (s *EntitlementService) loadSubscription(ctx context.Context, userID string) (*models.Subscription, error) {
	sub, err := s.BillingRepo.GetSubscription(ctx, userID)
	if err == nil {
		return sub, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err := s.BillingRepo.EnsureFreeSubscription(ctx, userID); err != nil {
		return nil, err
	}
	return s.BillingRepo.GetSubscription(ctx, userID)
}

func (s *EntitlementService) planName(ctx context.Context, planCode string) (string, error) {
	plans, err := s.BillingRepo.ListPlans(ctx)
	if err != nil {
		return "", err
	}
	for _, p := range plans {
		if p.Code == planCode {
			return p.Name, nil
		}
	}
	return planCode, nil
}

func (s *EntitlementService) usageForFeature(ctx context.Context, userID, featureKey string) (int, error) {
	switch featureKey {
	case models.FeatureLimitGoals:
		return s.BillingRepo.CountGoals(ctx, userID)
	case models.FeatureLimitHabits:
		return s.BillingRepo.CountHabits(ctx, userID)
	case models.FeatureLimitPlannerPerDay:
		return s.BillingRepo.CountPlannerForDate(ctx, userID, time.Now().UTC())
	case models.FeatureLimitCalendar:
		return s.BillingRepo.CountCalendar(ctx, userID)
	case models.FeatureLimitNotes:
		return s.BillingRepo.CountNotes(ctx, userID)
	case models.FeatureLimitFinance:
		return s.BillingRepo.CountFinance(ctx, userID)
	case models.FeatureLimitReminders:
		return s.BillingRepo.CountReminders(ctx, userID)
	default:
		return 0, ErrInvalidInput
	}
}

// GetSnapshot returns the user's subscription, plan entitlements and current usage.
func (s *EntitlementService) GetSnapshot(ctx context.Context, userID string) (*SubscriptionSnapshot, error) {
	sub, err := s.loadSubscription(ctx, userID)
	if err != nil {
		return nil, err
	}
	name, err := s.planName(ctx, sub.PlanCode)
	if err != nil {
		return nil, err
	}
	ents, err := s.BillingRepo.ListEntitlements(ctx, sub.PlanCode)
	if err != nil {
		return nil, err
	}
	limits := make(map[string]int, len(ents))
	for _, e := range ents {
		limits[e.FeatureKey] = e.LimitValue
	}

	usage := make([]models.UsageItem, 0, len(billingFeatureKeys))
	for _, key := range billingFeatureKeys {
		n, err := s.usageForFeature(ctx, userID, key)
		if err != nil {
			return nil, err
		}
		limit, ok := limits[key]
		if !ok {
			limit = -1
		}
		usage = append(usage, models.UsageItem{Feature: key, Usage: n, Limit: limit})
	}

	return &SubscriptionSnapshot{
		PlanCode:     sub.PlanCode,
		PlanName:     name,
		Status:       sub.Status,
		StartedAt:    sub.StartedAt.UTC().Format(time.RFC3339),
		Entitlements: toEntitlementDTOs(ents),
		Usage:        usage,
	}, nil
}

// EnsureWithinLimit returns *PlanLimitError when usage has reached the plan limit (limit < 0 = unlimited).
func (s *EntitlementService) EnsureWithinLimit(ctx context.Context, userID, featureKey string, usage int) error {
	sub, err := s.loadSubscription(ctx, userID)
	if err != nil {
		return err
	}
	ents, err := s.BillingRepo.ListEntitlements(ctx, sub.PlanCode)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.FeatureKey != featureKey {
			continue
		}
		if e.LimitValue < 0 {
			return nil
		}
		if usage >= e.LimitValue {
			return &PlanLimitError{Feature: featureKey, Limit: e.LimitValue, Usage: usage}
		}
		return nil
	}
	// No entitlement row for this feature: treat as unlimited.
	return nil
}

// EnsureCanCreate counts current usage for the feature and checks the plan limit.
func (s *EntitlementService) EnsureCanCreate(ctx context.Context, userID, featureKey string) error {
	usage, err := s.usageForFeature(ctx, userID, featureKey)
	if err != nil {
		return err
	}
	return s.EnsureWithinLimit(ctx, userID, featureKey, usage)
}

// EnsureCanCreatePlanner checks the per-day planner limit for the given plan date.
func (s *EntitlementService) EnsureCanCreatePlanner(ctx context.Context, userID string, planDate time.Time) error {
	usage, err := s.BillingRepo.CountPlannerForDate(ctx, userID, planDate)
	if err != nil {
		return err
	}
	return s.EnsureWithinLimit(ctx, userID, models.FeatureLimitPlannerPerDay, usage)
}

func (s *EntitlementService) setPlan(ctx context.Context, userID, planCode string) (*SubscriptionSnapshot, error) {
	if !isValidPlanCode(planCode) {
		return nil, ErrInvalidPlan
	}
	if _, err := uuid.Parse(userID); err != nil {
		return nil, ErrUserNotFound
	}
	if _, err := s.BillingRepo.UpsertSubscription(ctx, userID, planCode); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return s.GetSnapshot(ctx, userID)
}

// Upgrade sets the user's plan to pro.
func (s *EntitlementService) Upgrade(ctx context.Context, userID string) (*SubscriptionSnapshot, error) {
	return s.setPlan(ctx, userID, models.PlanCodePro)
}

// Downgrade sets the user's plan to free.
func (s *EntitlementService) Downgrade(ctx context.Context, userID string) (*SubscriptionSnapshot, error) {
	return s.setPlan(ctx, userID, models.PlanCodeFree)
}

// AdminSetPlan sets any valid plan (free|pro) for a user.
func (s *EntitlementService) AdminSetPlan(ctx context.Context, userID, planCode string) (*SubscriptionSnapshot, error) {
	return s.setPlan(ctx, userID, strings.ToLower(strings.TrimSpace(planCode)))
}
