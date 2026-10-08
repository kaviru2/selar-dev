package handler

import (
	"context"

	"github.com/selar-dev/selar-api/internal/settings"
)

const suggestionsOnOpenSetting = "suggestions.show_on_open"

// suggestionsEnabled returns whether the study condition permits suggestions
// for the current learner. A false cohort lock gates the server response;
// without that lock, the existing per-user behaviour remains unchanged.
func (h *Handler) suggestionsEnabled(ctx context.Context, userID string) (bool, error) {
	stored, _, locks, err := h.store.GetUserSettings(ctx, userID)
	if err != nil {
		return false, err
	}
	values, locked := settings.Effective(stored, locks)
	for _, key := range locked {
		if key != suggestionsOnOpenSetting {
			continue
		}
		enabled, ok := values[suggestionsOnOpenSetting].(bool)
		if !ok {
			return true, nil
		}
		return enabled, nil
	}
	return true, nil
}
