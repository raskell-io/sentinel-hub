package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/raskell-io/sentinel-hub/internal/auth"
	"github.com/raskell-io/sentinel-hub/internal/fleet"
	"github.com/raskell-io/sentinel-hub/internal/store"
	"github.com/rs/zerolog/log"
)

// Handler provides HTTP handlers for the Hub API.
type Handler struct {
	store        *store.Store
	orchestrator *fleet.Orchestrator
}

// NewHandler creates a new Handler instance.
func NewHandler(s *store.Store, o *fleet.Orchestrator) *Handler {
	return &Handler{store: s, orchestrator: o}
}

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Error().Err(err).Msg("Failed to encode JSON response")
	}
}

// writeError writes an error response.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Error: message, Code: code})
}

// auditLog creates an audit log entry for write operations.
func (h *Handler) auditLog(r *http.Request, action, resourceType, resourceID string, details interface{}) {
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

// ============================================
// Instance Handlers
// ============================================

// ListInstancesResponse represents the response for listing instances.
type ListInstancesResponse struct {
	Instances []store.Instance `json:"instances"`
	Total     int              `json:"total"`
}

// ListInstances handles GET /api/v1/instances
func (h *Handler) ListInstances(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	opts := store.ListInstancesOptions{}
	if status := r.URL.Query().Get("status"); status != "" {
		opts.Status = store.InstanceStatus(status)
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			opts.Limit = l
		}
	}
	if offset := r.URL.Query().Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil {
			opts.Offset = o
		}
	}

	instances, err := h.store.ListInstances(ctx, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list instances")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list instances")
		return
	}

	if instances == nil {
		instances = []store.Instance{}
	}

	writeJSON(w, http.StatusOK, ListInstancesResponse{
		Instances: instances,
		Total:     len(instances),
	})
}

// CreateInstanceRequest represents the request body for creating an instance.
type CreateInstanceRequest struct {
	Name            string            `json:"name"`
	Hostname        string            `json:"hostname"`
	AgentVersion    string            `json:"agent_version"`
	SentinelVersion string            `json:"sentinel_version"`
	Labels          map[string]string `json:"labels,omitempty"`
	Capabilities    []string          `json:"capabilities,omitempty"`
}

// CreateInstance handles POST /api/v1/instances
func (h *Handler) CreateInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	// Validate required fields
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}
	if req.Hostname == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "hostname is required")
		return
	}

	// Check if name already exists
	existing, err := h.store.GetInstanceByName(ctx, req.Name)
	if err != nil {
		log.Error().Err(err).Msg("Failed to check existing instance")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create instance")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "ALREADY_EXISTS", "Instance with this name already exists")
		return
	}

	inst := &store.Instance{
		ID:              uuid.New().String(),
		Name:            req.Name,
		Hostname:        req.Hostname,
		AgentVersion:    req.AgentVersion,
		SentinelVersion: req.SentinelVersion,
		Status:          store.InstanceStatusUnknown,
		Labels:          req.Labels,
		Capabilities:    req.Capabilities,
	}

	if err := h.store.CreateInstance(ctx, inst); err != nil {
		log.Error().Err(err).Msg("Failed to create instance")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create instance")
		return
	}

	h.auditLog(r, "create", "instance", inst.ID, map[string]string{"name": inst.Name})
	writeJSON(w, http.StatusCreated, inst)
}

// GetInstance handles GET /api/v1/instances/{id}
func (h *Handler) GetInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	inst, err := h.store.GetInstance(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to get instance")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get instance")
		return
	}

	if inst == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Instance not found")
		return
	}

	writeJSON(w, http.StatusOK, inst)
}

// UpdateInstanceRequest represents the request body for updating an instance.
type UpdateInstanceRequest struct {
	Name            *string            `json:"name,omitempty"`
	Hostname        *string            `json:"hostname,omitempty"`
	AgentVersion    *string            `json:"agent_version,omitempty"`
	SentinelVersion *string            `json:"sentinel_version,omitempty"`
	Status          *string            `json:"status,omitempty"`
	Labels          *map[string]string `json:"labels,omitempty"`
}

