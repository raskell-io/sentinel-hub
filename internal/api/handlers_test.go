package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/raskell-io/sentinel-hub/internal/store"
)

// setupTestStore creates a temporary SQLite store for testing.
func setupTestStore(t *testing.T) *store.Store {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "api-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	s, err := store.New(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create test store: %v", err)
	}

	t.Cleanup(func() {
		s.Close()
		os.Remove(tmpFile.Name())
		os.Remove(tmpFile.Name() + "-shm")
		os.Remove(tmpFile.Name() + "-wal")
	})
	return s
}

// setupTestHandler creates a Handler with a test store (no orchestrator).
func setupTestHandler(t *testing.T) (*Handler, *store.Store) {
	t.Helper()
	s := setupTestStore(t)
	h := NewHandler(s, nil)
	return h, s
}

// chiContext adds chi URL params to a request context.
func chiContext(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// ============================================
// Handler Tests
// ============================================

func TestNewHandler(t *testing.T) {
	s := setupTestStore(t)
	h := NewHandler(s, nil)

	if h == nil {
		t.Fatal("NewHandler returned nil")
	}
	if h.store != s {
		t.Error("store not set correctly")
	}
}

// ============================================
// Instance Handler Tests
// ============================================

func TestHandler_ListInstances(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	// Create some instances
	for i := 0; i < 3; i++ {
		s.CreateInstance(ctx, &store.Instance{
			Name:     "inst-" + string(rune('a'+i)),
			Hostname: "host-" + string(rune('a'+i)),
			Status:   store.InstanceStatusOnline,
		})
	}

	req := httptest.NewRequest("GET", "/api/v1/instances", nil)
	w := httptest.NewRecorder()

	h.ListInstances(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp ListInstancesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Total != 3 {
		t.Errorf("Total = %d, want 3", resp.Total)
	}
	if len(resp.Instances) != 3 {
		t.Errorf("Instances count = %d, want 3", len(resp.Instances))
	}
}

func TestHandler_ListInstances_WithFilters(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	s.CreateInstance(ctx, &store.Instance{Name: "inst-1", Hostname: "host-1", Status: store.InstanceStatusOnline})
	s.CreateInstance(ctx, &store.Instance{Name: "inst-2", Hostname: "host-2", Status: store.InstanceStatusOffline})
	s.CreateInstance(ctx, &store.Instance{Name: "inst-3", Hostname: "host-3", Status: store.InstanceStatusOnline})

	// Filter by status
	req := httptest.NewRequest("GET", "/api/v1/instances?status=online", nil)
	w := httptest.NewRecorder()
	h.ListInstances(w, req)

	var resp ListInstancesResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Total != 2 {
		t.Errorf("Total with status filter = %d, want 2", resp.Total)
	}
}

func TestHandler_ListInstances_WithPagination(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.CreateInstance(ctx, &store.Instance{
			Name:     "inst-" + string(rune('a'+i)),
			Hostname: "host-" + string(rune('a'+i)),
		})
	}

	req := httptest.NewRequest("GET", "/api/v1/instances?limit=2&offset=1", nil)
	w := httptest.NewRecorder()
	h.ListInstances(w, req)

	var resp ListInstancesResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Instances) != 2 {
		t.Errorf("Instances count with pagination = %d, want 2", len(resp.Instances))
	}
}

