package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/drywaters/permitpal/internal/middleware"
	"github.com/drywaters/permitpal/internal/model"
	"github.com/drywaters/permitpal/internal/repository"
	"github.com/drywaters/permitpal/internal/ui"
	"github.com/go-chi/chi/v5"
)

const (
	maxTotalHours = 60
	maxNightHours = 10
	maxNotesChars = 1000
)

type DashboardHandler struct {
	store repository.Store
	now   func() time.Time
	loc   *time.Location
}

// NewDashboardHandler uses loc to decide today's date and to parse submitted dates.
func NewDashboardHandler(store repository.Store, loc *time.Location) *DashboardHandler {
	return &DashboardHandler{store: store, now: time.Now, loc: loc}
}

func (h *DashboardHandler) localNow() time.Time {
	return h.now().In(h.loc)
}

// isFuture reports whether date falls after today in the app's time zone.
func (h *DashboardHandler) isFuture(date *time.Time) bool {
	if date == nil {
		return false
	}
	year, month, day := h.localNow().Date()
	return date.After(time.Date(year, month, day, 0, 0, 0, 0, h.loc))
}

func (h *DashboardHandler) dashboard(driver model.Driver, tracker model.Tracker) model.Dashboard {
	return model.NewDashboard(driver, tracker.Profile, tracker.Requirements, h.localNow())
}

// These handlers run behind RequireAuth, which always puts the driver in the context.

func (h *DashboardHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	driver, _ := middleware.DriverFromContext(r.Context())
	tracker, err := h.store.GetTracker(r.Context(), driver.ID)
	if err != nil {
		middleware.ServerError(w, r, "Unable to load dashboard", err)
		return
	}
	render(w, r, ui.DashboardPage(h.dashboard(driver, tracker)))
}

func (h *DashboardHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	driver, _ := middleware.DriverFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Unable to read progress form", http.StatusBadRequest)
		return
	}
	totalHours, err := parseHours(r.FormValue("total_hours"), maxTotalHours)
	if err != nil {
		http.Error(w, fmt.Sprintf("Total hours must be a number from 0 to %d", maxTotalHours), http.StatusBadRequest)
		return
	}
	nightHours, err := parseHours(r.FormValue("night_hours"), maxNightHours)
	if err != nil {
		http.Error(w, fmt.Sprintf("Night hours must be a number from 0 to %d", maxNightHours), http.StatusBadRequest)
		return
	}
	if nightHours > totalHours {
		http.Error(w, "Night hours cannot be more than total hours", http.StatusBadRequest)
		return
	}

	permitIssueDate, err := model.ParseDate(r.FormValue("permit_issue_date"), h.loc)
	if err != nil {
		http.Error(w, "Permit issue date must be a valid date in YYYY-MM-DD format", http.StatusBadRequest)
		return
	}
	if h.isFuture(permitIssueDate) {
		http.Error(w, "Permit issue date cannot be in the future", http.StatusBadRequest)
		return
	}

	tracker, err := h.store.GetTracker(r.Context(), driver.ID)
	if err != nil {
		middleware.ServerError(w, r, "Unable to load profile", err)
		return
	}

	profile := tracker.Profile
	profile.TotalHours = totalHours
	profile.NightHours = nightHours
	profile.PermitIssueDate = permitIssueDate

	tracker.Profile, err = h.store.UpdateProfile(r.Context(), driver.ID, profile)
	if err != nil {
		middleware.ServerError(w, r, "Unable to save progress", err)
		return
	}

	slog.Info("profile updated", "total_hours", tracker.Profile.TotalHours, "night_hours", tracker.Profile.NightHours, "has_permit_issue_date", tracker.Profile.PermitIssueDate != nil)
	render(w, r, ui.ProgressPanelWithMessage(h.dashboard(driver, tracker), "Progress saved"))
}

func (h *DashboardHandler) UpdateRequirement(w http.ResponseWriter, r *http.Request) {
	driver, _ := middleware.DriverFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Unable to read requirement form", http.StatusBadRequest)
		return
	}
	ratingValue := r.FormValue("rating")
	if r.FormValue("clear_rating") == "true" {
		ratingValue = string(model.RatingNotRated)
	}
	rating, ok := model.ParseRating(ratingValue)
	if !ok {
		http.Error(w, "Rating must be not_rated, bad, fair, or good", http.StatusBadRequest)
		return
	}
	ratedOn, err := model.ParseDate(r.FormValue("rated_on"), h.loc)
	if err != nil {
		http.Error(w, "Last rated date must be a valid date in YYYY-MM-DD format", http.StatusBadRequest)
		return
	}
	// Clearing discards the date, so a future date saved before this check
	// must not block it.
	if rating != model.RatingNotRated && h.isFuture(ratedOn) {
		http.Error(w, "Last rated date cannot be in the future", http.StatusBadRequest)
		return
	}
	notes := strings.TrimSpace(r.FormValue("notes"))
	if utf8.RuneCountInString(notes) > maxNotesChars {
		http.Error(w, fmt.Sprintf("Notes must be %d characters or fewer", maxNotesChars), http.StatusBadRequest)
		return
	}
	key := chi.URLParam(r, "key")
	tracker, err := h.store.GetTracker(r.Context(), driver.ID)
	if err != nil {
		middleware.ServerError(w, r, "Unable to load requirement", err)
		return
	}
	existing, ok := model.RequirementByKey(tracker.Requirements, key)
	if !ok {
		http.NotFound(w, r)
		return
	}

	existing.Rating = rating
	existing.RatedOn = ratedOn
	existing.Notes = notes
	if existing.Rating == model.RatingNotRated {
		existing.RatedOn = nil
	} else if existing.RatedOn == nil {
		date, _ := model.ParseDate(h.localNow().Format("2006-01-02"), h.loc)
		existing.RatedOn = date
	}

	updated, err := h.store.UpdateRequirement(r.Context(), driver.ID, existing)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		middleware.ServerError(w, r, "Unable to save requirement", err)
		return
	}
	slog.Info("requirement updated", "requirement", updated.Key, "status", updated.Rating, "has_rated_on", updated.RatedOn != nil)
	tracker.Requirements = replaceRequirement(tracker.Requirements, updated)
	render(w, r, ui.RequirementUpdate(updated, h.dashboard(driver, tracker)))
}

func parseHours(value string, max float64) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	if strings.ContainsAny(value, "eE") || decimalPlaces(value) > 1 {
		return 0, errors.New("invalid hours")
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 {
		return 0, errors.New("invalid hours")
	}
	if parsed > max {
		return 0, errors.New("hours exceed maximum")
	}
	return parsed, nil
}

func decimalPlaces(value string) int {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return 0
	}
	return len(parts[1])
}

func replaceRequirement(requirements []model.Requirement, updated model.Requirement) []model.Requirement {
	for i, req := range requirements {
		if req.Key == updated.Key {
			requirements[i] = updated
			break
		}
	}
	return requirements
}
