package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/middleware"
	"github.com/drywaters/permitpal/internal/model"
	"github.com/drywaters/permitpal/internal/repository"
	"github.com/go-chi/chi/v5"
)

// formFixture posts profile and requirement forms as the "aiden" driver with
// the handler clock pinned to now.
type formFixture struct {
	store  repository.Store
	driver model.Driver
	router http.Handler
	now    time.Time
}

func newFormFixture(t *testing.T, backend string, now time.Time) *formFixture {
	t.Helper()
	store := validationStore(t, backend, now)
	driver, err := store.EnsureDriver(context.Background(), "aiden", now)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewDashboardHandler(store, time.Local)
	handler.now = func() time.Time { return now }
	router := chi.NewRouter()
	router.Post("/profile", handler.UpdateProfile)
	router.Post("/requirements/{key}", handler.UpdateRequirement)
	return &formFixture{store: store, driver: driver, router: router, now: now}
}

func (f *formFixture) submit(t *testing.T, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req.WithContext(middleware.WithDriver(req.Context(), f.driver)))
	return rec
}

func (f *formFixture) load(t *testing.T) model.Tracker {
	t.Helper()
	tracker, err := f.store.GetTracker(context.Background(), f.driver.ID)
	if err != nil {
		t.Fatal(err)
	}
	return tracker
}

// requireOK posts a form that must be accepted.
func (f *formFixture) requireOK(t *testing.T, path string, form url.Values) {
	t.Helper()
	if rec := f.submit(t, path, form); rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

// requireRejected posts a form that must get a 400 containing want (any body
// when want is empty) and leave the stored tracker untouched.
func (f *formFixture) requireRejected(t *testing.T, path string, form url.Values, want string) {
	t.Helper()
	before := f.load(t)
	rec := f.submit(t, path, form)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("status=%d body=%q, want 400 %q", rec.Code, rec.Body.String(), want)
	}
	if !reflect.DeepEqual(before, f.load(t)) {
		t.Fatal("rejected input changed the tracker")
	}
}
