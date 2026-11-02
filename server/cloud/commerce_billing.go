//go:build billing

package cloud

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/appurl"
	"hitkeep/auth"
	"hitkeep/config"
	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/oauth"
	"hitkeep/server/shared"
)

const acpVersion = "2026-04-17"
const ucpVersion = "2026-08-25"

// CommerceAvailable reports the configured human-approved Checkout adapter, not a payment rail.
func CommerceAvailable(c *config.Config) bool {
	return c != nil && c.AIEnabled && c.CloudHosted && strings.TrimSpace(c.StripeSecretKey) != "" && (c.StripePriceProMonthly != "" || c.StripePriceProAnnual != "" || c.StripePriceBusinessMonthly != "" || c.StripePriceBusinessAnnual != "")
}

func (h *handler) registerCommerce(mux *http.ServeMux) {
	access := shared.HandlerConfig{RequireAuth: true, HumanOnly: true, OAuthScope: oauth.ScopeBillingWrite, ActiveTeamCap: auth.CapTeamManageSettings, RateLimiter: h.ctx.ApiLimiter}
	for _, protocol := range []string{"acp", "ucp"} {
		path := "/api/cloud/commerce/" + protocol + "/checkout_sessions"
		update := "POST"
		if protocol == "ucp" {
			path = "/api/cloud/commerce/ucp/checkout-sessions"
			update = "PUT"
		}
		for _, route := range []struct{ pattern, action string }{{"POST " + path, "create"}, {"GET " + path + "/{id}", "get"}, {update + " " + path + "/{id}", "update"}, {"POST " + path + "/{id}/complete", "complete"}, {"POST " + path + "/{id}/cancel", "cancel"}} {
			mux.HandleFunc(route.pattern, h.commerceGate(h.ctx.Handler(access, h.handleCommerceSession(protocol, route.action))))
		}
	}
	for _, rail := range []string{"mpp", "x402"} {
		mux.HandleFunc("POST /api/cloud/commerce/"+rail+"/purchases/{id}/pay", h.commerceGate(h.ctx.Handler(access, func(w http.ResponseWriter, r *http.Request) {
			shared.WriteJSON(r.Context(), w, http.StatusNotImplemented, map[string]any{"error": "payment_rail_unavailable", "protocol": rail, "reason": "This build has no native payment-proof and settlement adapter for this rail. Merchant payment configuration is also absent. Use the approved Checkout flow."})
		})))
	}
}

func (h *handler) commerceGate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		if !h.ctx.Config.AIEnabled || !h.ctx.Config.CloudHosted {
			http.NotFound(w, r)
			return
		}
		if h.ctx.Store == nil || (h.ctx.Cluster != nil && !h.ctx.Cluster.IsLeader()) {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		if !CommerceAvailable(h.ctx.Config) || h.stripe == nil {
			http.Error(w, "Checkout is not configured", http.StatusServiceUnavailable)
			return
		}
		next(w, r)
	}
}

type commerceRequest struct {
	LineItems []struct {
		ID   string `json:"id"`
		Item struct {
			ID string `json:"id"`
		} `json:"item"`
		Quantity *int `json:"quantity"`
	} `json:"line_items"`
	UCP struct {
		Version      string         `json:"version"`
		Capabilities map[string]any `json:"capabilities"`
	} `json:"ucp"`
}

