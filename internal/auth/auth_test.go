package auth

import (
	"context"
	"testing"
	"time"

	"github.com/raskell-io/sentinel-hub/internal/store"
)

func setupTestService(t *testing.T) (*Service, *store.Store) {
	db, err := store.New("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	config := Config{
		JWTSecret:          "test-secret-key-for-testing-12345",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4, // Low cost for faster tests
	}

	svc := NewService(db, config)
	return svc, db
}

func TestNewService(t *testing.T) {
	db, err := store.New("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	config := DefaultConfig()
	config.JWTSecret = "test-secret"

	svc := NewService(db, config)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}

func TestNewService_DefaultValues(t *testing.T) {
	db, err := store.New("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	// Empty config should get defaults
	svc := NewService(db, Config{JWTSecret: "secret"})
	if svc.config.AccessTokenExpiry == 0 {
		t.Error("AccessTokenExpiry should have default value")
	}
	if svc.config.RefreshTokenExpiry == 0 {
		t.Error("RefreshTokenExpiry should have default value")
	}
	if svc.config.BcryptCost == 0 {
		t.Error("BcryptCost should have default value")
	}
}

func TestService_CreateUser(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	user, err := svc.CreateUser(ctx, "test@example.com", "Test User", "TestPassword123", store.UserRoleOperator)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if user.ID == "" {
		t.Error("User ID should not be empty")
	}

	if user.Email != "test@example.com" {
		t.Errorf("User email = %q, want %q", user.Email, "test@example.com")
	}

	if user.Name != "Test User" {
		t.Errorf("User name = %q, want %q", user.Name, "Test User")
	}

	if user.Role != store.UserRoleOperator {
		t.Errorf("User role = %q, want %q", user.Role, store.UserRoleOperator)
	}

	if user.PasswordHash == nil {
		t.Error("User password hash should not be nil")
	}
}

func TestService_CreateUser_DuplicateEmail(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	_, err := svc.CreateUser(ctx, "test@example.com", "Test User", "TestPassword123", store.UserRoleAdmin)
	if err != nil {
		t.Fatalf("First CreateUser failed: %v", err)
	}

	_, err = svc.CreateUser(ctx, "test@example.com", "Another User", "TestPassword456", store.UserRoleViewer)
	if err != ErrEmailExists {
		t.Errorf("Second CreateUser = %v, want %v", err, ErrEmailExists)
	}
}

func TestService_CreateUser_WeakPassword(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	_, err := svc.CreateUser(ctx, "test@example.com", "Test User", "weak", store.UserRoleAdmin)
	if err != ErrPasswordTooShort {
		t.Errorf("CreateUser with weak password = %v, want %v", err, ErrPasswordTooShort)
	}
}

func TestService_Login(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	// Create user first
	_, err := svc.CreateUser(ctx, "test@example.com", "Test User", "TestPassword123", store.UserRoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Login
	tokenPair, err := svc.Login(ctx, "test@example.com", "TestPassword123", "127.0.0.1", "Test-Agent")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if tokenPair.AccessToken == "" {
		t.Error("AccessToken should not be empty")
	}

	if tokenPair.RefreshToken == "" {
		t.Error("RefreshToken should not be empty")
	}

	if tokenPair.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want %q", tokenPair.TokenType, "Bearer")
	}

	if tokenPair.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt should be in the future")
	}
}

func TestService_Login_WrongPassword(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	// Create user first
	_, err := svc.CreateUser(ctx, "test@example.com", "Test User", "TestPassword123", store.UserRoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Try wrong password
	_, err = svc.Login(ctx, "test@example.com", "WrongPassword123", "127.0.0.1", "Test-Agent")
	if err != ErrInvalidCredentials {
		t.Errorf("Login with wrong password = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestService_Login_NonExistentUser(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	_, err := svc.Login(ctx, "nonexistent@example.com", "Password123", "127.0.0.1", "Test-Agent")
	if err != ErrInvalidCredentials {
		t.Errorf("Login for non-existent user = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestService_RefreshTokens(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	// Create user and login
	_, err := svc.CreateUser(ctx, "test@example.com", "Test User", "TestPassword123", store.UserRoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	tokenPair, err := svc.Login(ctx, "test@example.com", "TestPassword123", "127.0.0.1", "Test-Agent")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Refresh tokens
	newTokenPair, err := svc.RefreshTokens(ctx, tokenPair.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshTokens failed: %v", err)
	}

	if newTokenPair.AccessToken == "" {
		t.Error("New AccessToken should not be empty")
	}

	if newTokenPair.RefreshToken == "" {
		t.Error("New RefreshToken should not be empty")
	}

	// Refresh tokens must be different (they use session IDs)
	if newTokenPair.RefreshToken == tokenPair.RefreshToken {
		t.Error("New RefreshToken should be different from old one")
	}

	// New tokens should be valid
	_, err = svc.ValidateAccessToken(newTokenPair.AccessToken)
	if err != nil {
		t.Errorf("New access token should be valid: %v", err)
	}
}

func TestService_RefreshTokens_InvalidToken(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	_, err := svc.RefreshTokens(ctx, "invalid-token")
	if err == nil {
		t.Error("RefreshTokens should fail for invalid token")
	}
}

func TestService_Logout(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	// Create user and login
	_, err := svc.CreateUser(ctx, "test@example.com", "Test User", "TestPassword123", store.UserRoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	tokenPair, err := svc.Login(ctx, "test@example.com", "TestPassword123", "127.0.0.1", "Test-Agent")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Logout
	err = svc.Logout(ctx, tokenPair.RefreshToken)
	if err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	// Try to refresh with the logged out token
	_, err = svc.RefreshTokens(ctx, tokenPair.RefreshToken)
	if err != ErrSessionRevoked {
		t.Errorf("RefreshTokens after logout = %v, want %v", err, ErrSessionRevoked)
	}
}

func TestService_GetUserFromToken(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	// Create user and login
	createdUser, err := svc.CreateUser(ctx, "test@example.com", "Test User", "TestPassword123", store.UserRoleOperator)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	tokenPair, err := svc.Login(ctx, "test@example.com", "TestPassword123", "127.0.0.1", "Test-Agent")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Get user from token
	user, err := svc.GetUserFromToken(ctx, tokenPair.AccessToken)
	if err != nil {
		t.Fatalf("GetUserFromToken failed: %v", err)
	}

	if user.ID != createdUser.ID {
		t.Errorf("User ID = %q, want %q", user.ID, createdUser.ID)
	}

	if user.Email != createdUser.Email {
		t.Errorf("User email = %q, want %q", user.Email, createdUser.Email)
	}
}

func TestService_UpdatePassword(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	// Create user
	user, err := svc.CreateUser(ctx, "test@example.com", "Test User", "TestPassword123", store.UserRoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Login first time
	_, err = svc.Login(ctx, "test@example.com", "TestPassword123", "127.0.0.1", "Test-Agent")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Update password
	err = svc.UpdatePassword(ctx, user.ID, "NewPassword456")
	if err != nil {
		t.Fatalf("UpdatePassword failed: %v", err)
	}

	// Old password should not work
	_, err = svc.Login(ctx, "test@example.com", "TestPassword123", "127.0.0.1", "Test-Agent")
	if err != ErrInvalidCredentials {
		t.Errorf("Login with old password = %v, want %v", err, ErrInvalidCredentials)
	}

	// New password should work
	_, err = svc.Login(ctx, "test@example.com", "NewPassword456", "127.0.0.1", "Test-Agent")
	if err != nil {
		t.Errorf("Login with new password failed: %v", err)
	}
}

func TestService_UpdatePassword_NonExistentUser(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	err := svc.UpdatePassword(ctx, "nonexistent-id", "NewPassword456")
	if err != ErrUserNotFound {
		t.Errorf("UpdatePassword for non-existent user = %v, want %v", err, ErrUserNotFound)
	}
}

func TestService_GeneratePasswordResetToken(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	// Create user
	user, err := svc.CreateUser(ctx, "test@example.com", "Test User", "TestPassword123", store.UserRoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Generate reset token
	token, err := svc.GeneratePasswordResetToken(ctx, user.ID)
	if err != nil {
		t.Fatalf("GeneratePasswordResetToken failed: %v", err)
	}

	if token == "" {
		t.Error("Reset token should not be empty")
	}
}

func TestService_GeneratePasswordResetToken_NonExistentUser(t *testing.T) {
	svc, db := setupTestService(t)
	defer db.Close()

	ctx := context.Background()

	_, err := svc.GeneratePasswordResetToken(ctx, "nonexistent-id")
	if err != ErrUserNotFound {
		t.Errorf("GeneratePasswordResetToken for non-existent user = %v, want %v", err, ErrUserNotFound)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.AccessTokenExpiry != 15*time.Minute {
		t.Errorf("AccessTokenExpiry = %v, want %v", config.AccessTokenExpiry, 15*time.Minute)
	}

	if config.RefreshTokenExpiry != 7*24*time.Hour {
		t.Errorf("RefreshTokenExpiry = %v, want %v", config.RefreshTokenExpiry, 7*24*time.Hour)
	}

	if config.BcryptCost != DefaultBcryptCost {
		t.Errorf("BcryptCost = %d, want %d", config.BcryptCost, DefaultBcryptCost)
	}
}
