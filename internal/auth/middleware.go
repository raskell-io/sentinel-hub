package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/raskell-io/sentinel-hub/internal/store"
	"github.com/rs/zerolog/log"
)

// contextKey is used for context values.
type contextKey string

const (
	// UserContextKey is the key for storing user in context.
	UserContextKey contextKey = "user"
)

// GetUserFromContext retrieves the authenticated user from context.
func GetUserFromContext(ctx context.Context) *store.User {
	if user, ok := ctx.Value(UserContextKey).(*store.User); ok {
		return user
	}
	return nil
}

// RequireAuth returns middleware that requires valid authentication.
func (s *Service) RequireAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization header required")
				return
			}

			// Check for Bearer prefix
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid authorization header format")
				return
			}

			tokenString := parts[1]

			// Validate token and get user
			user, err := s.GetUserFromToken(r.Context(), tokenString)
			if err != nil {
				log.Debug().Err(err).Msg("Token validation failed")

				switch err {
				case ErrTokenExpired:
					writeAuthError(w, http.StatusUnauthorized, "TOKEN_EXPIRED", "Token has expired")
				case ErrInvalidToken, ErrInvalidTokenType:
					writeAuthError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid token")
				case ErrUserNotFound:
					writeAuthError(w, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
				default:
					writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication failed")
				}
				return
			}

			// Add user to context
			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns middleware that requires specific roles.
func (s *Service) RequireRole(allowedRoles ...store.UserRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r.Context())
			if user == nil {
				// This should not happen if RequireAuth middleware is used first
				writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Not authenticated")
				return
			}

			// Check if user has required role
			allowed := false
			for _, role := range allowedRoles {
				if user.Role == role {
					allowed = true
					break
				}
			}

			if !allowed {
				log.Debug().
					Str("user_id", user.ID).
					Str("role", string(user.Role)).
					Strs("required_roles", roleStrings(allowedRoles)).
					Msg("Access denied - insufficient permissions")
				writeAuthError(w, http.StatusForbidden, "FORBIDDEN", "Insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// writeAuthError writes an authentication/authorization error response.
func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Simple JSON encoding to avoid import cycle with api package
	w.Write([]byte(`{"error":"` + message + `","code":"` + code + `"}`))
}

// roleStrings converts roles to strings for logging.
func roleStrings(roles []store.UserRole) []string {
	result := make([]string, len(roles))
	for i, r := range roles {
		result[i] = string(r)
	}
	return result
}

// CanPerformAction checks if a user can perform an action on a resource.
// This can be extended for more granular permissions.
func CanPerformAction(user *store.User, action, resourceType string) bool {
	if user == nil {
		return false
	}

	switch user.Role {
	case store.UserRoleAdmin:
		// Admins can do everything
		return true

	case store.UserRoleOperator:
		// Operators can read everything and write configs/deployments
		switch action {
		case "read":
			return true
		case "create", "update":
			switch resourceType {
			case "config", "deployment", "instance":
				return true
			}
		}
		return false

	case store.UserRoleViewer:
		// Viewers can only read
		return action == "read"

	default:
		return false
	}
}
