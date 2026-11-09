package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"go-be/internal/authz"
	"go-be/internal/mailer"
	"go-be/internal/models"
	"go-be/internal/repository"
	"go-be/internal/utils"
)

var (
	ErrCannotTargetSelf   = errors.New("cannot perform this action on yourself")
	ErrEmailVerified      = errors.New("email already verified")
	ErrUserAlreadyDeleted = errors.New("user already deleted")
	ErrUserNotDeleted     = errors.New("user is not deleted")
)

func (s *AuthService) AdminGetUser(ctx context.Context, userID string) (*models.User, error) {
	user, err := s.UserRepo.FindByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	return user, err
}

type AdminUserInput struct {
	Username string
	Email    string
	RoleID   string
}

func (s *AuthService) resolveRoleID(ctx context.Context, roleIDOrName string) (uuid.UUID, string, error) {
	roleIDOrName = strings.TrimSpace(roleIDOrName)
	if roleIDOrName == "" {
		roleIDOrName = authz.RoleUser
	}

	if id, err := uuid.Parse(roleIDOrName); err == nil {
		name, err := s.roleNameByID(ctx, id)
		if err != nil {
			return uuid.Nil, "", ErrInvalidInput
		}
		if !authz.IsValidRole(id.String()) && !authz.IsValidRole(name) {
			return uuid.Nil, "", ErrInvalidInput
		}
		return id, name, nil
	}

	id, err := s.UserRepo.FindRoleIDByName(ctx, roleIDOrName)
	if err != nil {
		return uuid.Nil, "", ErrInvalidInput
	}
	if !authz.IsValidRole(id.String()) && !authz.IsValidRole(roleIDOrName) {
		return uuid.Nil, "", ErrInvalidInput
	}
	return id, authz.NormalizeRole(roleIDOrName), nil
}

func (s *AuthService) roleNameByID(ctx context.Context, id uuid.UUID) (string, error) {
	var name string
	err := s.UserRepo.DB.QueryRow(ctx, `SELECT name FROM roles WHERE id = $1`, id).Scan(&name)
	if err != nil {
		return "", err
	}
	return name, nil
}

func normalizeAdminUserInput(input AdminUserInput) (AdminUserInput, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.RoleID = strings.TrimSpace(input.RoleID)

	if len(input.Username) < 3 || len(input.Username) > 50 {
		return input, ErrInvalidInput
	}
	if len(input.Email) == 0 || len(input.Email) > 255 || !strings.Contains(input.Email, "@") {
		return input, ErrInvalidInput
	}
	return input, nil
}

func mapUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "users_email_key", "users_email_active_key":
			return ErrEmailExists
		case "users_username_key", "users_username_active_key":
			return ErrUsernameExists
		}
	}
	return err
}

func (s *AuthService) AdminCreateUser(ctx context.Context, input AdminUserInput) (*models.User, error) {
	input, err := normalizeAdminUserInput(input)
	if err != nil {
		return nil, err
	}

	roleID, roleName, err := s.resolveRoleID(ctx, input.RoleID)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Username:     input.Username,
		Email:        input.Email,
		RoleID:       roleID,
		Role:         roleName,
		AuthProvider: "local",
	}

	if err := s.UserRepo.Create(ctx, user); err != nil {
		return nil, mapUniqueViolation(err)
	}
	if err := s.ensureSubscription(ctx, user.ID.String()); err != nil {
		return nil, err
	}

	return s.AdminGetUser(ctx, user.ID.String())
}

