package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
}

func NewDashboardHandler(store repository.Store) *DashboardHandler {
	return &DashboardHandler{store: store, now: time.Now}
}

func (h *DashboardHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	dashboard, err := h.store.GetDashboard(r.Context(), h.now())
	if err != nil {
		http.Error(w, "Unable to load dashboard", http.StatusInternalServerError)
		return
	}
	render(w, r, ui.DashboardPage(dashboard))
}

func (h *DashboardHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Unable to read progress form", http.StatusBadRequest)
		return
	}
	current, err := h.store.GetDashboard(r.Context(), h.now())
	if err != nil {
		http.Error(w, "Unable to load profile", http.StatusInternalServerError)
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

	profile := current.Profile
	profile.TotalHours = totalHours
	profile.NightHours = nightHours
	profile.PermitIssueDate = model.NormalizeDate(r.FormValue("permit_issue_date"))

	profile, err = h.store.UpdateProfile(r.Context(), profile)
	if err != nil {
		http.Error(w, "Unable to save progress", http.StatusInternalServerError)
		return
	}

	updated := model.NewDashboard(profile, current.Requirements, h.now())
	render(w, r, ui.ProgressPanel(updated))
}

func (h *DashboardHandler) UpdateRequirement(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Unable to read requirement form", http.StatusBadRequest)
		return
	}
	key := chi.URLParam(r, "key")
	current, err := h.store.GetDashboard(r.Context(), h.now())
	if err != nil {
		http.Error(w, "Unable to load requirement", http.StatusInternalServerError)
		return
	}
	existing, ok := model.RequirementByKey(current.Requirements, key)
	if !ok {
		http.NotFound(w, r)
		return
	}

	status, ok := model.ParseStatus(r.FormValue("status"))
	if !ok {
		http.Error(w, "Status must be needs_practice or mastered", http.StatusBadRequest)
		return
	}
	existing.Status = status
	existing.MasteredDate = model.NormalizeDate(r.FormValue("mastered_date"))
	existing.Notes = strings.TrimSpace(r.FormValue("notes"))
	if utf8.RuneCountInString(existing.Notes) > maxNotesChars {
		http.Error(w, fmt.Sprintf("Notes must be %d characters or fewer", maxNotesChars), http.StatusBadRequest)
		return
	}
	if existing.Status != model.StatusMastered {
		existing.MasteredDate = nil
	}

	updated, err := h.store.UpdateRequirement(r.Context(), existing)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Unable to save requirement", http.StatusInternalServerError)
		return
	}
	render(w, r, ui.RequirementRow(updated))
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
	if err != nil || parsed < 0 {
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
