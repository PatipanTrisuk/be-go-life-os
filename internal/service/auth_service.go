package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"go-be/internal/authz"
	"go-be/internal/mailer"
	"go-be/internal/models"
	"go-be/internal/oauth"
	"go-be/internal/repository"
	"go-be/internal/utils"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidInput   = errors.New("invalid input")
	ErrEmailExists    = errors.New("email already exists")
	ErrUsernameExists = errors.New("username already exists")
)

var (
	ErrInvalidCredentials   = errors.New("invalid email or password")
	ErrAccountLocked        = errors.New("account temporarily locked")
	ErrEmailNotVerified     = errors.New("email not verified")
	ErrEmailAlreadyVerified = errors.New("email already verified")
	ErrInvalidVerifyToken   = errors.New("invalid or expired verification token")
	ErrJWTSecretMissing     = errors.New("JWT secret is missing")
	ErrUserNotFound         = errors.New("user not found")
	ErrInvalidResetToken    = errors.New("invalid or expired reset token")
	ErrWrongPassword        = errors.New("current password is incorrect")
	ErrPasswordNotSet       = errors.New("password not set for this account")
	ErrDeviceNotFound       = errors.New("device not found")
	ErrRefreshTokenReuse    = errors.New("refresh token reuse detected")
	ErrDiscordNotConfigured = oauth.ErrDiscordNotConfigured
	ErrDiscordEmailMissing  = oauth.ErrDiscordEmailMissing
	ErrDiscordAuthFailed    = errors.New("discord authentication failed")
)

type AuthService struct {
	UserRepo              *repository.UserRepository
	RefreshTokenRepo      *repository.RefreshTokenRepository
	DeviceRepo            *repository.DeviceRepository
	LoginAttemptRepo      *repository.LoginAttemptRepository
	PasswordResetRepo     *repository.PasswordResetRepository
	EmailVerificationRepo *repository.EmailVerificationRepository
	Mailer                AuthMailer
	FrontendURL           string
	Discord               *oauth.DiscordClient
	BillingRepo           *repository.BillingRepository
}

// ensureSubscription creates a free subscription for a newly created user.
func (s *AuthService) ensureSubscription(ctx context.Context, userID string) error {
	if s.BillingRepo == nil {
		return nil
	}
	return s.BillingRepo.EnsureFreeSubscription(ctx, userID)
}

type AuthMailer interface {
	SendPasswordReset(to string, data mailer.PasswordResetEmailData) error
	SendEmailVerification(to string, data mailer.EmailVerificationData) error
}