func (h *handler) handleCommerceSession(protocol, action string) http.HandlerFunc {
	return h.commerceGate(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(shared.APIClientAuthKey) != nil {
			http.Error(w, "User-delegated billing authorization required", http.StatusUnauthorized)
			return
		}
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if protocol == "acp" && r.Header.Get("API-Version") != acpVersion {
			http.Error(w, "Unsupported API-Version", http.StatusBadRequest)
			return
		}
		if protocol == "ucp" {
			if r.Header.Get("Request-Id") == "" || len(r.Header.Get("Request-Id")) > 255 {
				http.Error(w, "Request-Id required", http.StatusBadRequest)
				return
			}
			w.Header().Set("Request-Id", r.Header.Get("Request-Id"))
			check := h.profileCheck
			if check == nil {
				check = fetchCommerceProfile
			}
			if status, err := check(r.Context(), r.Header.Get("UCP-Agent")); err != nil {
				http.Error(w, err.Error(), status)
				return
			}
		}
		var req commerceRequest
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
		if err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}
		}
		if protocol == "ucp" && req.UCP.Version != "" && req.UCP.Version != ucpVersion {
			http.Error(w, "Unsupported UCP version", http.StatusBadRequest)
			return
		}
		for name := range req.UCP.Capabilities {
			if name != "dev.ucp.shopping.checkout" {
				http.Error(w, "Unsupported UCP capability", http.StatusBadRequest)
				return
			}
		}
		teamID, err := h.ctx.ResolveRequestTeamID(r)
		if err != nil {
			http.Error(w, "Unable to resolve billing team", http.StatusBadRequest)
			return
		}
		if !shared.CheckAgentTeam(w, r, teamID) {
			return
		}
		role, err := h.ctx.Store.GetTenantRole(r.Context(), teamID, userID)
		if err != nil || !auth.TeamRoleHasCapability(role, auth.CapTeamManageSettings) {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
		h.commerceMu.Lock()
		defer h.commerceMu.Unlock()
		eventID := ""
		if action != "get" {
			key := r.Header.Get("Idempotency-Key")
			if key == "" || len(key) > 255 {
				http.Error(w, "Idempotency-Key required (maximum 255 characters)", http.StatusBadRequest)
				return
			}
			w.Header().Set("Idempotency-Key", key)
			eventID = fmt.Sprintf("agent_checkout_%x", sha256.Sum256([]byte(teamID.String()+":"+protocol+":"+action+":"+r.PathValue("id")+":"+key)))
			fingerprint := fmt.Sprintf("%x", sha256.Sum256(body))
			inserted, err := h.ctx.Store.CreateCloudBillingEvent(r.Context(), database.CloudBillingEvent{StripeEventID: eventID, TenantID: teamID, EventType: "agent.checkout.request", Payload: fingerprint})
			if err != nil {
				http.Error(w, "Unable to reserve request", http.StatusServiceUnavailable)
				return
			}
			if !inserted {
				prior, err := h.ctx.Store.GetCloudBillingEvent(r.Context(), eventID)
				if err != nil {
					http.Error(w, "Unable to load request", http.StatusServiceUnavailable)
					return
				}
				if prior.Payload != fingerprint {
					status := http.StatusConflict
					if protocol == "acp" {
						status = http.StatusUnprocessableEntity
					}
					http.Error(w, "Idempotency key reused with a different request", status)
					return
				}
				result, err := h.ctx.Store.GetCloudBillingEvent(r.Context(), eventID+"_result")
				if err != nil && !errors.Is(err, database.ErrCloudBillingEventNotFound) {
					http.Error(w, "Unable to load request result", http.StatusServiceUnavailable)
					return
				}
				if result != nil {
					var cached checkoutSessionOutput
					if err := json.Unmarshal([]byte(result.Payload), &cached); err != nil {
						http.Error(w, "Unable to load request result", http.StatusServiceUnavailable)
						return
					}
					status := http.StatusOK
					if action == "create" {
						status = http.StatusCreated
					}
					w.Header().Set("Idempotent-Replayed", "true")
					shared.WriteJSON(r.Context(), w, status, h.commerceResponse(protocol, &cached))
					return
				}
			}
		}
		var session *checkoutSessionOutput
		status := http.StatusOK
		itemID := ""
		quantity := 0
		if protocol == "acp" && len(req.LineItems) == 1 && req.LineItems[0].Quantity == nil {
			itemID = req.LineItems[0].ID
			quantity = 1
		}
		if protocol == "ucp" && len(req.LineItems) == 1 && req.LineItems[0].Quantity != nil {
			itemID = req.LineItems[0].Item.ID
			quantity = *req.LineItems[0].Quantity
		}
		if action == "create" {
			plan, interval, ok := strings.Cut(itemID, ":")
			if !ok || quantity != 1 || (plan != database.CloudPlanPro && plan != database.CloudPlanBusiness) || (interval != database.CloudBillingIntervalMonthly && interval != database.CloudBillingIntervalAnnual) {
				http.Error(w, "One paid plan item (pro:monthly, pro:annual, business:monthly, business:annual) is required", http.StatusBadRequest)
				return
			}
			price, err := h.stripe.GetPrice(r.Context(), priceIDForPlan(h.ctx.Config, plan, interval))
			currency := "eur"
			if shared.NormalizeCloudJurisdiction(h.ctx.Config.CloudJurisdiction) == "US" {
				currency = "usd"
			}
			cadence := "month"
			if interval == database.CloudBillingIntervalAnnual {
				cadence = "year"
			}
			if err != nil || price == nil || !price.Active || price.AmountMinor <= 0 || !strings.EqualFold(price.Currency, currency) || price.Interval != cadence {
				http.Error(w, "Plan price is unavailable", http.StatusConflict)
				return
			}
			session = h.createBillingCheckout(w, r, billingCheckoutSessionRequest{teamID: teamID, PlanCode: plan, BillingInterval: interval})
			if session == nil {
				return
			}
			status = http.StatusCreated
		} else {
			id := r.PathValue("id")
			if !strings.HasPrefix(id, "cs_") || len(id) > 255 {
				http.NotFound(w, r)
				return
			}
			session, err = h.stripe.GetCheckoutSession(r.Context(), id)
			if err != nil || session == nil {
				http.Error(w, "Unable to retrieve checkout", http.StatusBadGateway)
				return
			}
			account, err := h.ctx.Store.GetCloudBillingAccount(r.Context(), teamID)
			if err != nil || account == nil || session.TenantID != teamID.String() || session.CustomerID != account.StripeCustomerID {
				http.NotFound(w, r)
				return
			}
			if action == "update" && len(req.LineItems) > 0 && (quantity != 1 || itemID != session.PlanCode+":"+session.BillingInterval) {
				http.Error(w, "Cancel the existing checkout before changing the plan", http.StatusConflict)
				return
			}
			if action == "cancel" && session.Status != "expired" {
				if session.Status != "open" {
					http.Error(w, "Checkout cannot be canceled", http.StatusMethodNotAllowed)
					return
				}
				session, err = h.stripe.ExpireCheckoutSession(r.Context(), id, eventID)
				if err != nil {
					http.Error(w, "Unable to cancel checkout", http.StatusBadGateway)
					return
				}
			}
		}
		if eventID != "" {
			saved, err := json.Marshal(session)
			if err != nil {
				http.Error(w, "Unable to encode checkout", http.StatusInternalServerError)
				return
			}
			if _, err := h.ctx.Store.CreateCloudBillingEvent(r.Context(), database.CloudBillingEvent{StripeEventID: eventID + "_result", TenantID: teamID, EventType: "agent.checkout.result", Payload: string(saved), ProcessingStatus: database.CloudBillingEventStatusDone}); err != nil {
				http.Error(w, "Checkout result pending reconciliation; retry the same request", http.StatusServiceUnavailable)
				return
			}
		}
		shared.WriteJSON(r.Context(), w, status, h.commerceResponse(protocol, session))
	})
}

