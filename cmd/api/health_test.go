package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthState_Liveness(t *testing.T) {
	health := &HealthState{}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	health.HandleLiveness(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse liveness response: %v", err)
	}
	if resp["status"] != "UP" {
		t.Errorf("Expected status UP, got %v", resp["status"])
	}
}

func TestHealthState_Readiness(t *testing.T) {
	health := &HealthState{}

	// When not ready
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	health.HandleReadiness(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("Expected 503 when not ready, got %d", w.Code)
	}

	// When ready
	health.SetReady(true)
	w2 := httptest.NewRecorder()
	health.HandleReadiness(w2, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("Expected 200 when ready, got %d", w2.Code)
	}
}