type AccessTokenClaims struct {
	RoleID string `json:"role_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func NewAuthService(
	userRepo *repository.UserRepository,
	refreshTokenRepo *repository.RefreshTokenRepository,
	deviceRepo *repository.DeviceRepository,
	loginAttemptRepo *repository.LoginAttemptRepository,
	passwordResetRepo *repository.PasswordResetRepository,
	emailVerificationRepo *repository.EmailVerificationRepository,
	mailer AuthMailer,
	frontendURL string,
	discord *oauth.DiscordClient,
	billingRepo *repository.BillingRepository,
) *AuthService {
	frontendURL = strings.TrimRight(strings.TrimSpace(frontendURL), "/")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	return &AuthService{
		UserRepo:              userRepo,
		RefreshTokenRepo:      refreshTokenRepo,
		DeviceRepo:            deviceRepo,
		LoginAttemptRepo:      loginAttemptRepo,
		PasswordResetRepo:     passwordResetRepo,
		EmailVerificationRepo: emailVerificationRepo,
		Mailer:                mailer,
		FrontendURL:           frontendURL,
		Discord:               discord,
		BillingRepo:           billingRepo,
	}
}

type RegisterInput struct {
	Username string
	Email    string
	Password string
}

type DeviceInput struct {
	DeviceKey string
	Name      string
	UserAgent string
	IPAddress string
}

func (s *AuthService) Register(
	ctx context.Context,
	input *RegisterInput,
) (*models.User, error) {

	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)

	if len(input.Username) < 3 || len(input.Username) > 50 {
		return nil, ErrInvalidInput
	}

	if len(input.Email) == 0 || len(input.Email) > 255 || !strings.Contains(input.Email, "@") {
		return nil, ErrInvalidInput
	}

	if len(input.Password) < 8 || len(input.Password) > 72 {
		return nil, ErrInvalidInput
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	roleID, err := s.UserRepo.FindRoleIDByName(ctx, authz.RoleUser)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Username: input.Username,
		Email:    input.Email,
		Password: string(hashedPassword),
		RoleID:   roleID,
		Role:     authz.RoleUser,
	}

	err = s.UserRepo.Create(ctx, user)

	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "users_email_key":
				return nil, ErrEmailExists
			case "users_username_key":
				return nil, ErrUsernameExists
			}
		}

		return nil, err
	}

	if err := s.ensureSubscription(ctx, user.ID.String()); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *AuthService) RegisterAndSendVerification(
	ctx context.Context,
	input *RegisterInput,
	locale string,
) (*models.User, error) {
	user, err := s.Register(ctx, input)
	if err != nil {
		return nil, err
	}

	if sendErr := s.sendEmailVerification(ctx, user, locale); sendErr != nil {
		log.Printf("send verification email failed for %s: %v", user.Email, sendErr)
	}

	return user, nil
}

func (s *AuthService) Login(
	ctx context.Context,
	email string,
	password string,
	device *DeviceInput,
) (string, string, *models.User, error) {

	email = strings.ToLower(strings.TrimSpace(email))

	locked, err := s.LoginAttemptRepo.IsLocked(ctx, email)
	if err != nil {
		return "", "", nil, err
	}
	if locked {
		return "", "", nil, ErrAccountLocked
	}

	user, err := s.UserRepo.FindByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		if failErr := s.recordLoginFailure(ctx, email); failErr != nil {
			return "", "", nil, failErr
		}
		return "", "", nil, ErrInvalidCredentials
	}
	if err != nil {
		return "", "", nil, err
	}

	if strings.TrimSpace(user.Password) == "" {
		if failErr := s.recordLoginFailure(ctx, email); failErr != nil {
			return "", "", nil, failErr
		}
		return "", "", nil, ErrInvalidCredentials
	}

	err = bcrypt.CompareHashAndPassword(
		[]byte(user.Password),
		[]byte(password),
	)
	if err != nil {
		if failErr := s.recordLoginFailure(ctx, email); failErr != nil {
			return "", "", nil, failErr
		}
		return "", "", nil, ErrInvalidCredentials
	}

	if user.EmailVerifiedAt == nil {
		return "", "", nil, ErrEmailNotVerified
	}

	if err := s.LoginAttemptRepo.Reset(ctx, email); err != nil {
		return "", "", nil, err
	}

	accessToken, err := s.issueAccessToken(user)
	if err != nil {
		return "", "", nil, err
	}

	savedDevice, err := s.upsertDevice(ctx, user.ID.String(), device)
	if err != nil {
		return "", "", nil, err
	}

	var deviceID *string
	if savedDevice != nil {
		deviceID = &savedDevice.ID
	}

	refreshToken, err := s.issueRefreshToken(ctx, user.ID.String(), deviceID)
	if err != nil {
		return "", "", nil, err
	}

	authz.AttachPermissions(user)
	return accessToken, refreshToken, user, nil
}

func (s *AuthService) recordLoginFailure(ctx context.Context, email string) error {
	locked, err := s.LoginAttemptRepo.RegisterFailure(ctx, email)
	if err != nil {
		return err
	}
	if locked {
		return ErrAccountLocked
	}
	return nil
}

func (s *AuthService) LoginWithDiscord(
	ctx context.Context,
	code string,
	redirectURI string,
	device *DeviceInput,
) (string, string, *models.User, error) {
	if s.Discord == nil {
		return "", "", nil, ErrDiscordNotConfigured
	}

	accessToken, err := s.Discord.ExchangeCode(ctx, code, redirectURI)
	if err != nil {
		if errors.Is(err, oauth.ErrDiscordNotConfigured) {
			return "", "", nil, ErrDiscordNotConfigured
		}
		return "", "", nil, ErrDiscordAuthFailed
	}

	discordUser, err := s.Discord.FetchUser(ctx, accessToken)
	if err != nil {
		switch {
		case errors.Is(err, oauth.ErrDiscordEmailMissing):
			return "", "", nil, ErrDiscordEmailMissing
		case errors.Is(err, oauth.ErrDiscordNotConfigured):
			return "", "", nil, ErrDiscordNotConfigured
		default:
			return "", "", nil, ErrDiscordAuthFailed
		}
	}

	email := strings.ToLower(strings.TrimSpace(discordUser.Email))
	user, err := s.UserRepo.FindByDiscordID(ctx, discordUser.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		user, err = s.UserRepo.FindByEmail(ctx, email)
		switch {
		case err == nil:
			if user.DiscordID == nil || *user.DiscordID == "" {
				if linkErr := s.UserRepo.LinkDiscordID(ctx, user.ID.String(), discordUser.ID); linkErr != nil {
					return "", "", nil, linkErr
				}
				id := discordUser.ID
				user.DiscordID = &id
			} else if *user.DiscordID != discordUser.ID {
				return "", "", nil, ErrEmailExists
			}
		case errors.Is(err, pgx.ErrNoRows):
			username, genErr := s.uniqueUsername(ctx, discordUser.DisplayName())
			if genErr != nil {
				return "", "", nil, genErr
			}
			discordID := discordUser.ID
			now := time.Now()
			roleID, roleErr := s.UserRepo.FindRoleIDByName(ctx, authz.RoleUser)
			if roleErr != nil {
				return "", "", nil, roleErr
			}
			user = &models.User{
				Username:        username,
				Email:           email,
				RoleID:          roleID,
				Role:            authz.RoleUser,
				AuthProvider:    "discord",
				DiscordID:       &discordID,
				EmailVerifiedAt: &now,
			}
			if createErr := s.UserRepo.Create(ctx, user); createErr != nil {
				var pgErr *pgconn.PgError
				if errors.As(createErr, &pgErr) && pgErr.Code == "23505" {
					switch pgErr.ConstraintName {
					case "users_email_key":
						return "", "", nil, ErrEmailExists
					case "users_username_key":
						return "", "", nil, ErrUsernameExists
					case "users_discord_id_key":
						return "", "", nil, ErrDiscordAuthFailed
					}
				}
				return "", "", nil, createErr
			}
			if subErr := s.ensureSubscription(ctx, user.ID.String()); subErr != nil {
				return "", "", nil, subErr
			}
		default:
			return "", "", nil, err
		}
	} else if err != nil {
		return "", "", nil, err
	}

	issuedAccess, err := s.issueAccessToken(user)
	if err != nil {
		return "", "", nil, err
	}

	savedDevice, err := s.upsertDevice(ctx, user.ID.String(), device)
	if err != nil {
		return "", "", nil, err
	}

	var deviceID *string
	if savedDevice != nil {
		deviceID = &savedDevice.ID
	}

	refreshToken, err := s.issueRefreshToken(ctx, user.ID.String(), deviceID)
	if err != nil {
		return "", "", nil, err
	}

	authz.AttachPermissions(user)
	return issuedAccess, refreshToken, user, nil
}

func (s *AuthService) uniqueUsername(ctx context.Context, base string) (string, error) {
	base = sanitizeUsername(base)
	if len(base) < 3 {
		base = "discord"
	}
	if len(base) > 40 {
		base = base[:40]
	}

	candidate := base
	for i := 0; i < 20; i++ {
		exists, err := s.UserRepo.UsernameExists(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}

		suffix, err := generateDeviceKey()
		if err != nil {
			return "", err
		}
		if len(suffix) > 6 {
			suffix = suffix[:6]
		}
		candidate = base + "_" + suffix
		if len(candidate) > 50 {
			candidate = candidate[:50]
		}
	}

	return "", ErrUsernameExists
}

func sanitizeUsername(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
	}
	return b.String()
}

func (s *AuthService) GetProfile(
	ctx context.Context,
	userId string,
) (*models.User, error) {
	user, err := s.UserRepo.FindByID(ctx, userId)
	if err != nil {
		return nil, err
	}
	authz.AttachPermissions(user)
	return user, nil
}

func (s *AuthService) ListDevices(
	ctx context.Context,
	userID string,
) ([]models.Device, error) {
	return s.DeviceRepo.ListByUserID(ctx, userID)
}

func (s *AuthService) UpdateUsername(
	ctx context.Context,
	userID string,
	username string,
) (*models.User, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 50 {
		return nil, ErrInvalidInput
	}

	if err := s.UserRepo.UpdateUsername(ctx, userID, username); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrUsernameExists
		}
		return nil, err
	}

	return s.GetProfile(ctx, userID)
}

func (s *AuthService) ChangePassword(
	ctx context.Context,
	userID string,
	currentPassword string,
	newPassword string,
) error {
	if len(newPassword) < 8 || len(newPassword) > 72 {
		return ErrInvalidInput
	}

	user, err := s.UserRepo.FindByID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}

	if strings.TrimSpace(user.Password) == "" {
		return ErrPasswordNotSet
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(currentPassword)); err != nil {
		return ErrWrongPassword
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.UserRepo.UpdatePassword(ctx, userID, string(hashedPassword)); err != nil {
		return err
	}

	return s.RefreshTokenRepo.RevokeAllByUserID(ctx, userID)
}

func (s *AuthService) RevokeDevice(
	ctx context.Context,
	userID string,
	deviceID string,
) error {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return ErrInvalidInput
	}

	if _, err := s.DeviceRepo.FindByUserAndID(ctx, userID, deviceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDeviceNotFound
		}
		return err
	}

	if err := s.RefreshTokenRepo.RevokeByDeviceID(ctx, userID, deviceID); err != nil {
		return err
	}

	if err := s.DeviceRepo.DeleteByUserAndID(ctx, userID, deviceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDeviceNotFound
		}
		return err
	}

	return nil
}

func (s *AuthService) LogoutAllSessions(ctx context.Context, userID string) error {
	return s.RefreshTokenRepo.RevokeAllByUserID(ctx, userID)
}

func (s *AuthService) RequestEmailVerification(
	ctx context.Context,
	email string,
	locale string,
) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return ErrInvalidInput
	}

	user, err := s.UserRepo.FindByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	if user.EmailVerifiedAt != nil {
		return nil
	}

	return s.sendEmailVerification(ctx, user, locale)
}

func (s *AuthService) VerifyEmail(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidVerifyToken
	}

	record, err := s.EmailVerificationRepo.FindValidByHash(ctx, utils.HashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidVerifyToken
	}
	if err != nil {
		return err
	}

	user, err := s.UserRepo.FindByID(ctx, record.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}

	if user.EmailVerifiedAt != nil {
		_ = s.EmailVerificationRepo.MarkUsed(ctx, record.ID)
		return ErrEmailAlreadyVerified
	}

	if err := s.UserRepo.SetEmailVerified(ctx, record.UserID, time.Now()); err != nil {
		return err
	}

	if err := s.EmailVerificationRepo.MarkUsed(ctx, record.ID); err != nil {
		return err
	}

	_ = s.EmailVerificationRepo.InvalidateActiveByUserID(ctx, record.UserID)
	return nil
}

func (s *AuthService) sendEmailVerification(
	ctx context.Context,
	user *models.User,
	locale string,
) error {
	locale = normalizeLocale(locale)

	rawToken, err := GenerateRefreshToken()
	if err != nil {
		return err
	}

	if err := s.EmailVerificationRepo.InvalidateActiveByUserID(ctx, user.ID.String()); err != nil {
		return err
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	if _, err := s.EmailVerificationRepo.Create(
		ctx,
		user.ID.String(),
		utils.HashToken(rawToken),
		expiresAt,
	); err != nil {
		return err
	}

	if s.Mailer == nil {
		return errors.New("mailer is not configured")
	}

	expiresIn := "24 hours"
	if locale == "th" {
		expiresIn = "24 ชั่วโมง"
	}

	verifyURL := s.FrontendURL + "/" + locale + "/verify-email?token=" + rawToken
	return s.Mailer.SendEmailVerification(user.Email, mailer.EmailVerificationData{
		Username:  user.Username,
		VerifyURL: verifyURL,
		ExpiresIn: expiresIn,
		Locale:    locale,
	})
}

func normalizeLocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if locale != "th" {
		return "en"
	}
	return locale
}

func (s *AuthService) RequestPasswordReset(
	ctx context.Context,
	email string,
	locale string,
) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return ErrInvalidInput
	}

	locale = normalizeLocale(locale)

	user, err := s.UserRepo.FindByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		// Do not reveal whether the email exists.
		return nil
	}
	if err != nil {
		return err
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

func (s *AuthService) ResetPassword(
	ctx context.Context,
	token string,
	password string,
) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidResetToken
	}
	if len(password) < 8 || len(password) > 72 {
		return ErrInvalidInput
	}

	resetToken, err := s.PasswordResetRepo.FindValidByHash(ctx, utils.HashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidResetToken
	}
	if err != nil {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.UserRepo.UpdatePassword(ctx, resetToken.UserID, string(hashedPassword)); err != nil {
		return err
	}

	// Owning the reset link proves email access.
	_ = s.UserRepo.SetEmailVerified(ctx, resetToken.UserID, time.Now())

	if err := s.PasswordResetRepo.MarkUsed(ctx, resetToken.ID); err != nil {
		return err
	}

	_ = s.PasswordResetRepo.InvalidateActiveByUserID(ctx, resetToken.UserID)
	_ = s.RefreshTokenRepo.RevokeAllByUserID(ctx, resetToken.UserID)

	return nil
}

func (s *AuthService) Logout(
	ctx context.Context,
	refreshToken string,
) error {

	tokenHash := utils.HashToken(refreshToken)

	return s.RefreshTokenRepo.Revoke(ctx, tokenHash)
}

var ErrInvalidRefreshToken = errors.New("invalid refresh token")

func (s *AuthService) Refresh(
	ctx context.Context,
	refreshToken string,
	device *DeviceInput,
) (string, string, error) {
	tokenHash := utils.HashToken(refreshToken)
	active, err := s.RefreshTokenRepo.FindActive(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if record, findErr := s.RefreshTokenRepo.FindByHash(ctx, tokenHash); findErr == nil && record.RevokedAt != nil {
				_ = s.RefreshTokenRepo.RevokeAllByUserID(ctx, record.UserID)
				return "", "", ErrRefreshTokenReuse
			}
			return "", "", ErrInvalidRefreshToken
		}
		return "", "", err
	}

	user, err := s.UserRepo.FindByID(ctx, active.UserID)
	if err != nil {
		return "", "", err
	}

	accessToken, err := s.issueAccessToken(user)
	if err != nil {
		return "", "", err
	}

	deviceID := active.DeviceID
	switch {
	case device != nil && strings.TrimSpace(device.DeviceKey) != "":
		savedDevice, err := s.upsertDevice(ctx, user.ID.String(), device)
		if err != nil {
			return "", "", err
		}
		if savedDevice != nil {
			deviceID = &savedDevice.ID
		}
	case deviceID != nil:
		_ = s.DeviceRepo.Touch(ctx, *deviceID, deviceUserAgent(device), deviceIPAddress(device))
	default:
		savedDevice, err := s.upsertDevice(ctx, user.ID.String(), device)
		if err != nil {
			return "", "", err
		}
		if savedDevice != nil {
			deviceID = &savedDevice.ID
		}
	}

	newRefreshToken, err := s.issueRefreshToken(ctx, user.ID.String(), deviceID)
	if err != nil {
		return "", "", err
	}

	if err := s.Logout(ctx, refreshToken); err != nil {
		return "", "", err
	}

	return accessToken, newRefreshToken, nil
}

func (s *AuthService) issueAccessToken(user *models.User) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return "", ErrJWTSecretMissing
	}

	now := time.Now()
	claims := AccessTokenClaims{
		RoleID: user.RoleID.String(),
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func (s *AuthService) issueRefreshToken(
	ctx context.Context,
	userID string,
	deviceID *string,
) (string, error) {
	raw, err := GenerateRefreshToken()
	if err != nil {
		return "", err
	}

	err = s.RefreshTokenRepo.Create(
		ctx,
		userID,
		utils.HashToken(raw),
		time.Now().Add(7*24*time.Hour),
		deviceID,
	)
	if err != nil {
		return "", err
	}

	return raw, nil
}

func (s *AuthService) upsertDevice(
	ctx context.Context,
	userID string,
	device *DeviceInput,
) (*models.Device, error) {
	if device == nil {
		device = &DeviceInput{}
	}

	deviceKey := strings.TrimSpace(device.DeviceKey)
	if deviceKey == "" {
		generated, err := generateDeviceKey()
		if err != nil {
			return nil, err
		}
		deviceKey = generated
	}

	if len(deviceKey) > 128 {
		return nil, ErrInvalidInput
	}

	name := strings.TrimSpace(device.Name)
	if len(name) > 100 {
		name = name[:100]
	}

	return s.DeviceRepo.Upsert(ctx, repository.DeviceUpsertInput{
		UserID:    userID,
		DeviceKey: deviceKey,
		Name:      name,
		UserAgent: strings.TrimSpace(device.UserAgent),
		IPAddress: strings.TrimSpace(device.IPAddress),
	})
}

func generateDeviceKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func deviceUserAgent(device *DeviceInput) string {
	if device == nil {
		return ""
	}
	return device.UserAgent
}

func deviceIPAddress(device *DeviceInput) string {
	if device == nil {
		return ""
	}
	return device.IPAddress
}
