package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestEnvHelpers(t *testing.T) {
	t.Setenv("BOOL_TEST", "yes")
	if !boolEnv("BOOL_TEST", false) {
		t.Fatal("expected boolEnv yes to be true")
	}
	t.Setenv("DURATION_TEST", "2s")
	if durationEnv("DURATION_TEST", time.Second) != 2*time.Second {
		t.Fatal("expected durationEnv to parse 2s")
	}
	t.Setenv("DURATION_SECONDS_TEST", "3")
	if durationEnv("DURATION_SECONDS_TEST", time.Second) != 3*time.Second {
		t.Fatal("expected durationEnv to parse plain seconds")
	}
	if env("MISSING_ENV_TEST", "fallback") != "fallback" {
		t.Fatal("expected env fallback")
	}
}

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusCreated, map[string]string{"status": "ok"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content-type: %q", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestRecovererReturnsJSONError(t *testing.T) {
	h := recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["error"] == "" {
		t.Fatalf("expected error body, got %#v", body)
	}
}

func TestCORSOptions(t *testing.T) {
	h := cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodOptions, "/", nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("expected CORS header")
	}
}

func TestSampleEvents(t *testing.T) {
	events := sampleEvents()
	if len(events) != 1 || len(events[0].EventSessions) != 1 {
		t.Fatalf("unexpected sample events: %#v", events)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