func TestHandler_ListInstances_Empty(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/v1/instances", nil)
	w := httptest.NewRecorder()
	h.ListInstances(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp ListInstancesResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Total != 0 {
		t.Errorf("Total = %d, want 0", resp.Total)
	}
	if resp.Instances == nil {
		t.Error("Instances should be empty array, not nil")
	}
}

func TestHandler_CreateInstance(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"name": "test-instance", "hostname": "test.local", "agent_version": "1.0.0"}`
	req := httptest.NewRequest("POST", "/api/v1/instances", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateInstance(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d. Body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var inst store.Instance
	if err := json.NewDecoder(w.Body).Decode(&inst); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if inst.ID == "" {
		t.Error("ID should be generated")
	}
	if inst.Name != "test-instance" {
		t.Errorf("Name = %q, want %q", inst.Name, "test-instance")
	}
	if inst.Hostname != "test.local" {
		t.Errorf("Hostname = %q, want %q", inst.Hostname, "test.local")
	}
	if inst.Status != store.InstanceStatusUnknown {
		t.Errorf("Status = %q, want %q", inst.Status, store.InstanceStatusUnknown)
	}
}

func TestHandler_CreateInstance_WithLabels(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"name": "test-instance", "hostname": "test.local", "labels": {"env": "prod", "region": "us-west"}}`
	req := httptest.NewRequest("POST", "/api/v1/instances", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateInstance(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var inst store.Instance
	json.NewDecoder(w.Body).Decode(&inst)

	if inst.Labels["env"] != "prod" {
		t.Errorf("Labels[env] = %q, want %q", inst.Labels["env"], "prod")
	}
	if inst.Labels["region"] != "us-west" {
		t.Errorf("Labels[region] = %q, want %q", inst.Labels["region"], "us-west")
	}
}

func TestHandler_CreateInstance_InvalidJSON(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest("POST", "/api/v1/instances", bytes.NewBufferString("invalid json"))
	w := httptest.NewRecorder()

	h.CreateInstance(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Code != "INVALID_JSON" {
		t.Errorf("Code = %q, want %q", resp.Code, "INVALID_JSON")
	}
}

func TestHandler_CreateInstance_MissingName(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"hostname": "test.local"}`
	req := httptest.NewRequest("POST", "/api/v1/instances", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.CreateInstance(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Code != "VALIDATION_ERROR" {
		t.Errorf("Code = %q, want %q", resp.Code, "VALIDATION_ERROR")
	}
}

func TestHandler_CreateInstance_MissingHostname(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"name": "test-instance"}`
	req := httptest.NewRequest("POST", "/api/v1/instances", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.CreateInstance(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_CreateInstance_Duplicate(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	// Create first instance
	s.CreateInstance(ctx, &store.Instance{Name: "test-instance", Hostname: "test.local"})

	// Try to create duplicate
	body := `{"name": "test-instance", "hostname": "other.local"}`
	req := httptest.NewRequest("POST", "/api/v1/instances", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.CreateInstance(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Code != "ALREADY_EXISTS" {
		t.Errorf("Code = %q, want %q", resp.Code, "ALREADY_EXISTS")
	}
}

func TestHandler_GetInstance(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	inst := &store.Instance{Name: "test-instance", Hostname: "test.local"}
	s.CreateInstance(ctx, inst)

	req := httptest.NewRequest("GET", "/api/v1/instances/"+inst.ID, nil)
	req = chiContext(req, map[string]string{"id": inst.ID})
	w := httptest.NewRecorder()

	h.GetInstance(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var retrieved store.Instance
	json.NewDecoder(w.Body).Decode(&retrieved)

	if retrieved.ID != inst.ID {
		t.Errorf("ID = %q, want %q", retrieved.ID, inst.ID)
	}
	if retrieved.Name != "test-instance" {
		t.Errorf("Name = %q, want %q", retrieved.Name, "test-instance")
	}
}

func TestHandler_GetInstance_NotFound(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/v1/instances/nonexistent", nil)
	req = chiContext(req, map[string]string{"id": "nonexistent"})
	w := httptest.NewRecorder()

	h.GetInstance(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Code != "NOT_FOUND" {
		t.Errorf("Code = %q, want %q", resp.Code, "NOT_FOUND")
	}
}

func TestHandler_UpdateInstance(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	inst := &store.Instance{Name: "test-instance", Hostname: "test.local", Status: store.InstanceStatusUnknown}
	s.CreateInstance(ctx, inst)

	body := `{"name": "updated-instance", "status": "online"}`
	req := httptest.NewRequest("PUT", "/api/v1/instances/"+inst.ID, bytes.NewBufferString(body))
	req = chiContext(req, map[string]string{"id": inst.ID})
	w := httptest.NewRecorder()

	h.UpdateInstance(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var updated store.Instance
	json.NewDecoder(w.Body).Decode(&updated)

	if updated.Name != "updated-instance" {
		t.Errorf("Name = %q, want %q", updated.Name, "updated-instance")
	}
	if updated.Status != store.InstanceStatusOnline {
		t.Errorf("Status = %q, want %q", updated.Status, store.InstanceStatusOnline)
	}
}

func TestHandler_UpdateInstance_NotFound(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"name": "updated"}`
	req := httptest.NewRequest("PUT", "/api/v1/instances/nonexistent", bytes.NewBufferString(body))
	req = chiContext(req, map[string]string{"id": "nonexistent"})
	w := httptest.NewRecorder()

	h.UpdateInstance(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandler_UpdateInstance_InvalidJSON(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	inst := &store.Instance{Name: "test-instance", Hostname: "test.local"}
	s.CreateInstance(ctx, inst)

	req := httptest.NewRequest("PUT", "/api/v1/instances/"+inst.ID, bytes.NewBufferString("invalid"))
	req = chiContext(req, map[string]string{"id": inst.ID})
	w := httptest.NewRecorder()

	h.UpdateInstance(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_DeleteInstance(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	inst := &store.Instance{Name: "test-instance", Hostname: "test.local"}
	s.CreateInstance(ctx, inst)

	req := httptest.NewRequest("DELETE", "/api/v1/instances/"+inst.ID, nil)
	req = chiContext(req, map[string]string{"id": inst.ID})
	w := httptest.NewRecorder()

	h.DeleteInstance(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	// Verify deleted
	deleted, _ := s.GetInstance(ctx, inst.ID)
	if deleted != nil {
		t.Error("instance should be deleted")
	}
}

func TestHandler_DeleteInstance_NotFound(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest("DELETE", "/api/v1/instances/nonexistent", nil)
	req = chiContext(req, map[string]string{"id": "nonexistent"})
	w := httptest.NewRecorder()

	h.DeleteInstance(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ============================================
// Config Handler Tests
// ============================================

func TestHandler_ListConfigs(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		s.CreateConfig(ctx, &store.Config{Name: "cfg-" + string(rune('a'+i))})
	}

	req := httptest.NewRequest("GET", "/api/v1/configs", nil)
	w := httptest.NewRecorder()

	h.ListConfigs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp ListConfigsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Total != 3 {
		t.Errorf("Total = %d, want 3", resp.Total)
	}
}

func TestHandler_ListConfigs_WithPagination(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.CreateConfig(ctx, &store.Config{Name: "cfg-" + string(rune('a'+i))})
	}

	req := httptest.NewRequest("GET", "/api/v1/configs?limit=2&offset=1", nil)
	w := httptest.NewRecorder()
	h.ListConfigs(w, req)

	var resp ListConfigsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Configs) != 2 {
		t.Errorf("Configs count = %d, want 2", len(resp.Configs))
	}
}

func TestHandler_ListConfigs_Empty(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/v1/configs", nil)
	w := httptest.NewRecorder()
	h.ListConfigs(w, req)

	var resp ListConfigsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Configs == nil {
		t.Error("Configs should be empty array, not nil")
	}
}

func TestHandler_CreateConfig(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"name": "test-config", "content": "server { listen 8080 }"}`
	req := httptest.NewRequest("POST", "/api/v1/configs", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.CreateConfig(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d. Body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp CreateConfigResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Config.ID == "" {
		t.Error("Config.ID should be generated")
	}
	if resp.Config.Name != "test-config" {
		t.Errorf("Config.Name = %q, want %q", resp.Config.Name, "test-config")
	}
	if resp.Version.Version != 1 {
		t.Errorf("Version.Version = %d, want 1", resp.Version.Version)
	}
	if resp.Version.Content != "server { listen 8080 }" {
		t.Errorf("Version.Content = %q, want %q", resp.Version.Content, "server { listen 8080 }")
	}
	if resp.Version.ContentHash == "" {
		t.Error("Version.ContentHash should be set")
	}
}

func TestHandler_CreateConfig_WithDescription(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"name": "test-config", "description": "Test configuration", "content": "server {}"}`
	req := httptest.NewRequest("POST", "/api/v1/configs", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.CreateConfig(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp CreateConfigResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Config.Description == nil || *resp.Config.Description != "Test configuration" {
		t.Errorf("Description = %v, want %q", resp.Config.Description, "Test configuration")
	}
}

func TestHandler_CreateConfig_InvalidJSON(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest("POST", "/api/v1/configs", bytes.NewBufferString("invalid"))
	w := httptest.NewRecorder()

	h.CreateConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_CreateConfig_MissingName(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"content": "server {}"}`
	req := httptest.NewRequest("POST", "/api/v1/configs", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.CreateConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Code != "VALIDATION_ERROR" {
		t.Errorf("Code = %q, want %q", resp.Code, "VALIDATION_ERROR")
	}
}

func TestHandler_CreateConfig_MissingContent(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"name": "test-config"}`
	req := httptest.NewRequest("POST", "/api/v1/configs", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.CreateConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_GetConfig(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	cfg := &store.Config{Name: "test-config"}
	s.CreateConfig(ctx, cfg)
	s.CreateConfigVersion(ctx, &store.ConfigVersion{
		ConfigID: cfg.ID,
		Version:  1,
		Content:  "server {}",
	})

	req := httptest.NewRequest("GET", "/api/v1/configs/"+cfg.ID, nil)
	req = chiContext(req, map[string]string{"id": cfg.ID})
	w := httptest.NewRecorder()

	h.GetConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp GetConfigResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Config.ID != cfg.ID {
		t.Errorf("Config.ID = %q, want %q", resp.Config.ID, cfg.ID)
	}
	if resp.CurrentVersion == nil {
		t.Error("CurrentVersion should be set")
	}
	if resp.CurrentVersion != nil && resp.CurrentVersion.Version != 1 {
		t.Errorf("CurrentVersion.Version = %d, want 1", resp.CurrentVersion.Version)
	}
}

func TestHandler_GetConfig_NotFound(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/v1/configs/nonexistent", nil)
	req = chiContext(req, map[string]string{"id": "nonexistent"})
	w := httptest.NewRecorder()

	h.GetConfig(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandler_UpdateConfig(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	cfg := &store.Config{Name: "test-config", CurrentVersion: 1}
	s.CreateConfig(ctx, cfg)
	s.CreateConfigVersion(ctx, &store.ConfigVersion{ConfigID: cfg.ID, Version: 1, Content: "old content"})

	body := `{"name": "updated-config", "content": "new content", "change_summary": "Updated config"}`
	req := httptest.NewRequest("PUT", "/api/v1/configs/"+cfg.ID, bytes.NewBufferString(body))
	req = chiContext(req, map[string]string{"id": cfg.ID})
	w := httptest.NewRecorder()

	h.UpdateConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp GetConfigResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Config.Name != "updated-config" {
		t.Errorf("Config.Name = %q, want %q", resp.Config.Name, "updated-config")
	}
	if resp.CurrentVersion == nil {
		t.Fatal("CurrentVersion should be set")
	}
	if resp.CurrentVersion.Version != 2 {
		t.Errorf("CurrentVersion.Version = %d, want 2", resp.CurrentVersion.Version)
	}
	if resp.CurrentVersion.Content != "new content" {
		t.Errorf("CurrentVersion.Content = %q, want %q", resp.CurrentVersion.Content, "new content")
	}
}

func TestHandler_UpdateConfig_MetadataOnly(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	desc := "Original description"
	cfg := &store.Config{Name: "test-config", Description: &desc}
	s.CreateConfig(ctx, cfg)

	newDesc := "Updated description"
	body := `{"description": "Updated description"}`
	req := httptest.NewRequest("PUT", "/api/v1/configs/"+cfg.ID, bytes.NewBufferString(body))
	req = chiContext(req, map[string]string{"id": cfg.ID})
	w := httptest.NewRecorder()

	h.UpdateConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp GetConfigResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Config.Description == nil || *resp.Config.Description != newDesc {
		t.Errorf("Description = %v, want %q", resp.Config.Description, newDesc)
	}
	if resp.CurrentVersion != nil {
		t.Error("CurrentVersion should be nil when only metadata updated")
	}
}

func TestHandler_UpdateConfig_NotFound(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"name": "updated"}`
	req := httptest.NewRequest("PUT", "/api/v1/configs/nonexistent", bytes.NewBufferString(body))
	req = chiContext(req, map[string]string{"id": "nonexistent"})
	w := httptest.NewRecorder()

	h.UpdateConfig(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandler_DeleteConfig(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	cfg := &store.Config{Name: "test-config"}
	s.CreateConfig(ctx, cfg)

	req := httptest.NewRequest("DELETE", "/api/v1/configs/"+cfg.ID, nil)
	req = chiContext(req, map[string]string{"id": cfg.ID})
	w := httptest.NewRecorder()

	h.DeleteConfig(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestHandler_DeleteConfig_NotFound(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest("DELETE", "/api/v1/configs/nonexistent", nil)
	req = chiContext(req, map[string]string{"id": "nonexistent"})
	w := httptest.NewRecorder()

	h.DeleteConfig(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandler_ListConfigVersions(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	cfg := &store.Config{Name: "test-config"}
	s.CreateConfig(ctx, cfg)
	s.CreateConfigVersion(ctx, &store.ConfigVersion{ConfigID: cfg.ID, Version: 1, Content: "v1"})
	s.CreateConfigVersion(ctx, &store.ConfigVersion{ConfigID: cfg.ID, Version: 2, Content: "v2"})

	req := httptest.NewRequest("GET", "/api/v1/configs/"+cfg.ID+"/versions", nil)
	req = chiContext(req, map[string]string{"id": cfg.ID})
	w := httptest.NewRecorder()

	h.ListConfigVersions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp ListConfigVersionsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Total != 2 {
		t.Errorf("Total = %d, want 2", resp.Total)
	}
}

func TestHandler_ListConfigVersions_ConfigNotFound(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/v1/configs/nonexistent/versions", nil)
	req = chiContext(req, map[string]string{"id": "nonexistent"})
	w := httptest.NewRecorder()

	h.ListConfigVersions(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandler_RollbackConfig(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	cfg := &store.Config{Name: "test-config"}
	s.CreateConfig(ctx, cfg)
	s.CreateConfigVersion(ctx, &store.ConfigVersion{ConfigID: cfg.ID, Version: 1, Content: "original content"})
	s.CreateConfigVersion(ctx, &store.ConfigVersion{ConfigID: cfg.ID, Version: 2, Content: "new content"})
	// Update config's current version
	cfg.CurrentVersion = 2
	s.UpdateConfig(ctx, cfg)

	body := `{"version": 1}`
	req := httptest.NewRequest("POST", "/api/v1/configs/"+cfg.ID+"/rollback", bytes.NewBufferString(body))
	req = chiContext(req, map[string]string{"id": cfg.ID})
	w := httptest.NewRecorder()

	h.RollbackConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d. Body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp GetConfigResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.CurrentVersion == nil {
		t.Fatal("CurrentVersion should be set")
	}
	if resp.CurrentVersion.Version != 3 {
		t.Errorf("CurrentVersion.Version = %d, want 3 (new version created)", resp.CurrentVersion.Version)
	}
	if resp.CurrentVersion.Content != "original content" {
		t.Errorf("CurrentVersion.Content = %q, want %q", resp.CurrentVersion.Content, "original content")
	}
}

func TestHandler_RollbackConfig_ConfigNotFound(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"version": 1}`
	req := httptest.NewRequest("POST", "/api/v1/configs/nonexistent/rollback", bytes.NewBufferString(body))
	req = chiContext(req, map[string]string{"id": "nonexistent"})
	w := httptest.NewRecorder()

	h.RollbackConfig(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandler_RollbackConfig_VersionNotFound(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	cfg := &store.Config{Name: "test-config"}
	s.CreateConfig(ctx, cfg)
	s.CreateConfigVersion(ctx, &store.ConfigVersion{ConfigID: cfg.ID, Version: 1, Content: "content"})

	body := `{"version": 99}`
	req := httptest.NewRequest("POST", "/api/v1/configs/"+cfg.ID+"/rollback", bytes.NewBufferString(body))
	req = chiContext(req, map[string]string{"id": cfg.ID})
	w := httptest.NewRecorder()

	h.RollbackConfig(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandler_RollbackConfig_InvalidVersion(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	cfg := &store.Config{Name: "test-config"}
	s.CreateConfig(ctx, cfg)

	body := `{"version": 0}`
	req := httptest.NewRequest("POST", "/api/v1/configs/"+cfg.ID+"/rollback", bytes.NewBufferString(body))
	req = chiContext(req, map[string]string{"id": cfg.ID})
	w := httptest.NewRecorder()

	h.RollbackConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_RollbackConfig_InvalidJSON(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	cfg := &store.Config{Name: "test-config"}
	s.CreateConfig(ctx, cfg)

	req := httptest.NewRequest("POST", "/api/v1/configs/"+cfg.ID+"/rollback", bytes.NewBufferString("invalid"))
	req = chiContext(req, map[string]string{"id": cfg.ID})
	w := httptest.NewRecorder()

	h.RollbackConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ============================================
// Deployment Handler Tests
// ============================================

func TestHandler_ListDeployments(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	// Create config for FK
	cfg := &store.Config{Name: "test-config"}
	s.CreateConfig(ctx, cfg)
	s.CreateConfigVersion(ctx, &store.ConfigVersion{ConfigID: cfg.ID, Version: 1, Content: "content"})

	for i := 0; i < 3; i++ {
		s.CreateDeployment(ctx, &store.Deployment{
			ConfigID:        cfg.ID,
			ConfigVersion:   1,
			TargetInstances: []string{"inst-1"},
			Strategy:        store.DeploymentStrategyRolling,
			Status:          store.DeploymentStatusPending,
		})
	}

	req := httptest.NewRequest("GET", "/api/v1/deployments", nil)
	w := httptest.NewRecorder()

	h.ListDeployments(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp ListDeploymentsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Total != 3 {
		t.Errorf("Total = %d, want 3", resp.Total)
	}
}

func TestHandler_ListDeployments_WithFilters(t *testing.T) {
	h, s := setupTestHandler(t)
	ctx := context.Background()

	cfg := &store.Config{Name: "test-config"}
	s.CreateConfig(ctx, cfg)
	s.CreateConfigVersion(ctx, &store.ConfigVersion{ConfigID: cfg.ID, Version: 1, Content: "content"})

	s.CreateDeployment(ctx, &store.Deployment{
		ConfigID: cfg.ID, ConfigVersion: 1, TargetInstances: []string{"inst-1"},
		Strategy: store.DeploymentStrategyRolling, Status: store.DeploymentStatusPending,
	})
	s.CreateDeployment(ctx, &store.Deployment{
		ConfigID: cfg.ID, ConfigVersion: 1, TargetInstances: []string{"inst-1"},
		Strategy: store.DeploymentStrategyRolling, Status: store.DeploymentStatusCompleted,
	})

	req := httptest.NewRequest("GET", "/api/v1/deployments?status=completed", nil)
	w := httptest.NewRecorder()
	h.ListDeployments(w, req)

	var resp ListDeploymentsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Total != 1 {
		t.Errorf("Total with status filter = %d, want 1", resp.Total)
	}
}

func TestHandler_ListDeployments_Empty(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest("GET", "/api/v1/deployments", nil)
	w := httptest.NewRecorder()
	h.ListDeployments(w, req)

	var resp ListDeploymentsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Deployments == nil {
		t.Error("Deployments should be empty array, not nil")
	}
}

func TestHandler_CreateDeployment_MissingConfigID(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"target_instances": ["inst-1"]}`
	req := httptest.NewRequest("POST", "/api/v1/deployments", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.CreateDeployment(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Code != "VALIDATION_ERROR" {
		t.Errorf("Code = %q, want %q", resp.Code, "VALIDATION_ERROR")
	}
}

func TestHandler_CreateDeployment_MissingTargets(t *testing.T) {
	h, _ := setupTestHandler(t)

	body := `{"config_id": "cfg-1"}`
	req := httptest.NewRequest("POST", "/api/v1/deployments", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.CreateDeployment(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_CreateDeployment_InvalidJSON(t *testing.T) {
	h, _ := setupTestHandler(t)

	req := httptest.NewRequest("POST", "/api/v1/deployments", bytes.NewBufferString("invalid"))
	w := httptest.NewRecorder()

	h.CreateDeployment(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ============================================
// Helper Function Tests
// ============================================

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"key": "value"}

	writeJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}

	var result map[string]string
	json.NewDecoder(w.Body).Decode(&result)

	if result["key"] != "value" {
		t.Errorf("key = %q, want %q", result["key"], "value")
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()

	writeError(w, http.StatusBadRequest, "TEST_ERROR", "Test error message")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Error != "Test error message" {
		t.Errorf("Error = %q, want %q", resp.Error, "Test error message")
	}
	if resp.Code != "TEST_ERROR" {
		t.Errorf("Code = %q, want %q", resp.Code, "TEST_ERROR")
	}
}

func TestErrorResponse_Structure(t *testing.T) {
	resp := ErrorResponse{
		Error:   "Something went wrong",
		Code:    "INTERNAL_ERROR",
		Details: "Additional details",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ErrorResponse
	json.Unmarshal(data, &decoded)

	if decoded.Error != resp.Error {
		t.Errorf("Error = %q, want %q", decoded.Error, resp.Error)
	}
	if decoded.Code != resp.Code {
		t.Errorf("Code = %q, want %q", decoded.Code, resp.Code)
	}
	if decoded.Details != resp.Details {
		t.Errorf("Details = %q, want %q", decoded.Details, resp.Details)
	}
}
