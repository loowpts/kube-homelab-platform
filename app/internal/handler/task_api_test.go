package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteErrorHidesServerErrorDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusInternalServerError, errors.New(`pq: password authentication failed for user "dev_user"`))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if body := rec.Body.String(); strings.Contains(body, "dev_user") {
		t.Errorf("response leaks error details: %s", body)
	}
}

func TestWriteErrorKeepsClientErrorMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, errors.New("title is required"))

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "title is required" {
		t.Errorf("error = %q, want %q", body["error"], "title is required")
	}
}

func TestCreateRejectsOversizedBody(t *testing.T) {
	// The body is rejected while decoding, before the service is used.
	h := NewTaskAPIHandler(nil)
	body := `{"title":"` + strings.Repeat("a", maxBodyBytes) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
