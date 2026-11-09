package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"go-be/internal/models"
)

type BillingRepository struct {
	DB *pgxpool.Pool
}

func NewBillingRepository(db *pgxpool.Pool) *BillingRepository {
	return &BillingRepository{DB: db}
}

func (r *BillingRepository) ListPlans(ctx context.Context) ([]models.Plan, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT code, name, description, is_active, created_at, updated_at
		FROM plans
		ORDER BY CASE code WHEN 'free' THEN 0 ELSE 1 END, code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plans := []models.Plan{}
	for rows.Next() {
		var p models.Plan
		if err := rows.Scan(&p.Code, &p.Name, &p.Description, &p.IsActive, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

func (r *BillingRepository) ListEntitlements(ctx context.Context, planCode string) ([]models.PlanEntitlement, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT plan_code, feature_key, limit_value
		FROM plan_entitlements
		WHERE plan_code = $1
		ORDER BY feature_key
	`, planCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntitlements(rows)
}

func (r *BillingRepository) ListAllEntitlements(ctx context.Context) ([]models.PlanEntitlement, error) {
	rows, err := r.DB.Query(ctx, `
		SELECT plan_code, feature_key, limit_value
		FROM plan_entitlements
		ORDER BY plan_code, feature_key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntitlements(rows)
}

type entitlementRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanEntitlements(rows entitlementRows) ([]models.PlanEntitlement, error) {
	items := []models.PlanEntitlement{}
	for rows.Next() {
		var e models.PlanEntitlement
		if err := rows.Scan(&e.PlanCode, &e.FeatureKey, &e.LimitValue); err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

// GetSubscription returns the user's subscription or pgx.ErrNoRows when missing.
func (r *BillingRepository) GetSubscription(ctx context.Context, userID string) (*models.Subscription, error) {
	var s models.Subscription
	err := r.DB.QueryRow(ctx, `
		SELECT id, user_id, plan_code, status, started_at, updated_at
		FROM subscriptions
		WHERE user_id = $1
	`, userID).Scan(&s.ID, &s.UserID, &s.PlanCode, &s.Status, &s.StartedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *BillingRepository) EnsureFreeSubscription(ctx context.Context, userID string) error {
	_, err := r.DB.Exec(ctx, `
		INSERT INTO subscriptions (user_id, plan_code, status)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO NOTHING
	`, userID, models.PlanCodeFree, models.SubscriptionStatusActive)
	return err
}

// UpsertSubscription inserts or updates the plan; started_at resets only when the plan changes.
func (r *BillingRepository) UpsertSubscription(ctx context.Context, userID, planCode string) (*models.Subscription, error) {
	var s models.Subscription
	err := r.DB.QueryRow(ctx, `
		INSERT INTO subscriptions (user_id, plan_code, status)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET
			started_at = CASE
				WHEN subscriptions.plan_code IS DISTINCT FROM EXCLUDED.plan_code THEN CURRENT_TIMESTAMP
				ELSE subscriptions.started_at
			END,
			plan_code = EXCLUDED.plan_code,
			status = $3,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id, user_id, plan_code, status, started_at, updated_at
	`, userID, planCode, models.SubscriptionStatusActive).
		Scan(&s.ID, &s.UserID, &s.PlanCode, &s.Status, &s.StartedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *BillingRepository) count(ctx context.Context, query string, args ...any) (int, error) {
	var n int
	if err := r.DB.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *BillingRepository) CountGoals(ctx context.Context, userID string) (int, error) {
	return r.count(ctx, `SELECT COUNT(*) FROM goals WHERE user_id = $1`, userID)
}

func (r *BillingRepository) CountHabits(ctx context.Context, userID string) (int, error) {
	return r.count(ctx, `SELECT COUNT(*) FROM habits WHERE user_id = $1`, userID)
}

func (r *BillingRepository) CountPlannerForDate(ctx context.Context, userID string, date time.Time) (int, error) {
	return r.count(ctx, `SELECT COUNT(*) FROM planner_tasks WHERE user_id = $1 AND plan_date = $2::date`,
		userID, date.Format("2006-01-02"))
}

func (r *BillingRepository) CountCalendar(ctx context.Context, userID string) (int, error) {
	return r.count(ctx, `SELECT COUNT(*) FROM calendar_events WHERE user_id = $1`, userID)
}

func (r *BillingRepository) CountNotes(ctx context.Context, userID string) (int, error) {
	return r.count(ctx, `SELECT COUNT(*) FROM notes WHERE user_id = $1`, userID)
}

func (r *BillingRepository) CountFinance(ctx context.Context, userID string) (int, error) {
	return r.count(ctx, `SELECT COUNT(*) FROM finance_transactions WHERE user_id = $1`, userID)
}

func (r *BillingRepository) CountReminders(ctx context.Context, userID string) (int, error) {
	return r.count(ctx, `SELECT COUNT(*) FROM reminders WHERE user_id = $1`, userID)
}
