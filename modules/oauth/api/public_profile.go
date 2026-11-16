package httpapi

import (
	"errors"
	"net/http"

	"github.com/AlbaTextus/atnexus/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Deliberately separate from userView: account credentials and email are private.
func (a *API) publicProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "profile_not_found"})
		return
	}
	user, err := a.Store.UserByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || err == nil && !user.IsActive {
		writeJSON(w, 404, map[string]string{"error": "profile_not_found"})
		return
	}
	if err != nil {
		a.ApplicationError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"id": user.ID, "numeric_id": user.NumericID, "display_name": user.DisplayName,
		"avatar_url": user.AvatarURL, "background_url": user.BackgroundURL, "bio": user.Bio,
		"level": store.RoleLevel(user.Role), "created_at": user.CreatedAt,
	})
}
