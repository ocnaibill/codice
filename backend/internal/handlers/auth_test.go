package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRegister_DisabledByDefault(t *testing.T) {
	os.Unsetenv("ALLOW_REGISTRATION")
	os.Unsetenv("APP_ENV")

	handler := &AuthHandler{}

	req := httptest.NewRequest("POST", "/auth/register",
		strings.NewReader(`{"username":"test","password":"test123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Register(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 when registration disabled, got %d", rec.Code)
	}
}


func TestRegister_DisabledInProduction(t *testing.T) {
	os.Setenv("APP_ENV", "production")
	os.Unsetenv("ALLOW_REGISTRATION")
	defer os.Unsetenv("APP_ENV")

	handler := &AuthHandler{}

	req := httptest.NewRequest("POST", "/auth/register",
		strings.NewReader(`{"username":"test","password":"test123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Register(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 production without ALLOW_REGISTRATION=true, got %d", rec.Code)
	}
}

func TestRegister_InvalidJSON(t *testing.T) {
	os.Setenv("ALLOW_REGISTRATION", "true")
	defer os.Unsetenv("ALLOW_REGISTRATION")

	handler := &AuthHandler{}

	req := httptest.NewRequest("POST", "/auth/register",
		strings.NewReader(`invalid json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rec.Code)
	}
}

func TestRegister_UsernameAndPasswordRequired(t *testing.T) {
	os.Setenv("ALLOW_REGISTRATION", "true")
	defer os.Unsetenv("ALLOW_REGISTRATION")

	handler := &AuthHandler{}

	// Missing password
	req := httptest.NewRequest("POST", "/auth/register",
		strings.NewReader(`{"username":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when password missing, got %d", rec.Code)
	}
}

func TestSetupStatusJSON(t *testing.T) {
	// Verify the response structure serializes correctly
	resp := SetupStatusResponse{IsFirstRun: true}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if !strings.Contains(string(data), `"isFirstRun":true`) {
		t.Errorf("expected isFirstRun:true in JSON, got %s", data)
	}

	resp2 := SetupStatusResponse{IsFirstRun: false}
	data2, _ := json.Marshal(resp2)
	if !strings.Contains(string(data2), `"isFirstRun":false`) {
		t.Errorf("expected isFirstRun:false in JSON, got %s", data2)
	}
}