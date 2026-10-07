package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/ocnaibill/codice/backend/internal/people"
	"github.com/ocnaibill/codice/backend/internal/profile"
	"github.com/ocnaibill/codice/backend/internal/reading"
)

// PeopleHandler lets owner and admin review people who may be the same person ("Herbert, Frank" and "Frank
// Herbert", #36). The system only proposes; nothing is merged without a decision.
type PeopleHandler struct {
	DB *sql.DB
}

// List returns the pairs waiting for a decision.
func (h *PeopleHandler) List(w http.ResponseWriter, r *http.Request) {
	pairs, err := people.ListPending(r.Context(), h.DB)
	if err != nil {
		log.Println("Error listing people that may be the same:", err)
		http.Error(w, "Error listing people", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": pairs})
}

func mergeID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil && id > 0
}

// Dismiss records that the two are not the same person.
func (h *PeopleHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	id, ok := mergeID(r)
	if !ok {
		http.Error(w, "Candidate not found", http.StatusNotFound)
		return
	}
	switch err := people.Dismiss(r.Context(), h.DB, id, currentUserID(r)); {
	case errors.Is(err, people.ErrNotFound):
		http.Error(w, "Candidate not found", http.StatusNotFound)
	case err != nil:
		log.Println("Error dismissing a person pair:", err)
		http.Error(w, "Error recording the decision", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// Merge makes the two people one. It cannot be undone, so it needs the person to keep and an explicit
// confirmation.
func (h *PeopleHandler) Merge(w http.ResponseWriter, r *http.Request) {
	id, ok := mergeID(r)
	if !ok {
		http.Error(w, "Candidate not found", http.StatusNotFound)
		return
	}
	var req struct {
		Keep    int  `json:"keep"`
		Confirm bool `json:"confirm"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.Keep <= 0 || !req.Confirm {
		http.Error(w, "keep (the person who stays) and confirm=true are required", http.StatusBadRequest)
		return
	}
	switch err := people.Merge(r.Context(), h.DB, id, req.Keep, currentUserID(r)); {
	case errors.Is(err, people.ErrNotFound):
		http.Error(w, "Candidate not found", http.StatusNotFound)
	case errors.Is(err, people.ErrBadKeep):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case err != nil:
		log.Println("Error merging people:", err)
		http.Error(w, "Error merging the people", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// GetPreferences tells an account how names are shown to it (#64): its own choice, the library's default, and
// what applies.
func (h *PeopleHandler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	h.writePreferences(w, r)
}

// preferencesResponse is what an account has chosen: how names are shown, and how the text of a book looks (#106). Reader
// is null until the person has made a choice.
type preferencesResponse struct {
	people.Preference
	Reader *reading.Settings `json:"reader"`
	// DisplayName is how the person wants to be called ("" for the user name), and DisplayNameAsked whether they were asked.
	DisplayName      string `json:"displayName"`
	DisplayNameAsked bool   `json:"displayNameAsked"`
}

func (h *PeopleHandler) writePreferences(w http.ResponseWriter, r *http.Request) {
	userID := currentUserID(r)
	saved, err := reading.Get(r.Context(), h.DB, userID)
	if err != nil {
		log.Println("Error reading the reading preferences:", err)
		http.Error(w, "Error reading the preferences", http.StatusInternalServerError)
		return
	}
	called, err := profile.Get(r.Context(), h.DB, userID)
	if err != nil {
		log.Println("Error reading the display name:", err)
		http.Error(w, "Error reading the preferences", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, preferencesResponse{Preference: people.OrderFor(r.Context(), h.DB, userID), Reader: saved,
		DisplayName: called.Display, DisplayNameAsked: called.Asked})
}

// SetPreferences records an account's own choices: how names are shown (empty goes back to the library's) and how the text of
// a book looks (null takes the choice away). What the request does not mention stays as it is.
func (h *PeopleHandler) SetPreferences(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NameOrder   *string         `json:"nameOrder"`
		Reader      json.RawMessage `json:"reader"`
		DisplayName *string         `json:"displayName"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || (req.NameOrder != nil && *req.NameOrder != "" && !people.ValidOrder(*req.NameOrder)) {
		http.Error(w, "nameOrder is given_first, family_first or empty", http.StatusBadRequest)
		return
	}
	if req.DisplayName != nil {
		if _, err := profile.CleanDisplayName(*req.DisplayName); err != nil {
			http.Error(w, "displayName is at most 60 characters", http.StatusBadRequest)
			return
		}
	}
	var choice *reading.Settings
	if len(req.Reader) > 0 && string(req.Reader) != "null" {
		parsed, err := reading.Parse(req.Reader)
		if err != nil {
			http.Error(w, "reader is a choice of the lists of the reader", http.StatusBadRequest)
			return
		}
		choice = &parsed
	}
	userID := currentUserID(r)
	if req.NameOrder != nil {
		if err := people.SetChoice(r.Context(), h.DB, userID, *req.NameOrder); err != nil {
			log.Println("Error saving a name order:", err)
			http.Error(w, "Error saving the preference", http.StatusInternalServerError)
			return
		}
	}
	if req.DisplayName != nil {
		if err := profile.Set(r.Context(), h.DB, userID, *req.DisplayName); err != nil {
			log.Println("Error saving a display name:", err)
			http.Error(w, "Error saving the preference", http.StatusInternalServerError)
			return
		}
	}
	if len(req.Reader) > 0 {
		if err := reading.Set(r.Context(), h.DB, userID, choice); err != nil {
			log.Println("Error saving the reading preferences:", err)
			http.Error(w, "Error saving the preference", http.StatusInternalServerError)
			return
		}
	}
	h.writePreferences(w, r)
}

// SetLibraryOrder records the library's default (the owner's).
func (h *PeopleHandler) SetLibraryOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NameOrder string `json:"nameOrder"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || !people.ValidOrder(req.NameOrder) {
		http.Error(w, "nameOrder is given_first or family_first", http.StatusBadRequest)
		return
	}
	if err := people.SetLibraryOrder(r.Context(), h.DB, req.NameOrder, currentUserID(r)); err != nil {
		log.Println("Error saving the library name order:", err)
		http.Error(w, "Error saving the preference", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, people.OrderFor(r.Context(), h.DB, currentUserID(r)))
}

// SetName corrects, by hand, which words of a person's name are the surname and which are the given names, or
// takes the division away (empty). The words have to be the ones the name already has (#64): this says which is
// which, it does not rename anyone.
func (h *PeopleHandler) SetName(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	var req struct {
		Family string `json:"family"`
		Given  string `json:"given"`
	}
	if !ok || json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req) != nil {
		http.Error(w, "family and given are the parts of the name", http.StatusBadRequest)
		return
	}
	switch err := people.SetParts(r.Context(), h.DB, id, req.Family, req.Given, currentUserID(r)); {
	case errors.Is(err, people.ErrNotFound):
		http.Error(w, "Person not found", http.StatusNotFound)
	case errors.Is(err, people.ErrNotTheName):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case err != nil:
		log.Println("Error correcting a name:", err)
		http.Error(w, "Error correcting the name", http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
