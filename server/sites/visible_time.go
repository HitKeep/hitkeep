package sites

import (
	"net/http"

	"hitkeep/api"
	"hitkeep/server/shared"
)

func (h *handler) handleGetSiteVisibleTime() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		siteID, ok := shared.PathUUID(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}
		window, ok := parseAnalyticsWindow(w, r)
		if !ok {
			return
		}
		q := r.URL.Query()
		goalIDs, err := parseHitUUIDQueryParam(q, "goal_id")
		if err != nil {
			http.Error(w, "Invalid goal_id", http.StatusBadRequest)
			return
		}
		funnelIDs, err := parseHitUUIDQueryParam(q, "funnel_id")
		if err != nil {
			http.Error(w, "Invalid funnel_id", http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		store, err := h.ctx.AnalyticsStore(ctx, siteID)
		if err != nil {
			shared.LoggerFromContext(ctx).Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		pages, err := store.GetVisibleTimePages(ctx, api.AnalyticsParams{
			SiteID:    siteID,
			Start:     window.Start,
			End:       window.End,
			Filters:   window.Filters,
			GoalIDs:   goalIDs,
			FunnelIDs: funnelIDs,
		})
		if err != nil {
			shared.LoggerFromContext(ctx).Error("Failed to get visible time by page", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		shared.WriteJSON(ctx, w, http.StatusOK, pages)
	}
}
