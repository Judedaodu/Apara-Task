package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handleHealth(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var h HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&h); err != nil {
		t.Fatal(err)
	}
	if h.Status != "ok" || h.Service != "go-api" {
		t.Fatalf("unexpected %+v", h)
	}
}

func TestInitializeAkinNetFailure(t *testing.T) {
	t.Setenv("AKINNET_FAILURE_RATE", "1")
	t.Setenv("KOTLIN_CORE_URL", "http://127.0.0.1:9")

	body := `{"templateId":"t1","amount":100,"currency":"AKN","receiverAccount":"r1","senderBank":"b1"}`
	req := httptest.NewRequest(http.MethodPost, "/initialize", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	handleInitialize(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 got %d body %s", rr.Code, rr.Body.String())
	}
}

func TestInitializeCoreUnreachable(t *testing.T) {
	t.Setenv("AKINNET_FAILURE_RATE", "0")
	t.Setenv("KOTLIN_CORE_URL", "http://127.0.0.1:9")

	body := `{"templateId":"t1","amount":100,"currency":"AKN","receiverAccount":"r1","senderBank":"b1"}`
	req := httptest.NewRequest(http.MethodPost, "/initialize", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	handleInitialize(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("want 502 got %d", rr.Code)
	}
}

func TestInitializeHappyPath(t *testing.T) {
	t.Setenv("AKINNET_FAILURE_RATE", "0")
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in CoreSubmitRequest
		_ = json.NewDecoder(r.Body).Decode(&in)
		_ = json.NewEncoder(w).Encode(CoreSubmitResponse{
			Fingerprint: in.Fingerprint,
			State:       "SETTLED",
			Timestamp:   1,
		})
	}))
	defer core.Close()
	t.Setenv("KOTLIN_CORE_URL", core.URL)

	body := `{"templateId":"t1","amount":100,"currency":"AKN","receiverAccount":"r1","senderBank":"b1"}`
	req := httptest.NewRequest(http.MethodPost, "/initialize", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	handleInitialize(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d %s", rr.Code, rr.Body.String())
	}
	var out InitializeResponse
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.State != "IN_TRANSIT" {
		t.Fatalf("state %q", out.State)
	}
}

func TestGetenvFloat(t *testing.T) {
	_ = os.Setenv("X", "0.5")
	defer os.Unsetenv("X")
	if getenvFloat("X", 0) != 0.5 {
		t.Fatal()
	}
}
