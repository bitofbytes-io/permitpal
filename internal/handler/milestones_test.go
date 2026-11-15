package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/middleware"
)

func TestProfileSaveCelebratesOnlyTheSaveThatCrossesAMilestone(t *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			at := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
			store := validationStore(t, backend, at)
			driver, err := store.EnsureDriver(context.Background(), "milestones", at)
			if err != nil {
				t.Fatal(err)
			}
			h := NewDashboardHandler(store, time.UTC)
			h.now = func() time.Time { return at }
			submit := func(total, night string) string {
				t.Helper()
				form := url.Values{"total_hours": {total}, "night_hours": {night}, "permit_issue_date": {"2025-01-01"}}
				request := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				rec := httptest.NewRecorder()
				h.UpdateProfile(rec, request.WithContext(middleware.WithDriver(request.Context(), driver)))
				if rec.Code != http.StatusOK {
					t.Fatalf("save %s/%s: %d %s", total, night, rec.Code, rec.Body.String())
				}
				return rec.Body.String()
			}
			if body := submit("5", "0"); strings.Contains(body, "milestone-banner") {
				t.Fatalf("save below every line celebrated: %s", body)
			}
			at = at.AddDate(0, 0, 1)
			body := submit("12", "0")
			if !strings.Contains(body, "milestone-banner") || !strings.Contains(body, "10 total hours") || !strings.Contains(body, `datetime="2026-01-06"`) {
				t.Fatalf("crossing 10 hours was not celebrated and dated: %s", body)
			}
			if strings.Contains(submit("12", "0"), "milestone-banner") {
				t.Fatal("resubmitting celebrated again")
			}
			// Dropping back below the line removes the badge; crossing again
			// celebrates again, with the new date.
			at = at.AddDate(0, 0, 1)
			if body := submit("8", "0"); strings.Contains(body, "milestone-banner") || strings.Contains(body, `datetime="2026-01-06"`) {
				t.Fatalf("correction kept the badge or celebrated: %s", body)
			}
			at = at.AddDate(0, 0, 1)
			if body := submit("10", "0"); !strings.Contains(body, "milestone-banner") || !strings.Contains(body, `datetime="2026-01-08"`) {
				t.Fatalf("re-crossing was not celebrated with the new date: %s", body)
			}
		})
	}
}