func (s *AuthService) AdminUpdateUser(ctx context.Context, actorID, targetID string, input AdminUserInput) (*models.User, error) {
	input, err := normalizeAdminUserInput(input)
	if err != nil {
		return nil, err
	}

	existing, err := s.AdminGetUser(ctx, targetID)
	if err != nil {
		return nil, err
	}

	roleID, roleName, err := s.resolveRoleID(ctx, input.RoleID)
	if err != nil {
		return nil, err
	}

	// Prevent an admin from removing their own admin role.
	if strings.EqualFold(actorID, targetID) &&
		authz.NormalizeRole(existing.Role) == authz.RoleAdmin &&
		authz.NormalizeRole(roleName) != authz.RoleAdmin {
		return nil, ErrCannotTargetSelf
	}

	if err := s.UserRepo.UpdateProfile(ctx, targetID, repository.UserProfileUpdate{
		Username: input.Username,
		Email:    input.Email,
		RoleID:   roleID.String(),
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, mapUniqueViolation(err)
	}

	return s.AdminGetUser(ctx, targetID)
}

func (s *AuthService) AdminDeleteUser(ctx context.Context, actorID, targetID string) error {
	if strings.EqualFold(actorID, targetID) {
		return ErrCannotTargetSelf
	}

	user, err := s.UserRepo.FindByIDAny(ctx, targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	if user.DeletedAt != nil {
		return ErrUserAlreadyDeleted
	}

	if err := s.UserRepo.SoftDelete(ctx, targetID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}

	_ = s.RefreshTokenRepo.RevokeAllByUserID(ctx, targetID)
	_ = s.PasswordResetRepo.InvalidateActiveByUserID(ctx, targetID)
	return nil
}

func (s *AuthService) AdminRestoreUser(ctx context.Context, actorID, targetID string) (*models.User, error) {
	if strings.EqualFold(actorID, targetID) {
		return nil, ErrCannotTargetSelf
	}

	user, err := s.UserRepo.FindByIDAny(ctx, targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if user.DeletedAt == nil {
		return nil, ErrUserNotDeleted
	}

	if err := s.UserRepo.Restore(ctx, targetID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, mapUniqueViolation(err)
	}

	return s.AdminGetUser(ctx, targetID)
}

func (s *AuthService) AdminKickUser(ctx context.Context, actorID, targetID string) error {
	if strings.EqualFold(actorID, targetID) {
		return ErrCannotTargetSelf
	}

	if _, err := s.AdminGetUser(ctx, targetID); err != nil {
		return err
	}

	return s.RefreshTokenRepo.RevokeAllByUserID(ctx, targetID)
}

func (s *AuthService) AdminSetPassword(ctx context.Context, actorID, targetID, password string) error {
	if strings.EqualFold(actorID, targetID) {
		return ErrCannotTargetSelf
	}
	if len(password) < 8 || len(password) > 72 {
		return ErrInvalidInput
	}

	if _, err := s.AdminGetUser(ctx, targetID); err != nil {
		return err
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.UserRepo.UpdatePassword(ctx, targetID, string(hashed)); err != nil {
		return err
	}

	_ = s.PasswordResetRepo.InvalidateActiveByUserID(ctx, targetID)
	_ = s.RefreshTokenRepo.RevokeAllByUserID(ctx, targetID)
	return nil
}

func (s *AuthService) AdminSendPasswordReset(ctx context.Context, targetID, locale string) error {
	user, err := s.AdminGetUser(ctx, targetID)
	if err != nil {
		return err
	}

	locale = strings.ToLower(strings.TrimSpace(locale))
	if locale != "th" {
		locale = "en"
	}

	rawToken, err := GenerateRefreshToken()
	if err != nil {
		return err
	}

	if err := s.PasswordResetRepo.InvalidateActiveByUserID(ctx, user.ID.String()); err != nil {
		return err
	}

	expiresAt := time.Now().Add(1 * time.Hour)
	if _, err := s.PasswordResetRepo.Create(
		ctx,
		user.ID.String(),
		utils.HashToken(rawToken),
		expiresAt,
	); err != nil {
		return err
	}

	resetURL := s.FrontendURL + "/" + locale + "/reset-password?token=" + rawToken

	if s.Mailer == nil {
		return errors.New("mailer is not configured")
	}

	expiresIn := "1 hour"
	if locale == "th" {
		expiresIn = "1 ชั่วโมง"
	}

	return s.Mailer.SendPasswordReset(user.Email, mailer.PasswordResetEmailData{
		Username:  user.Username,
		ResetURL:  resetURL,
		ExpiresIn: expiresIn,
		Locale:    locale,
	})
}

func (s *AuthService) AdminVerifyEmail(ctx context.Context, targetID string) (*models.User, error) {
	user, err := s.AdminGetUser(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if user.EmailVerifiedAt != nil {
		return nil, ErrEmailVerified
	}

	if err := s.UserRepo.SetEmailVerified(ctx, targetID, time.Now()); err != nil {
		return nil, err
	}

	return s.AdminGetUser(ctx, targetID)
}
