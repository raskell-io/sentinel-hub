package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/raskell-io/sentinel-hub/internal/store"
)

func TestGetUserFromContext_NoUser(t *testing.T) {
	ctx := context.Background()
	user := GetUserFromContext(ctx)
	if user != nil {
		t.Error("GetUserFromContext should return nil when no user in context")
	}
}

func TestGetUserFromContext_WithUser(t *testing.T) {
	ctx := context.Background()
	expectedUser := &store.User{
		ID:    "user-123",
		Email: "test@example.com",
		Role:  store.UserRoleAdmin,
	}

	ctx = context.WithValue(ctx, UserContextKey, expectedUser)
	user := GetUserFromContext(ctx)

	if user == nil {
		t.Fatal("GetUserFromContext should return user")
	}

	if user.ID != expectedUser.ID {
		t.Errorf("User ID = %q, want %q", user.ID, expectedUser.ID)
	}
}

func TestService_RequireAuth_NoHeader(t *testing.T) {
	db, err := store.New("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	svc := NewService(db, Config{
		JWTSecret:         "test-secret",
		AccessTokenExpiry: 15 * time.Minute,
	})

	handler := svc.RequireAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestService_RequireAuth_InvalidHeaderFormat(t *testing.T) {
	db, err := store.New("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	svc := NewService(db, Config{
		JWTSecret:         "test-secret",
		AccessTokenExpiry: 15 * time.Minute,
	})

	handler := svc.RequireAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "InvalidFormat")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestService_RequireAuth_InvalidToken(t *testing.T) {
	db, err := store.New("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	svc := NewService(db, Config{
		JWTSecret:         "test-secret",
		AccessTokenExpiry: 15 * time.Minute,
	})

	handler := svc.RequireAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestService_RequireAuth_ValidToken(t *testing.T) {
	db, err := store.New("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	svc := NewService(db, Config{
		JWTSecret:          "test-secret-key-for-testing-12345",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		BcryptCost:         4,
	})

	ctx := context.Background()

	// Create user and login
	_, err = svc.CreateUser(ctx, "test@example.com", "Test User", "TestPassword123", store.UserRoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	tokenPair, err := svc.Login(ctx, "test@example.com", "TestPassword123", "127.0.0.1", "Test-Agent")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	var userFromContext *store.User
	handler := svc.RequireAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userFromContext = GetUserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenPair.AccessToken)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	if userFromContext == nil {
		t.Fatal("User should be in context")
	}

	if userFromContext.Email != "test@example.com" {
		t.Errorf("User email = %q, want %q", userFromContext.Email, "test@example.com")
	}
}

func TestService_RequireRole_NoUser(t *testing.T) {
	db, err := store.New("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	svc := NewService(db, Config{JWTSecret: "test-secret"})

	handler := svc.RequireRole(store.UserRoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestService_RequireRole_AllowedRole(t *testing.T) {
	db, err := store.New("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	svc := NewService(db, Config{JWTSecret: "test-secret"})

	handler := svc.RequireRole(store.UserRoleAdmin, store.UserRoleOperator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test with admin role
	user := &store.User{
		ID:   "user-123",
		Role: store.UserRoleAdmin,
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), UserContextKey, user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code for admin = %d, want %d", rec.Code, http.StatusOK)
	}

	// Test with operator role
	user.Role = store.UserRoleOperator
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx = context.WithValue(req.Context(), UserContextKey, user)
	req = req.WithContext(ctx)
	rec = httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code for operator = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestService_RequireRole_DeniedRole(t *testing.T) {
	db, err := store.New("sqlite://:memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	svc := NewService(db, Config{JWTSecret: "test-secret"})

	handler := svc.RequireRole(store.UserRoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	user := &store.User{
		ID:   "user-123",
		Role: store.UserRoleViewer,
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), UserContextKey, user)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCanPerformAction_Admin(t *testing.T) {
	user := &store.User{Role: store.UserRoleAdmin}

	// Admin should be able to do everything
	actions := []string{"read", "create", "update", "delete"}
	resourceTypes := []string{"config", "deployment", "instance", "user"}

	for _, action := range actions {
		for _, rt := range resourceTypes {
			if !CanPerformAction(user, action, rt) {
				t.Errorf("Admin should be able to %s %s", action, rt)
			}
		}
	}
}

func TestCanPerformAction_Operator(t *testing.T) {
	user := &store.User{Role: store.UserRoleOperator}

	// Operator can read everything
	for _, rt := range []string{"config", "deployment", "instance", "user"} {
		if !CanPerformAction(user, "read", rt) {
			t.Errorf("Operator should be able to read %s", rt)
		}
	}

	// Operator can create/update configs, deployments, instances
	for _, action := range []string{"create", "update"} {
		for _, rt := range []string{"config", "deployment", "instance"} {
			if !CanPerformAction(user, action, rt) {
				t.Errorf("Operator should be able to %s %s", action, rt)
			}
		}
	}

	// Operator cannot delete
	if CanPerformAction(user, "delete", "config") {
		t.Error("Operator should not be able to delete")
	}

	// Operator cannot manage users
	if CanPerformAction(user, "create", "user") {
		t.Error("Operator should not be able to create users")
	}
}

func TestCanPerformAction_Viewer(t *testing.T) {
	user := &store.User{Role: store.UserRoleViewer}

	// Viewer can only read
	if !CanPerformAction(user, "read", "config") {
		t.Error("Viewer should be able to read")
	}

	// Viewer cannot create, update, or delete
	for _, action := range []string{"create", "update", "delete"} {
		if CanPerformAction(user, action, "config") {
			t.Errorf("Viewer should not be able to %s", action)
		}
	}
}

func TestCanPerformAction_NilUser(t *testing.T) {
	if CanPerformAction(nil, "read", "config") {
		t.Error("Nil user should not be able to perform any action")
	}
}

func TestRoleStrings(t *testing.T) {
	roles := []store.UserRole{store.UserRoleAdmin, store.UserRoleOperator}
	strs := roleStrings(roles)

	if len(strs) != 2 {
		t.Errorf("len(roleStrings) = %d, want 2", len(strs))
	}

	if strs[0] != "admin" {
		t.Errorf("strs[0] = %q, want %q", strs[0], "admin")
	}

	if strs[1] != "operator" {
		t.Errorf("strs[1] = %q, want %q", strs[1], "operator")
	}
}
