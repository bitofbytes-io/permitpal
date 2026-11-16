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
	"github.com/go-chi/chi/v5"
)

func TestRoadTestAppointmentOnDashboard(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.Local)
			store := validationStore(t, backend, now)
			driver, err := store.EnsureDriver(context.Background(), "aiden", now)
			if err != nil {
				t.Fatal(err)
			}
			handler := NewDashboardHandler(store, time.Local)
			handler.now = func() time.Time { return now }
			router := chi.NewRouter()
			router.Get("/", handler.Dashboard)
			router.Post("/profile", handler.UpdateProfile)
			do := func(req *http.Request) *httptest.ResponseRecorder {
				t.Helper()
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req.WithContext(middleware.WithDriver(req.Context(), driver)))
				return rec
			}
			save := func(form url.Values) *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return do(req)
			}
			page := func() string {
				t.Helper()
				rec := do(httptest.NewRequest(http.MethodGet, "/", nil))
				if rec.Code != http.StatusOK {
					t.Fatalf("dashboard status = %d", rec.Code)
				}
				return rec.Body.String()
			}
			stored := func() model.Profile {
				t.Helper()
				tracker, err := store.GetTracker(context.Background(), driver.ID)
				if err != nil {
					t.Fatal(err)
				}
				return tracker.Profile
			}
			form := func(permit, appointment string, total, night string) url.Values {
				return url.Values{"total_hours": {total}, "night_hours": {night}, "permit_issue_date": {permit}, "road_test_date": {appointment}}
			}

			if body := page(); !strings.Contains(body, `id="road_test_date"`) || !strings.Contains(body, "Add the date you booked") {
				t.Fatalf("empty dashboard missing the appointment field or prompt: %s", body)
			}

			// An appointment before the hold ends, with too few hours at the
			// observed pace, saves and shows every warning.
			rec := save(form("2026-06-01", "2026-12-01", "12.5", "2.0"))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
			}
			for _, want := range []string{"<strong>15</strong> days to go", "Appointment is Dec 1, 2026", `data-warning="before_eligible"`, "must be Mar 1, 2027 or later", `data-warning="total_hours"`, `data-warning="night_hours"`} {
				if !strings.Contains(rec.Body.String(), want) {
					t.Errorf("save response missing %q: %s", want, rec.Body.String())
				}
			}
			if got := model.DateValue(stored().RoadTestDate); got != "2026-12-01" {
				t.Fatalf("stored appointment = %q", got)
			}
			body := page()
			for _, want := range []string{`value="2026-12-01"`, "<strong>15</strong> days to go", `data-warning="before_eligible"`} {
				if !strings.Contains(body, want) {
					t.Errorf("dashboard missing %q", want)
				}
			}

			// A form from before the field existed leaves the appointment alone.
			old := form("2026-06-01", "", "13", "2.0")
			old.Del("road_test_date")
			if rec := save(old); rec.Code != http.StatusOK {
				t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
			}
			if got := model.DateValue(stored().RoadTestDate); got != "2026-12-01" || stored().TotalHours != 13 {
				t.Fatalf("stored profile = %+v", stored())
			}

			// A date far enough out for the pace, even under the weekly cap
			// that credits only 10 of these 60 hours, has no warnings.
			rec = save(form("2026-01-15", "2028-01-01", "60", "10"))
			if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "appointment-warnings") || !strings.Contains(rec.Body.String(), "<strong>411</strong> days to go") {
				t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
			}

			// An appointment that has passed is kept and saves, with no warning.
			rec = save(form("2026-01-15", "2026-11-10", "60", "10"))
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Date has passed") || strings.Contains(rec.Body.String(), "appointment-warnings") {
				t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
			}

			// Invalid dates are rejected without changing anything.
			for _, bad := range []string{"not-a-date", "2026-02-30", "2026-13-01", "2026-12-01T12:00:00Z"} {
				before := stored()
				rec := save(form("2026-01-15", bad, "20", "3"))
				if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Road test appointment") {
					t.Fatalf("%q: status = %d body = %q", bad, rec.Code, rec.Body.String())
				}
				if !reflect.DeepEqual(before, stored()) {
					t.Fatalf("%q changed the stored profile", bad)
				}
			}

			// Clearing the field removes the appointment.
			if rec := save(form("2026-01-15", "", "60", "10")); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Add the date you booked") {
				t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
			}
			if got := stored().RoadTestDate; got != nil {
				t.Fatalf("stored appointment = %v after clearing", got)
			}
		})
	}
}
