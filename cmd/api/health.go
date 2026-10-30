package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

// Pinger abstracts connectivity checks for readiness probes.
type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthState struct {
	isReady atomic.Bool
	pinger  Pinger
}

func (h *HealthState) SetReady(ready bool) {
	h.isReady.Store(ready)
}

func (h *HealthState) SetPinger(p Pinger) {
	h.pinger = p
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

	if h.pinger != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := h.pinger.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "DOWN",
				"message": "Storage backend ping failed: " + err.Error(),
			})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "READY",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
