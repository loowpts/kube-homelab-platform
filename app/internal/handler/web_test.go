package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Форма редактирования, как и API, не принимает пустой title — до обращения к сервису.
func TestWebUpdateRejectsEmptyTitle(t *testing.T) {
	h := &WebHandler{}
	form := url.Values{"title": {""}, "description": {"x"}}
	req := httptest.NewRequest(http.MethodPost, "/tasks/1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