func commerceSessionStatus(s *checkoutSessionOutput) string {
	switch s.Status {
	case "complete":
		if s.PaymentStatus == "paid" {
			return "completed"
		}
		return "complete_in_progress"
	case "expired":
		return "canceled"
	case "open":
		return "requires_escalation"
	default:
		return "incomplete"
	}
}

func (h *handler) commerceResponse(protocol string, s *checkoutSessionOutput) map[string]any {
	status := commerceSessionStatus(s)
	itemID := s.PlanCode + ":" + s.BillingInterval
	total := map[string]any{"type": "total", "display_text": "Total", "amount": s.AmountTotal}
	line := map[string]any{"id": itemID, "item": map[string]any{"id": itemID}, "quantity": 1, "totals": []any{total}}
	response := map[string]any{"id": s.ID, "status": status, "currency": s.Currency, "line_items": []any{line}, "totals": []any{total}, "messages": []any{}, "links": []any{}}
	if s.ExpiresAt > 0 {
		response["expires_at"] = time.Unix(s.ExpiresAt, 0).UTC().Format(time.RFC3339)
	}
	if protocol == "acp" {
		response["protocol"] = map[string]any{"version": acpVersion}
		response["capabilities"] = map[string]any{}
		response["fulfillment_options"] = []any{}
	}
	if protocol == "ucp" {
		response["currency"] = strings.ToUpper(s.Currency)
		response["ucp"] = map[string]any{"version": ucpVersion, "payment_handlers": map[string]any{}, "capabilities": map[string]any{"dev.ucp.shopping.checkout": []any{map[string]any{"version": ucpVersion}}}}
		line["item"] = map[string]any{"id": itemID, "title": planNameForCode(s.PlanCode) + " (" + s.BillingInterval + ")", "price": s.AmountSubtotal}
	}
	if status == "requires_escalation" {
		response["continue_url"] = s.URL
		message := map[string]any{"type": "error", "code": "requires_sign_in", "content": "A billing administrator must approve the subscription and payment in Checkout.", "content_type": "plain", "resolution": "requires_buyer_review"}
		if protocol == "ucp" {
			delete(message, "resolution")
			delete(message, "content_type")
			message["severity"] = "requires_buyer_review"
			response["currency"] = strings.ToUpper(s.Currency)
		}
		response["messages"] = []any{message}
	}
	if status == "completed" {
		id := s.SubscriptionID
		if id == "" {
			id = s.ID
		}
		response["order"] = map[string]any{"id": id, "checkout_session_id": s.ID, "permalink_url": appurl.Path(h.ctx.Config.PublicURL, "/admin/team")}
		if protocol == "ucp" {
			delete(response["order"].(map[string]any), "checkout_session_id")
		}
	}
	return response
}