// UpdateInstance handles PUT /api/v1/instances/{id}
func (h *Handler) UpdateInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	inst, err := h.store.GetInstance(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to get instance")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update instance")
		return
	}

	if inst == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Instance not found")
		return
	}

	var req UpdateInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	// Apply updates
	if req.Name != nil {
		inst.Name = *req.Name
	}
	if req.Hostname != nil {
		inst.Hostname = *req.Hostname
	}
	if req.AgentVersion != nil {
		inst.AgentVersion = *req.AgentVersion
	}
	if req.SentinelVersion != nil {
		inst.SentinelVersion = *req.SentinelVersion
	}
	if req.Status != nil {
		inst.Status = store.InstanceStatus(*req.Status)
	}
	if req.Labels != nil {
		inst.Labels = *req.Labels
	}

	if err := h.store.UpdateInstance(ctx, inst); err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to update instance")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update instance")
		return
	}

	h.auditLog(r, "update", "instance", inst.ID, map[string]string{"name": inst.Name})
	writeJSON(w, http.StatusOK, inst)
}

// DeleteInstance handles DELETE /api/v1/instances/{id}
func (h *Handler) DeleteInstance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.store.DeleteInstance(ctx, id); err != nil {
		if err.Error() == "instance not found" {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Instance not found")
			return
		}
		log.Error().Err(err).Str("id", id).Msg("Failed to delete instance")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete instance")
		return
	}

	h.auditLog(r, "delete", "instance", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// ============================================
// Config Handlers
// ============================================

// ListConfigsResponse represents the response for listing configs.
type ListConfigsResponse struct {
	Configs []store.Config `json:"configs"`
	Total   int            `json:"total"`
}

// ListConfigs handles GET /api/v1/configs
func (h *Handler) ListConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opts := store.ListConfigsOptions{}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			opts.Limit = l
		}
	}
	if offset := r.URL.Query().Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil {
			opts.Offset = o
		}
	}

	configs, err := h.store.ListConfigs(ctx, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list configs")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list configs")
		return
	}

	if configs == nil {
		configs = []store.Config{}
	}

	writeJSON(w, http.StatusOK, ListConfigsResponse{
		Configs: configs,
		Total:   len(configs),
	})
}

// CreateConfigRequest represents the request body for creating a config.
type CreateConfigRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Content     string  `json:"content"` // KDL configuration content
}

// CreateConfigResponse includes the config and its initial version.
type CreateConfigResponse struct {
	Config  store.Config        `json:"config"`
	Version store.ConfigVersion `json:"version"`
}

// CreateConfig handles POST /api/v1/configs
func (h *Handler) CreateConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "content is required")
		return
	}

	// TODO: Validate KDL syntax

	cfg := &store.Config{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
	}

	if err := h.store.CreateConfig(ctx, cfg); err != nil {
		log.Error().Err(err).Msg("Failed to create config")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create config")
		return
	}

	// Create initial version
	hash := sha256.Sum256([]byte(req.Content))
	ver := &store.ConfigVersion{
		ID:          uuid.New().String(),
		ConfigID:    cfg.ID,
		Version:     1,
		Content:     req.Content,
		ContentHash: hex.EncodeToString(hash[:]),
	}

	if err := h.store.CreateConfigVersion(ctx, ver); err != nil {
		log.Error().Err(err).Msg("Failed to create config version")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create config version")
		return
	}

	h.auditLog(r, "create", "config", cfg.ID, map[string]string{"name": cfg.Name})
	writeJSON(w, http.StatusCreated, CreateConfigResponse{
		Config:  *cfg,
		Version: *ver,
	})
}

// GetConfigResponse includes the config and its current version content.
type GetConfigResponse struct {
	Config         store.Config        `json:"config"`
	CurrentVersion *store.ConfigVersion `json:"current_version,omitempty"`
}

// GetConfig handles GET /api/v1/configs/{id}
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	cfg, err := h.store.GetConfig(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to get config")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get config")
		return
	}

	if cfg == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Config not found")
		return
	}

	// Get current version
	ver, err := h.store.GetLatestConfigVersion(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to get config version")
	}

	writeJSON(w, http.StatusOK, GetConfigResponse{
		Config:         *cfg,
		CurrentVersion: ver,
	})
}

// UpdateConfigRequest represents the request body for updating a config.
type UpdateConfigRequest struct {
	Name          *string `json:"name,omitempty"`
	Description   *string `json:"description,omitempty"`
	Content       *string `json:"content,omitempty"` // If provided, creates a new version
	ChangeSummary *string `json:"change_summary,omitempty"`
}

