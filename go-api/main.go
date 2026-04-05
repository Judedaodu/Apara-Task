package main

import (
	"bytes"
	crand "crypto/rand"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"
)

type InitializeRequest struct {
	TemplateID      string `json:"templateId"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
	ReceiverAccount string `json:"receiverAccount"`
	SenderBank      string `json:"senderBank"`
}

type InitializeResponse struct {
	Fingerprint string `json:"fingerprint"`
	State       string `json:"state"`
	Timestamp   int64  `json:"timestamp"`
}

type AkinNetError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type CoreSubmitRequest struct {
	Fingerprint string `json:"fingerprint"`
	TemplateID  string `json:"templateId"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	SenderBank  string `json:"senderBank"`
}

type CoreSubmitResponse struct {
	Fingerprint string `json:"fingerprint"`
	State       string `json:"state"`
	Timestamp   int64  `json:"timestamp"`
	Message     string `json:"message,omitempty"`
}

type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

func main() {
	mux := http.NewServeMux()
	// Go 1.21 mux does not support "GET /path" patterns (Go 1.22+); Dockerfile uses Go 1.21.
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/initialize", handleInitialize)

	addr := ":8080"
	log.Printf("go-api listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok", Service: "go-api"})
}

func handleInitialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"METHOD_NOT_ALLOWED"}`, http.StatusMethodNotAllowed)
		return
	}
	var req InitializeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("initialize: bad json: %v", err)
		http.Error(w, `{"error":"BAD_REQUEST"}`, http.StatusBadRequest)
		return
	}

	if akinNetFailure() {
		writeJSON(w, http.StatusUnprocessableEntity, AkinNetError{
			Error:   "AKINNET_FAILURE",
			Message: "simulated AkinNet connectivity failure",
		})
		return
	}

	fingerprint := newFingerprint()
	coreURL := getenv("KOTLIN_CORE_URL", "http://localhost:8081")
	submit := CoreSubmitRequest{
		Fingerprint: fingerprint,
		TemplateID:  req.TemplateID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		SenderBank:  req.SenderBank,
	}

	body, err := json.Marshal(submit)
	if err != nil {
		http.Error(w, `{"error":"INTERNAL"}`, http.StatusInternalServerError)
		return
	}

	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, coreURL+"/core/receive-submit", bytes.NewReader(body))
	if err != nil {
		http.Error(w, `{"error":"INTERNAL"}`, http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if sig := sponsorSignature(body); sig != "" {
		httpReq.Header.Set("X-Sponsor-Signature", sig)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		http.Error(w, `{"error":"CORE_UNREACHABLE"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, `{"error":"INTERNAL"}`, http.StatusInternalServerError)
		return
	}

	if resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(respBody)
		return
	}

	var core CoreSubmitResponse
	if err := json.Unmarshal(respBody, &core); err != nil {
		http.Error(w, `{"error":"BAD_CORE_RESPONSE"}`, http.StatusBadGateway)
		return
	}
	if core.Fingerprint != fingerprint {
		http.Error(w, `{"error":"FINGERPRINT_MISMATCH"}`, http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, InitializeResponse{
		Fingerprint: fingerprint,
		State:       "IN_TRANSIT",
		Timestamp:   time.Now().UnixMilli(),
	})
}

func akinNetFailure() bool {
	rate := getenvFloat("AKINNET_FAILURE_RATE", 0)
	if rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	return rand.Float64() < rate
}

func newFingerprint() string {
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		for i := range b {
			b[i] = byte(rand.Intn(256))
		}
	}
	return hex.EncodeToString(b)
}

func sponsorSignature(payload []byte) string {
	secret := os.Getenv("SPONSOR_HMAC_SECRET")
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func getenv(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func getenvFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
