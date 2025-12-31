package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/raskell-io/sentinel-hub/internal/store"
	"github.com/rs/zerolog/log"
)

var (
	// ErrUserNotFound is returned when a user is not found.
	ErrUserNotFound = errors.New("user not found")

	// ErrSessionRevoked is returned when a session has been revoked.
	ErrSessionRevoked = errors.New("session has been revoked")

	// ErrSessionExpired is returned when a session has expired.
	ErrSessionExpired = errors.New("session has expired")

	// ErrEmailExists is returned when email is already registered.
	ErrEmailExists = errors.New("email already exists")
)

// Config holds authentication configuration.
type Config struct {
	JWTSecret          string
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
	BcryptCost         int
}

// DefaultConfig returns default auth configuration.
func DefaultConfig() Config {
	return Config{
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour, // 7 days
		BcryptCost:         DefaultBcryptCost,
	}
}

// Service provides authentication operations.
type Service struct {
	store  *store.Store
	config Config
}

// NewService creates a new auth service.
func NewService(s *store.Store, config Config) *Service {
	if config.AccessTokenExpiry == 0 {
		config.AccessTokenExpiry = DefaultConfig().AccessTokenExpiry
	}
	if config.RefreshTokenExpiry == 0 {
		config.RefreshTokenExpiry = DefaultConfig().RefreshTokenExpiry
	}
	if config.BcryptCost == 0 {
		config.BcryptCost = DefaultBcryptCost
	}

	return &Service{
		store:  s,
		config: config,
	}
}

// Login authenticates a user and returns a token pair.
func (s *Service) Login(ctx context.Context, email, password, ipAddress, userAgent string) (*TokenPair, error) {
	// Get user by email
	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		log.Error().Err(err).Str("email", email).Msg("Failed to get user by email")
		return nil, ErrInvalidCredentials
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	// Verify password
	if user.PasswordHash == nil {
		return nil, ErrInvalidCredentials
	}
	if err := VerifyPassword(*user.PasswordHash, password); err != nil {
		return nil, ErrInvalidCredentials
	}

	// Generate token pair
	tokenPair, err := s.createTokenPair(ctx, user, ipAddress, userAgent)
	if err != nil {
		return nil, err
	}

	// Update last login
	if err := s.store.UpdateUserLastLogin(ctx, user.ID); err != nil {
		log.Warn().Err(err).Str("user_id", user.ID).Msg("Failed to update last login")
	}

	log.Info().
		Str("user_id", user.ID).
		Str("email", user.Email).
		Str("role", string(user.Role)).
		Msg("User logged in")

	return tokenPair, nil
}

// Logout invalidates a refresh token.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	// Parse the refresh token to get the session ID
	claims, err := s.parseToken(refreshToken)
	if err != nil {
		return err
	}

	if claims.TokenType != TokenTypeRefresh {
		return ErrInvalidTokenType
	}

	// Get session by token hash
	tokenHash := HashToken(refreshToken)
	session, err := s.store.GetUserSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	if session == nil {
		return nil // Already logged out
	}

	// Revoke the session
	if err := s.store.RevokeUserSession(ctx, session.ID); err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	log.Info().
		Str("user_id", claims.UserID).
		Str("session_id", session.ID).
		Msg("User logged out")

	return nil
}

// RefreshTokens exchanges a refresh token for a new token pair.
func (s *Service) RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error) {
	// Parse the refresh token
	claims, err := s.parseToken(refreshToken)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != TokenTypeRefresh {
		return nil, ErrInvalidTokenType
	}

	// Verify session is still valid
	tokenHash := HashToken(refreshToken)
	session, err := s.store.GetUserSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if session == nil {
		return nil, ErrInvalidToken
	}

	// Check if session is revoked
	if session.RevokedAt != nil {
		return nil, ErrSessionRevoked
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	// Get user to ensure they still exist and get current role
	user, err := s.store.GetUser(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	// Revoke old session
	if err := s.store.RevokeUserSession(ctx, session.ID); err != nil {
		log.Warn().Err(err).Str("session_id", session.ID).Msg("Failed to revoke old session")
	}

	// Create new token pair with a new session
	tokenPair, err := s.createTokenPair(ctx, user, *session.IPAddress, *session.UserAgent)
	if err != nil {
		return nil, err
	}

	log.Debug().
		Str("user_id", user.ID).
		Msg("Tokens refreshed")

	return tokenPair, nil
}

// createTokenPair generates both access and refresh tokens.
func (s *Service) createTokenPair(ctx context.Context, user *store.User, ipAddress, userAgent string) (*TokenPair, error) {
	// Generate access token
	accessToken, expiresAt, err := s.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	// Create session for refresh token
	sessionID := uuid.New().String()
	refreshToken, refreshExpiresAt, err := s.generateRefreshToken(user, sessionID)
	if err != nil {
		return nil, err
	}

	// Store session
	session := &store.UserSession{
		ID:               sessionID,
		UserID:           user.ID,
		RefreshTokenHash: HashToken(refreshToken),
		ExpiresAt:        refreshExpiresAt,
		IPAddress:        &ipAddress,
		UserAgent:        &userAgent,
	}
	if err := s.store.CreateUserSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		TokenType:    "Bearer",
	}, nil
}

// CreateUser creates a new user with a hashed password.
func (s *Service) CreateUser(ctx context.Context, email, name, password string, role store.UserRole) (*store.User, error) {
	// Validate password strength
	if err := ValidatePasswordStrength(password); err != nil {
		return nil, err
	}

	// Hash password
	hash, err := HashPassword(password, s.config.BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &store.User{
		ID:           uuid.New().String(),
		Email:        email,
		Name:         name,
		Role:         role,
		PasswordHash: &hash,
	}

	if err := s.store.CreateUser(ctx, user); err != nil {
		if err.Error() == "user with this email already exists" {
			return nil, ErrEmailExists
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	log.Info().
		Str("user_id", user.ID).
		Str("email", email).
		Str("role", string(role)).
		Msg("User created")

	return user, nil
}

// UpdatePassword updates a user's password.
func (s *Service) UpdatePassword(ctx context.Context, userID, newPassword string) error {
	// Validate password strength
	if err := ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	// Get user
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return ErrUserNotFound
	}

	// Hash new password
	hash, err := HashPassword(newPassword, s.config.BcryptCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	user.PasswordHash = &hash
	if err := s.store.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	// Revoke all existing sessions (force re-login)
	if err := s.store.RevokeAllUserSessions(ctx, userID); err != nil {
		log.Warn().Err(err).Str("user_id", userID).Msg("Failed to revoke sessions after password change")
	}

	log.Info().
		Str("user_id", userID).
		Msg("Password updated")

	return nil
}

// GeneratePasswordResetToken creates a password reset token for admin use.
func (s *Service) GeneratePasswordResetToken(ctx context.Context, userID string) (string, error) {
	// Verify user exists
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return "", ErrUserNotFound
	}

	// Generate a random token
	token, err := GenerateRandomToken(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	log.Info().
		Str("user_id", userID).
		Msg("Password reset token generated")

	// For now, we return the token directly (admin shares with user)
	// In a full implementation, this would be stored with expiry
	return token, nil
}

// GetUserFromToken validates a token and returns the associated user.
func (s *Service) GetUserFromToken(ctx context.Context, tokenString string) (*store.User, error) {
	claims, err := s.ValidateAccessToken(tokenString)
	if err != nil {
		return nil, err
	}

	user, err := s.store.GetUser(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	return user, nil
}