// UpdateConfig handles PUT /api/v1/configs/{id}
func (h *Handler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	cfg, err := h.store.GetConfig(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to get config")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update config")
		return
	}

	if cfg == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Config not found")
		return
	}

	var req UpdateConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	// Apply metadata updates
	if req.Name != nil {
		cfg.Name = *req.Name
	}
	if req.Description != nil {
		cfg.Description = req.Description
	}

	// If content is provided, create a new version
	var newVersion *store.ConfigVersion
	if req.Content != nil {
		// TODO: Validate KDL syntax

		hash := sha256.Sum256([]byte(*req.Content))
		newVersion = &store.ConfigVersion{
			ID:            uuid.New().String(),
			ConfigID:      cfg.ID,
			Version:       cfg.CurrentVersion + 1,
			Content:       *req.Content,
			ContentHash:   hex.EncodeToString(hash[:]),
			ChangeSummary: req.ChangeSummary,
		}

		if err := h.store.CreateConfigVersion(ctx, newVersion); err != nil {
			log.Error().Err(err).Msg("Failed to create config version")
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create config version")
			return
		}

		cfg.CurrentVersion = newVersion.Version
	}

	if err := h.store.UpdateConfig(ctx, cfg); err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to update config")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update config")
		return
	}

	h.auditLog(r, "update", "config", cfg.ID, map[string]interface{}{
		"name":        cfg.Name,
		"new_version": newVersion != nil,
	})
	writeJSON(w, http.StatusOK, GetConfigResponse{
		Config:         *cfg,
		CurrentVersion: newVersion,
	})
}

// DeleteConfig handles DELETE /api/v1/configs/{id}
func (h *Handler) DeleteConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.store.DeleteConfig(ctx, id); err != nil {
		if err.Error() == "config not found" {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Config not found")
			return
		}
		log.Error().Err(err).Str("id", id).Msg("Failed to delete config")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete config")
		return
	}

	h.auditLog(r, "delete", "config", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// ListConfigVersionsResponse represents the response for listing config versions.
type ListConfigVersionsResponse struct {
	Versions []store.ConfigVersion `json:"versions"`
	Total    int                   `json:"total"`
}

// ListConfigVersions handles GET /api/v1/configs/{id}/versions
func (h *Handler) ListConfigVersions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Check config exists
	cfg, err := h.store.GetConfig(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to get config")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get config")
		return
	}
	if cfg == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Config not found")
		return
	}

	versions, err := h.store.ListConfigVersions(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to list config versions")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list config versions")
		return
	}

	if versions == nil {
		versions = []store.ConfigVersion{}
	}

	writeJSON(w, http.StatusOK, ListConfigVersionsResponse{
		Versions: versions,
		Total:    len(versions),
	})
}

// RollbackConfigRequest represents the request body for rolling back a config.
type RollbackConfigRequest struct {
	Version int `json:"version"`
}

// RollbackConfig handles POST /api/v1/configs/{id}/rollback
func (h *Handler) RollbackConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	cfg, err := h.store.GetConfig(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to get config")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to rollback config")
		return
	}
	if cfg == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Config not found")
		return
	}

	var req RollbackConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	if req.Version <= 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "version must be positive")
		return
	}

	// Get the target version
	targetVersion, err := h.store.GetConfigVersion(ctx, id, req.Version)
	if err != nil {
		log.Error().Err(err).Str("id", id).Int("version", req.Version).Msg("Failed to get config version")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to rollback config")
		return
	}
	if targetVersion == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Target version not found")
		return
	}

	// Create a new version with the content from the target version
	hash := sha256.Sum256([]byte(targetVersion.Content))
	summary := "Rollback to version " + strconv.Itoa(req.Version)
	newVersion := &store.ConfigVersion{
		ID:            uuid.New().String(),
		ConfigID:      cfg.ID,
		Version:       cfg.CurrentVersion + 1,
		Content:       targetVersion.Content,
		ContentHash:   hex.EncodeToString(hash[:]),
		ChangeSummary: &summary,
	}

	if err := h.store.CreateConfigVersion(ctx, newVersion); err != nil {
		log.Error().Err(err).Msg("Failed to create rollback version")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to rollback config")
		return
	}

	cfg.CurrentVersion = newVersion.Version
	if err := h.store.UpdateConfig(ctx, cfg); err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to update config after rollback")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to rollback config")
		return
	}

	h.auditLog(r, "rollback", "config", cfg.ID, map[string]interface{}{
		"from_version": cfg.CurrentVersion - 1,
		"to_version":   req.Version,
		"new_version":  newVersion.Version,
	})
	writeJSON(w, http.StatusOK, GetConfigResponse{
		Config:         *cfg,
		CurrentVersion: newVersion,
	})
}

