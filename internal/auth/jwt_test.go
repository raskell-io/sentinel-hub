package auth

import (
	"testing"
	"time"

	"github.com/raskell-io/sentinel-hub/internal/store"
)

func TestService_generateAccessToken(t *testing.T) {
	s := &Service{
		config: Config{
			JWTSecret:         "test-secret-key-for-testing",
			AccessTokenExpiry: 15 * time.Minute,
		},
	}

	user := &store.User{
		ID:    "user-123",
		Email: "test@example.com",
		Role:  store.UserRoleAdmin,
	}

	token, expiresAt, err := s.generateAccessToken(user)
	if err != nil {
		t.Fatalf("generateAccessToken failed: %v", err)
	}

	if token == "" {
		t.Fatal("generateAccessToken returned empty token")
	}

	if expiresAt.Before(time.Now()) {
		t.Error("expiresAt should be in the future")
	}

	// Token should be valid
	claims, err := s.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken failed: %v", err)
	}

	if claims.UserID != user.ID {
		t.Errorf("claims.UserID = %q, want %q", claims.UserID, user.ID)
	}

	if claims.Email != user.Email {
		t.Errorf("claims.Email = %q, want %q", claims.Email, user.Email)
	}

	if claims.Role != string(user.Role) {
		t.Errorf("claims.Role = %q, want %q", claims.Role, string(user.Role))
	}

	if claims.TokenType != TokenTypeAccess {
		t.Errorf("claims.TokenType = %q, want %q", claims.TokenType, TokenTypeAccess)
	}
}

func TestService_generateRefreshToken(t *testing.T) {
	s := &Service{
		config: Config{
			JWTSecret:          "test-secret-key-for-testing",
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		},
	}

	user := &store.User{
		ID:    "user-123",
		Email: "test@example.com",
		Role:  store.UserRoleOperator,
	}

	sessionID := "session-456"

	token, expiresAt, err := s.generateRefreshToken(user, sessionID)
	if err != nil {
		t.Fatalf("generateRefreshToken failed: %v", err)
	}

	if token == "" {
		t.Fatal("generateRefreshToken returned empty token")
	}

	if expiresAt.Before(time.Now()) {
		t.Error("expiresAt should be in the future")
	}

	// Parse the token
	claims, err := s.parseToken(token)
	if err != nil {
		t.Fatalf("parseToken failed: %v", err)
	}

	if claims.TokenType != TokenTypeRefresh {
		t.Errorf("claims.TokenType = %q, want %q", claims.TokenType, TokenTypeRefresh)
	}

	if claims.SessionID != sessionID {
		t.Errorf("claims.SessionID = %q, want %q", claims.SessionID, sessionID)
	}
}

func TestService_ValidateAccessToken_InvalidToken(t *testing.T) {
	s := &Service{
		config: Config{
			JWTSecret: "test-secret-key-for-testing",
		},
	}

	_, err := s.ValidateAccessToken("invalid-token")
	if err == nil {
		t.Error("ValidateAccessToken should fail for invalid token")
	}
}

func TestService_ValidateAccessToken_WrongSecret(t *testing.T) {
	s1 := &Service{
		config: Config{
			JWTSecret:         "secret-1",
			AccessTokenExpiry: 15 * time.Minute,
		},
	}

	s2 := &Service{
		config: Config{
			JWTSecret: "secret-2",
		},
	}

	user := &store.User{
		ID:    "user-123",
		Email: "test@example.com",
		Role:  store.UserRoleViewer,
	}

	token, _, err := s1.generateAccessToken(user)
	if err != nil {
		t.Fatalf("generateAccessToken failed: %v", err)
	}

	// Token from s1 should not validate with s2
	_, err = s2.ValidateAccessToken(token)
	if err == nil {
		t.Error("ValidateAccessToken should fail with wrong secret")
	}
}

func TestService_ValidateAccessToken_ExpiredToken(t *testing.T) {
	s := &Service{
		config: Config{
			JWTSecret:         "test-secret-key-for-testing",
			AccessTokenExpiry: -1 * time.Hour, // Already expired
		},
	}

	user := &store.User{
		ID:    "user-123",
		Email: "test@example.com",
		Role:  store.UserRoleAdmin,
	}

	token, _, err := s.generateAccessToken(user)
	if err != nil {
		t.Fatalf("generateAccessToken failed: %v", err)
	}

	_, err = s.ValidateAccessToken(token)
	if err != ErrTokenExpired {
		t.Errorf("ValidateAccessToken = %v, want %v", err, ErrTokenExpired)
	}
}

func TestService_ValidateAccessToken_RefreshTokenType(t *testing.T) {
	s := &Service{
		config: Config{
			JWTSecret:          "test-secret-key-for-testing",
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		},
	}

	user := &store.User{
		ID:    "user-123",
		Email: "test@example.com",
		Role:  store.UserRoleAdmin,
	}

	// Generate a refresh token
	token, _, err := s.generateRefreshToken(user, "session-123")
	if err != nil {
		t.Fatalf("generateRefreshToken failed: %v", err)
	}

	// Trying to validate as access token should fail
	_, err = s.ValidateAccessToken(token)
	if err != ErrInvalidTokenType {
		t.Errorf("ValidateAccessToken = %v, want %v", err, ErrInvalidTokenType)
	}
}

func TestTokenType_Constants(t *testing.T) {
	// Ensure token types are distinct
	if TokenTypeAccess == TokenTypeRefresh {
		t.Error("TokenTypeAccess and TokenTypeRefresh should be different")
	}
}
