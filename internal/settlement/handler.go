package settlement

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ubaid/hisaab/internal/auth"
)

// HandlerConfig provides operational flags for the HTTP handler.
type HandlerConfig struct {
	VerifySignatures bool `json:"verify_signatures"`
}

// StoredClaimRecord holds a received claim and its cached Beckn ACK.
type StoredClaimRecord struct {
	MessageID   string             `json:"message_id"`
	RequestHash string             `json:"request_hash"`
	Claim       SettleClaimPayload `json:"claim"`
	AckResponse AckResponse        `json:"ack_response"`
	ReceivedAt  time.Time          `json:"received_at"`
}

// Handler serves HTTP endpoints for the Settlement Agency simulator.
type Handler struct {
	config       HandlerConfig
	signer       *auth.RequestSigner
	matcher      *Matcher
	orchestrator *Orchestrator

	mu     sync.RWMutex
	claims map[string]*StoredClaimRecord // key: message_id
}

// NewHandler creates an initialized settlement HTTP handler.
func NewHandler(config HandlerConfig, signer *auth.RequestSigner, matcher *Matcher) *Handler {
	if matcher == nil {
		matcher = NewMatcher(DefaultPolicy())
	}
	return &Handler{
		config:  config,
		signer:  signer,
		matcher: matcher,
		claims:  make(map[string]*StoredClaimRecord),
	}
}

// SetOrchestrator attaches the settlement orchestrator to drive end-to-end processing.
func (h *Handler) SetOrchestrator(orch *Orchestrator) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.orchestrator = orch
}

// HandleSettle handles inbound POST /settle claims from Network Participants.
func (h *Handler) HandleSettle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Guard against memory exhaustion with 1MB bounded reader (B6)
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeNACK(w, http.StatusBadRequest, nil, "INVALID_REQUEST", "Failed to read request body: "+err.Error())
		return
	}
	defer r.Body.Close()

	// 1. Signature Verification (if enabled)
	if h.config.VerifySignatures && h.signer != nil {
		if err := h.signer.VerifyRequest(r, body); err != nil {
			h.writeNACK(w, http.StatusUnauthorized, nil, "AUTH_FAILED", err.Error())
			return
		}
	}

	// 2. Decode Claim Payload (handles both wire decimal format and internal paise representation)
	claim, err := ParseClaimFromJSON(body)
	if err != nil {
		h.writeNACK(w, http.StatusBadRequest, nil, "SCHEMA_ERROR", "Malformed JSON payload: "+err.Error())
		return
	}

	// 3. Schema & Context Validation
	if claim.Context == nil || claim.Context.MessageID == "" {
		h.writeNACK(w, http.StatusBadRequest, claim.Context, "SCHEMA_ERROR", "Missing context or message_id (EC-07)")
		return
	}

	if claim.Context.Action != "settle" {
		h.writeNACK(w, http.StatusBadRequest, claim.Context, "INVALID_ACTION", "Action must be 'settle'")
		return
	}

	if err := ValidateClaimPayload(claim); err != nil {
		h.writeNACK(w, http.StatusBadRequest, claim.Context, "VALIDATION_FAILED", err.Error())
		return
	}

	// 4. Delegate to Orchestrator if attached
	h.mu.RLock()
	orch := h.orchestrator
	h.mu.RUnlock()

	if orch != nil {
		ack, err := orch.SubmitClaim(r.Context(), claim)
		if err != nil {
			h.writeNACK(w, http.StatusBadRequest, claim.Context, "ORCHESTRATION_ERROR", err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ack)
		return
	}

	// 5. Standalone in-memory idempotency check (Message ID + Payload Hash)
	hasher := sha256.New()
	hasher.Write(body)
	reqHash := hex.EncodeToString(hasher.Sum(nil))

	msgID := claim.Context.MessageID

	h.mu.Lock()
	existing, found := h.claims[msgID]
	if found {
		h.mu.Unlock()
		if existing.RequestHash == reqHash {
			// Idempotent replay: return cached ACK
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(existing.AckResponse)
			return
		}
		// Duplicate message_id with conflicting payload: Reject
		h.writeNACK(w, http.StatusBadRequest, claim.Context, "IDEMPOTENCY_CONFLICT",
			"Duplicate message_id with conflicting payload")
		return
	}

	// 5. Build synchronous ACK response
	ack := AckResponse{
		Context: claim.Context,
	}
	ack.Message.Ack.Status = AckStatusACK

	// Store claim
	h.claims[msgID] = &StoredClaimRecord{
		MessageID:   msgID,
		RequestHash: reqHash,
		Claim:       *claim,
		AckResponse: ack,
		ReceivedAt:  time.Now(),
	}
	h.mu.Unlock()

	// 6. Return synchronous ACK
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ack)
}

// HandleOnSettle receives /on_settle asynchronous settlement confirmation callbacks.
func (h *Handler) HandleOnSettle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeNACK(w, http.StatusBadRequest, nil, "INVALID_REQUEST", "Failed to read request body: "+err.Error())
		return
	}
	defer r.Body.Close()

	if h.config.VerifySignatures && h.signer != nil {
		if err := h.signer.VerifyRequest(r, body); err != nil {
			h.writeNACK(w, http.StatusUnauthorized, nil, "AUTH_FAILED", err.Error())
			return
		}
	}

	// Validate /on_settle callback payload against Inv-14 status enumeration
	var callback OnSettleCallbackPayload
	if err := json.Unmarshal(body, &callback); err == nil && callback.Status != "" {
		validStatuses := map[string]bool{"SETTLED": true, "FAILED": true, "PARTIAL": true}
		if !validStatuses[callback.Status] {
			h.writeNACK(w, http.StatusBadRequest, nil, "INVALID_STATUS", "Status must be SETTLED, FAILED, or PARTIAL (Inv-14)")
			return
		}
	}

	var ack AckResponse
	ack.Message.Ack.Status = AckStatusACK
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ack)
}

// GetStoredClaim retrieves a stored claim record by message_id for inspection or settlement matching.
func (h *Handler) GetStoredClaim(messageID string) (*StoredClaimRecord, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	rec, ok := h.claims[messageID]
	return rec, ok
}

// writeNACK writes a standardized Beckn NACK response.
func (h *Handler) writeNACK(w http.ResponseWriter, statusCode int, ctx *BecknContext, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := AckResponse{
		Context: ctx,
		Error: &BecknError{
			Type:    "DOMAIN-ERROR",
			Code:    code,
			Message: message,
		},
	}
	resp.Message.Ack.Status = AckStatusNACK
	_ = json.NewEncoder(w).Encode(resp)
}
