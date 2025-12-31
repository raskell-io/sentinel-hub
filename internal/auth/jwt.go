package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/raskell-io/sentinel-hub/internal/store"
)

var (
	// ErrInvalidToken is returned when a token is invalid.
	ErrInvalidToken = errors.New("invalid token")

	// ErrTokenExpired is returned when a token has expired.
	ErrTokenExpired = errors.New("token expired")

	// ErrInvalidTokenType is returned when token type doesn't match expected.
	ErrInvalidTokenType = errors.New("invalid token type")
)

// TokenType identifies the type of JWT token.
type TokenType string

const (
	// TokenTypeAccess is an access token (short-lived).
	TokenTypeAccess TokenType = "access"

	// TokenTypeRefresh is a refresh token (long-lived).
	TokenTypeRefresh TokenType = "refresh"
)

// Claims represents the JWT claims.
type Claims struct {
	jwt.RegisteredClaims
	UserID    string    `json:"uid"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	TokenType TokenType `json:"type"`
	SessionID string    `json:"sid,omitempty"` // Only for refresh tokens
}

// TokenPair contains both access and refresh tokens.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// generateAccessToken creates a new access token for a user.
func (s *Service) generateAccessToken(user *store.User) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.config.AccessTokenExpiry)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Issuer:    "sentinel-hub",
		},
		UserID:    user.ID,
		Email:     user.Email,
		Role:      string(user.Role),
		TokenType: TokenTypeAccess,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(s.config.JWTSecret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign access token: %w", err)
	}

	return signedToken, expiresAt, nil
}

// generateRefreshToken creates a new refresh token for a user.
func (s *Service) generateRefreshToken(user *store.User, sessionID string) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.config.RefreshTokenExpiry)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Issuer:    "sentinel-hub",
		},
		UserID:    user.ID,
		Email:     user.Email,
		Role:      string(user.Role),
		TokenType: TokenTypeRefresh,
		SessionID: sessionID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(s.config.JWTSecret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return signedToken, expiresAt, nil
}

// ValidateAccessToken validates an access token and returns the claims.
func (s *Service) ValidateAccessToken(tokenString string) (*Claims, error) {
	claims, err := s.parseToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != TokenTypeAccess {
		return nil, ErrInvalidTokenType
	}

	return claims, nil
}

// parseToken parses and validates a JWT token.
func (s *Service) parseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWTSecret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// HashToken creates a SHA256 hash of a token for storage.
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// GenerateRandomToken generates a cryptographically secure random token.
func GenerateRandomToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
