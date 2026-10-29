package main

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

type HealthState struct {
	isReady atomic.Bool
}

func (h *HealthState) SetReady(ready bool) {
	h.isReady.Store(ready)
}

func (h *HealthState) HandleLiveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "UP",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *HealthState) HandleReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.isReady.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "DOWN",
			"message": "Service initializing",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "READY",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