// ============================================
// Deployment Handlers
// ============================================

// ListDeploymentsResponse represents the response for listing deployments.
type ListDeploymentsResponse struct {
	Deployments []store.Deployment `json:"deployments"`
	Total       int                `json:"total"`
}

// ListDeployments handles GET /api/v1/deployments
func (h *Handler) ListDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opts := store.ListDeploymentsOptions{}
	if status := r.URL.Query().Get("status"); status != "" {
		opts.Status = store.DeploymentStatus(status)
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			opts.Limit = l
		}
	}
	if offset := r.URL.Query().Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil {
			opts.Offset = o
		}
	}

	deployments, err := h.store.ListDeployments(ctx, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list deployments")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list deployments")
		return
	}

	if deployments == nil {
		deployments = []store.Deployment{}
	}

	writeJSON(w, http.StatusOK, ListDeploymentsResponse{
		Deployments: deployments,
		Total:       len(deployments),
	})
}

// CreateDeploymentRequest represents the request body for creating a deployment.
type CreateDeploymentRequest struct {
	ConfigID        string            `json:"config_id"`
	ConfigVersion   *int              `json:"config_version,omitempty"` // If not specified, uses current version
	TargetInstances []string          `json:"target_instances"`         // Instance IDs (optional if target_labels specified)
	TargetLabels    map[string]string `json:"target_labels,omitempty"`  // Label selector (alternative to instance IDs)
	Strategy        string            `json:"strategy,omitempty"`       // all_at_once, rolling, canary
	BatchSize       int               `json:"batch_size,omitempty"`
}

// CreateDeployment handles POST /api/v1/deployments
func (h *Handler) CreateDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateDeploymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	if req.ConfigID == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "config_id is required")
		return
	}
	if len(req.TargetInstances) == 0 && len(req.TargetLabels) == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "target_instances or target_labels is required")
		return
	}

	// Determine config version
	configVersion := 0
	if req.ConfigVersion != nil {
		configVersion = *req.ConfigVersion
	}

	// Default strategy
	strategy := store.DeploymentStrategyRolling
	if req.Strategy != "" {
		strategy = store.DeploymentStrategy(req.Strategy)
	}

	// Use orchestrator to create and start deployment
	dep, err := h.orchestrator.CreateDeployment(ctx, fleet.CreateDeploymentRequest{
		ConfigID:        req.ConfigID,
		ConfigVersion:   configVersion,
		TargetInstances: req.TargetInstances,
		TargetLabels:    req.TargetLabels,
		Strategy:        strategy,
		BatchSize:       req.BatchSize,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to create deployment")
		writeError(w, http.StatusBadRequest, "DEPLOYMENT_ERROR", err.Error())
		return
	}

	h.auditLog(r, "create", "deployment", dep.ID, map[string]interface{}{
		"config_id":        req.ConfigID,
		"strategy":         string(strategy),
		"target_instances": len(dep.TargetInstances),
	})
	writeJSON(w, http.StatusCreated, dep)
}

// DeploymentStatusResponse includes deployment info and per-instance results.
type DeploymentStatusResponse struct {
	Deployment      *store.Deployment                        `json:"deployment"`
	InstanceResults map[string]fleet.InstanceDeploymentResult `json:"instance_results,omitempty"`
}

// GetDeployment handles GET /api/v1/deployments/{id}
func (h *Handler) GetDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Get detailed status from orchestrator (includes per-instance results for active deployments)
	status, err := h.orchestrator.GetDeploymentStatus(ctx, id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to get deployment")
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, DeploymentStatusResponse{
		Deployment:      status.Deployment,
		InstanceResults: status.InstanceResults,
	})
}

// CancelDeployment handles POST /api/v1/deployments/{id}/cancel
func (h *Handler) CancelDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Use orchestrator to cancel the deployment (stops running tasks)
	if err := h.orchestrator.CancelDeployment(ctx, id); err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to cancel deployment")
		writeError(w, http.StatusBadRequest, "CANCEL_ERROR", err.Error())
		return
	}

	h.auditLog(r, "cancel", "deployment", id, nil)

	// Get updated status
	status, err := h.orchestrator.GetDeploymentStatus(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
		return
	}

	writeJSON(w, http.StatusOK, status.Deployment)
}
