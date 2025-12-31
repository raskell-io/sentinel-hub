package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/raskell-io/sentinel-hub/internal/auth"
	"github.com/rs/zerolog/log"
)

// AuthHandler provides HTTP handlers for authentication endpoints.
type AuthHandler struct {
	authService *auth.Service
}

// NewAuthHandler creates a new AuthHandler instance.
func NewAuthHandler(authService *auth.Service) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// LoginRequest represents the login request body.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse represents the login response.
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    string `json:"expires_at"`
	TokenType    string `json:"token_type"`
	User         struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	} `json:"user"`
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "email is required")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "password is required")
		return
	}

	// Get client info
	ipAddress := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ipAddress = strings.Split(forwarded, ",")[0]
	}
	userAgent := r.Header.Get("User-Agent")

	// Authenticate
	tokenPair, err := h.authService.Login(r.Context(), req.Email, req.Password, ipAddress, userAgent)
	if err != nil {
		if err == auth.ErrInvalidCredentials {
			writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
			return
		}
		log.Error().Err(err).Msg("Login failed")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Login failed")
		return
	}

	// Get user info for response
	user, err := h.authService.GetUserFromToken(r.Context(), tokenPair.AccessToken)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user after login")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Login failed")
		return
	}

	resp := LoginResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.ExpiresAt.Format("2006-01-02T15:04:05Z"),
		TokenType:    tokenPair.TokenType,
	}
	resp.User.ID = user.ID
	resp.User.Email = user.Email
	resp.User.Name = user.Name
	resp.User.Role = string(user.Role)

	writeJSON(w, http.StatusOK, resp)
}

// LogoutRequest represents the logout request body.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req LogoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "refresh_token is required")
		return
	}

	if err := h.authService.Logout(r.Context(), req.RefreshToken); err != nil {
		log.Debug().Err(err).Msg("Logout failed")
		// Don't return error - logout should be idempotent
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// RefreshRequest represents the refresh token request body.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshResponse represents the refresh token response.
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    string `json:"expires_at"`
	TokenType    string `json:"token_type"`
}

// Refresh handles POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "refresh_token is required")
		return
	}

	tokenPair, err := h.authService.RefreshTokens(r.Context(), req.RefreshToken)
	if err != nil {
		switch err {
		case auth.ErrInvalidToken, auth.ErrInvalidTokenType:
			writeError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid refresh token")
		case auth.ErrTokenExpired, auth.ErrSessionExpired:
			writeError(w, http.StatusUnauthorized, "TOKEN_EXPIRED", "Refresh token has expired")
		case auth.ErrSessionRevoked:
			writeError(w, http.StatusUnauthorized, "SESSION_REVOKED", "Session has been revoked")
		case auth.ErrUserNotFound:
			writeError(w, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
		default:
			log.Error().Err(err).Msg("Token refresh failed")
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Token refresh failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, RefreshResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.ExpiresAt.Format("2006-01-02T15:04:05Z"),
		TokenType:    tokenPair.TokenType,
	})
}

// GetCurrentUser handles GET /api/v1/auth/me
func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Not authenticated")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":            user.ID,
		"email":         user.Email,
		"name":          user.Name,
		"role":          user.Role,
		"created_at":    user.CreatedAt,
		"last_login_at": user.LastLoginAt,
	})
}
