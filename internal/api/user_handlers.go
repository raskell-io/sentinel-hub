package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/raskell-io/sentinel-hub/internal/auth"
	"github.com/raskell-io/sentinel-hub/internal/store"
	"github.com/rs/zerolog/log"
)

// UserHandler provides HTTP handlers for user management endpoints.
type UserHandler struct {
	store       *store.Store
	authService *auth.Service
}

// NewUserHandler creates a new UserHandler instance.
func NewUserHandler(s *store.Store, authService *auth.Service) *UserHandler {
	return &UserHandler{store: s, authService: authService}
}

// auditLog creates an audit log entry for user-related operations.
func (h *UserHandler) auditLog(r *http.Request, action, resourceType, resourceID string, details interface{}) {
	user := auth.GetUserFromContext(r.Context())
	var userIDPtr *string
	if user != nil {
		userIDPtr = &user.ID
	}

	var detailsJSON json.RawMessage
	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			detailsJSON = b
		}
	}

	auditLog := &store.AuditLog{
		ID:           uuid.New().String(),
		UserID:       userIDPtr,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   &resourceID,
		Details:      detailsJSON,
	}

	if err := h.store.CreateAuditLog(r.Context(), auditLog); err != nil {
		log.Warn().Err(err).
			Str("action", action).
			Str("resource_type", resourceType).
			Str("resource_id", resourceID).
			Msg("Failed to create audit log")
	}
}

// ListUsersResponse represents the response for listing users.
type ListUsersResponse struct {
	Users []UserResponse `json:"users"`
	Total int            `json:"total"`
}

// UserResponse represents a user in API responses (excludes sensitive data).
type UserResponse struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	Name        string  `json:"name"`
	Role        string  `json:"role"`
	CreatedAt   string  `json:"created_at"`
	LastLoginAt *string `json:"last_login_at,omitempty"`
}

func userToResponse(u store.User) UserResponse {
	resp := UserResponse{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Role:      string(u.Role),
		CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if u.LastLoginAt != nil {
		t := u.LastLoginAt.Format("2006-01-02T15:04:05Z")
		resp.LastLoginAt = &t
	}
	return resp
}

// ListUsers handles GET /api/v1/users
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opts := store.ListUsersOptions{}
	if role := r.URL.Query().Get("role"); role != "" {
		opts.Role = store.UserRole(role)
	}

	users, err := h.store.ListUsers(ctx, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list users")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list users")
		return
	}

	if users == nil {
		users = []store.User{}
	}

	resp := ListUsersResponse{
		Users: make([]UserResponse, len(users)),
		Total: len(users),
	}
	for i, u := range users {
		resp.Users[i] = userToResponse(u)
	}

	writeJSON(w, http.StatusOK, resp)
}

// CreateUserRequest represents the request body for creating a user.
type CreateUserRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// CreateUser handles POST /api/v1/users
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	// Validate required fields
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "email is required")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "password is required")
		return
	}

	// Default role to viewer
	role := store.UserRoleViewer
	if req.Role != "" {
		role = store.UserRole(req.Role)
		// Validate role
		switch role {
		case store.UserRoleAdmin, store.UserRoleOperator, store.UserRoleViewer:
			// valid
		default:
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid role (must be admin, operator, or viewer)")
			return
		}
	}

	user, err := h.authService.CreateUser(r.Context(), req.Email, req.Name, req.Password, role)
	if err != nil {
		switch err {
		case auth.ErrEmailExists:
			writeError(w, http.StatusConflict, "ALREADY_EXISTS", "User with this email already exists")
		case auth.ErrPasswordTooShort:
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Password must be at least 8 characters")
		case auth.ErrPasswordNoUppercase:
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Password must contain at least one uppercase letter")
		case auth.ErrPasswordNoLowercase:
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Password must contain at least one lowercase letter")
		case auth.ErrPasswordNoDigit:
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Password must contain at least one digit")
		default:
			log.Error().Err(err).Msg("Failed to create user")
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create user")
		}
		return
	}

	h.auditLog(r, "create", "user", user.ID, map[string]string{"email": user.Email, "role": string(user.Role)})
	writeJSON(w, http.StatusCreated, userToResponse(*user))
}

// GetUser handles GET /api/v1/users/{id}
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	user, err := h.store.GetUser(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to get user")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get user")
		return
	}

	if user == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
		return
	}

	writeJSON(w, http.StatusOK, userToResponse(*user))
}

// UpdateUserRequest represents the request body for updating a user.
type UpdateUserRequest struct {
	Email *string `json:"email,omitempty"`
	Name  *string `json:"name,omitempty"`
	Role  *string `json:"role,omitempty"`
}

// UpdateUser handles PUT /api/v1/users/{id}
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	user, err := h.store.GetUser(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to get user")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update user")
		return
	}

	if user == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	// Apply updates
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Name != nil {
		user.Name = *req.Name
	}
	if req.Role != nil {
		role := store.UserRole(*req.Role)
		switch role {
		case store.UserRoleAdmin, store.UserRoleOperator, store.UserRoleViewer:
			user.Role = role
		default:
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid role (must be admin, operator, or viewer)")
			return
		}
	}

	if err := h.store.UpdateUser(ctx, user); err != nil {
		if err.Error() == "user with this email already exists" {
			writeError(w, http.StatusConflict, "ALREADY_EXISTS", "User with this email already exists")
			return
		}
		log.Error().Err(err).Str("id", id).Msg("Failed to update user")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update user")
		return
	}

	h.auditLog(r, "update", "user", user.ID, map[string]string{"email": user.Email})
	writeJSON(w, http.StatusOK, userToResponse(*user))
}

// DeleteUser handles DELETE /api/v1/users/{id}
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Prevent self-deletion
	currentUser := auth.GetUserFromContext(ctx)
	if currentUser != nil && currentUser.ID == id {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Cannot delete your own account")
		return
	}

	if err := h.store.DeleteUser(ctx, id); err != nil {
		if err.Error() == "user not found" {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
			return
		}
		log.Error().Err(err).Str("id", id).Msg("Failed to delete user")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete user")
		return
	}

	// Revoke all sessions for deleted user
	if err := h.store.RevokeAllUserSessions(ctx, id); err != nil {
		log.Warn().Err(err).Str("id", id).Msg("Failed to revoke sessions for deleted user")
	}

	h.auditLog(r, "delete", "user", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// ResetPasswordRequest represents the password reset request body.
type ResetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// ResetPasswordResponse represents the password reset response.
type ResetPasswordResponse struct {
	ResetToken string `json:"reset_token,omitempty"`
	Message    string `json:"message"`
}

// ResetPassword handles POST /api/v1/users/{id}/reset-password
func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// If no body, generate a token
		token, err := h.authService.GeneratePasswordResetToken(ctx, id)
		if err != nil {
			if err == auth.ErrUserNotFound {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
				return
			}
			log.Error().Err(err).Str("id", id).Msg("Failed to generate reset token")
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate reset token")
			return
		}

		h.auditLog(r, "reset_password_token", "user", id, nil)
		writeJSON(w, http.StatusOK, ResetPasswordResponse{
			ResetToken: token,
			Message:    "Share this token with the user to reset their password",
		})
		return
	}

	// If new_password is provided, update it directly
	if req.NewPassword != "" {
		if err := h.authService.UpdatePassword(ctx, id, req.NewPassword); err != nil {
			switch err {
			case auth.ErrUserNotFound:
				writeError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
			case auth.ErrPasswordTooShort:
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Password must be at least 8 characters")
			case auth.ErrPasswordNoUppercase:
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Password must contain at least one uppercase letter")
			case auth.ErrPasswordNoLowercase:
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Password must contain at least one lowercase letter")
			case auth.ErrPasswordNoDigit:
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Password must contain at least one digit")
			default:
				log.Error().Err(err).Str("id", id).Msg("Failed to reset password")
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to reset password")
			}
			return
		}

		h.auditLog(r, "reset_password", "user", id, nil)
		writeJSON(w, http.StatusOK, ResetPasswordResponse{
			Message: "Password has been reset successfully",
		})
		return
	}

	writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "new_password is required")
}

// ListAuditLogsResponse represents the response for listing audit logs.
type ListAuditLogsResponse struct {
	Logs  []store.AuditLog `json:"logs"`
	Total int              `json:"total"`
}

// ListAuditLogs handles GET /api/v1/audit-logs
func (h *UserHandler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opts := store.ListAuditLogsOptions{
		Limit: 100, // Default limit
	}

	if userID := r.URL.Query().Get("user_id"); userID != "" {
		opts.UserID = userID
	}
	if action := r.URL.Query().Get("action"); action != "" {
		opts.Action = action
	}
	if resourceType := r.URL.Query().Get("resource_type"); resourceType != "" {
		opts.ResourceType = resourceType
	}
	if resourceID := r.URL.Query().Get("resource_id"); resourceID != "" {
		opts.ResourceID = resourceID
	}

	logs, err := h.store.ListAuditLogs(ctx, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list audit logs")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list audit logs")
		return
	}

	if logs == nil {
		logs = []store.AuditLog{}
	}

	writeJSON(w, http.StatusOK, ListAuditLogsResponse{
		Logs:  logs,
		Total: len(logs),
	})
}
