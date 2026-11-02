//go:build billing

package cloud

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"hitkeep/appurl"
	authcore "hitkeep/auth"
	"hitkeep/config"
	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

func (h *handler) handleStripeWebhook() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.ctx.Config.CloudHosted {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if strings.TrimSpace(h.ctx.Config.StripeWebhookSecret) == "" {
			http.Error(w, "Webhook signing secret not configured", http.StatusNotImplemented)
			return
		}

		payload, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Unable to read request body", http.StatusBadRequest)
			return
		}

		event, err := h.webhooks.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), h.ctx.Config.StripeWebhookSecret)
		if err != nil {
			if errors.Is(err, errStripeAPIVersionMismatch) {
				shared.LoggerFromContext(r.Context()).Error("Unsupported Stripe webhook API version", slog.Group("stripe",
					"event_id", event.ID,
					"event_type", event.Type,
					"api_version", event.APIVersion,
					"expected_api_version", stripeAPIVersion,
				))
				http.Error(w, "Unsupported webhook API version", http.StatusBadRequest)
				return
			}
			http.Error(w, "Invalid webhook signature", http.StatusBadRequest)
			return
		}

		if err := h.handleStripeEvent(r.Context(), event); err != nil {
			attrs := make([]any, 0, 12)
			attrs = append(attrs,
				"event_type", event.Type,
				"event_id", event.ID,
			)
			attrs = append(attrs, stripeErrorLogAttrs(err)...)
			shared.LoggerFromContext(r.Context()).Error("Failed to process Stripe webhook", slog.Group("stripe", attrs...))
			http.Error(w, "Webhook processing failed", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func (h *handler) handleCreateBillingPortalSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.ctx.Config.CloudHosted {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		activeTenantID, err := h.ctx.Store.GetActiveTenantID(r.Context(), userID)
		if err != nil {
			http.Error(w, "Unable to resolve active team", http.StatusBadRequest)
			return
		}

		if !shared.CheckAgentTeam(w, r, activeTenantID) {
			return
		}

		account, err := h.ctx.Store.GetCloudBillingAccount(r.Context(), activeTenantID)
		if errors.Is(err, database.ErrCloudBillingAccountNotFound) || account == nil {
			newAccount := database.CloudBillingAccount{
				TenantID:           activeTenantID,
				PlanCode:           database.CloudPlanFree,
				PlanName:           planNameForCode(database.CloudPlanFree),
				SubscriptionStatus: database.CloudSubscriptionStatusFree,
			}
			if upsertErr := h.ctx.Store.UpsertCloudBillingAccount(r.Context(), newAccount); upsertErr != nil {
				shared.LoggerFromContext(r.Context()).Error("Failed to auto-create cloud billing account", "error", upsertErr, "team_id", activeTenantID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			account = &newAccount
		}
		if err != nil && !errors.Is(err, database.ErrCloudBillingAccountNotFound) {
			shared.LoggerFromContext(r.Context()).Error("Failed to load cloud billing account", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if strings.TrimSpace(account.StripeCustomerID) == "" {
			http.Error(w, "Stripe customer is not configured", http.StatusConflict)
			return
		}
		if strings.TrimSpace(h.ctx.Config.StripeSecretKey) == "" {
			http.Error(w, "Stripe secret key is not configured", http.StatusNotImplemented)
			return
		}

		var req billingPortalSessionRequest
		if r.Body != nil {
			if err := json.UnmarshalReadOptional(r.Body, &req); err != nil && !errors.Is(err, io.EOF) {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}
		}

		session, err := h.stripe.CreatePortalSession(r.Context(), createPortalSessionInput{
			CustomerID:      account.StripeCustomerID,
			ConfigurationID: strings.TrimSpace(h.ctx.Config.StripePortalConfigurationID),
			ReturnURL:       billingPortalReturnURL(h.ctx.Config),
			Locale:          normalizeStripeLocale(req.Locale),
		})
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to create Stripe billing portal session", slog.Group("stripe", stripeErrorLogAttrs(err)...))
			http.Error(w, "Unable to start billing portal", http.StatusBadGateway)
			return
		}

		shared.WriteJSON(r.Context(), w, http.StatusOK, billingPortalSessionResponse{URL: session.URL})
	}
}

func (h *handler) handleCreateBillingCheckoutSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req billingCheckoutSessionRequest
		if err := json.UnmarshalRead(http.MaxBytesReader(w, r.Body, 64<<10), &req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if session := h.createBillingCheckout(w, r, req); session != nil {
			shared.WriteJSON(r.Context(), w, http.StatusOK, billingCheckoutSessionResponse{URL: session.URL})
		}
	}
}

func (h *handler) createBillingCheckout(w http.ResponseWriter, r *http.Request, req billingCheckoutSessionRequest) *checkoutSessionOutput {
	h.checkoutMu.Lock()
	defer h.checkoutMu.Unlock()
	if h.ctx.Cluster != nil && !h.ctx.Cluster.IsLeader() {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return nil
	}

	if !h.ctx.Config.CloudHosted {
		http.Error(w, "Not found", http.StatusNotFound)
		return nil
	}
	if h.ctx.Store == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return nil
	}

	userID := shared.GetUserIDFromContext(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return nil
	}

	activeTenantID := req.teamID
	if activeTenantID == uuid.Nil {
		var err error
		activeTenantID, err = h.ctx.Store.GetActiveTenantID(r.Context(), userID)
		if err != nil {
			http.Error(w, "Unable to resolve active team", http.StatusBadRequest)
			return nil
		}
	}
	if !shared.CheckAgentTeam(w, r, activeTenantID) {
		return nil
	}
	role, err := h.ctx.Store.GetTenantRole(r.Context(), activeTenantID, userID)
	if err != nil || !authcore.TeamRoleHasCapability(role, authcore.CapTeamManageSettings) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return nil
	}

	account, err := h.ctx.Store.GetCloudBillingAccount(r.Context(), activeTenantID)
	if errors.Is(err, database.ErrCloudBillingAccountNotFound) || account == nil {
		// Auto-create a free billing account for legacy users who signed up
		// before the billing account table existed.
		newAccount := database.CloudBillingAccount{
			TenantID:           activeTenantID,
			PlanCode:           database.CloudPlanFree,
			PlanName:           planNameForCode(database.CloudPlanFree),
			SubscriptionStatus: database.CloudSubscriptionStatusFree,
		}
		if upsertErr := h.ctx.Store.UpsertCloudBillingAccount(r.Context(), newAccount); upsertErr != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to auto-create cloud billing account", "error", upsertErr, "team_id", activeTenantID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return nil
		}
		account = &newAccount
	}
	if err != nil && !errors.Is(err, database.ErrCloudBillingAccountNotFound) {
		shared.LoggerFromContext(r.Context()).Error("Failed to load cloud billing account", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil
	}
	if strings.TrimSpace(h.ctx.Config.StripeSecretKey) == "" {
		http.Error(w, "Stripe secret key is not configured", http.StatusNotImplemented)
		return nil
	}

	req.PlanCode = normalizePlanCode(req.PlanCode)
	req.BillingInterval = normalizeBillingInterval(req.BillingInterval)
	req.Locale = normalizeStripeLocale(req.Locale)
	if req.PlanCode != database.CloudPlanPro && req.PlanCode != database.CloudPlanBusiness {
		http.Error(w, "Paid plan code is required", http.StatusBadRequest)
		return nil
	}

	currentPlanCode, _ := effectivePlanCode(account)
	if currentPlanCode != database.CloudPlanFree {
		http.Error(w, "Use billing portal to manage an existing paid plan", http.StatusConflict)
		return nil
	}

	user, err := h.ctx.Store.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to load user for billing checkout", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil
	}

	team, err := h.ctx.Store.GetTenant(r.Context(), activeTenantID)
	if err != nil || team == nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to load team for billing checkout", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil
	}

	priceID := priceIDForPlan(h.ctx.Config, req.PlanCode, req.BillingInterval)
	if priceID == "" {
		http.Error(w, "Plan is not configured for checkout", http.StatusBadRequest)
		return nil
	}

	customerID := strings.TrimSpace(account.StripeCustomerID)
	if customerID == "" {
		customerID, err = h.stripe.CreateCustomer(r.Context(), createCustomerInput{
			Email:          strings.TrimSpace(strings.ToLower(user.Email)),
			Name:           stripeCustomerName(user, team.Name),
			UserID:         user.ID,
			TenantID:       activeTenantID,
			PlanCode:       req.PlanCode,
			Jurisdiction:   strings.TrimSpace(strings.ToUpper(h.ctx.Config.CloudJurisdiction)),
			IdempotencyKey: stripeCustomerCreateIdempotencyKey(user.ID, activeTenantID, user.Email),
		})
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to create Stripe customer for billing checkout", slog.Group("stripe", stripeErrorLogAttrs(err)...))
			http.Error(w, "Unable to start checkout", http.StatusBadGateway)
			return nil
		}
	}

	if account.StripeCustomerID != customerID {
		account.StripeCustomerID = customerID
		if err := h.ctx.Store.UpsertCloudBillingAccount(r.Context(), *account); err != nil {
			http.Error(w, "Unable to persist billing customer", http.StatusServiceUnavailable)
			return nil
		}
	}

	session, err := h.reuseOrCreateCheckout(r.Context(), createCheckoutSessionInput{
		CustomerID:      customerID,
		PriceID:         priceID,
		SuccessURL:      checkoutSuccessURL(h.ctx.Config),
		CancelURL:       checkoutCancelURLForIntent(h.ctx.Config, req.PlanCode, req.BillingInterval, activeTenantID),
		Locale:          req.Locale,
		UserID:          user.ID,
		TenantID:        activeTenantID,
		PlanCode:        req.PlanCode,
		PlanName:        planNameForCode(req.PlanCode),
		BillingInterval: req.BillingInterval,
		Jurisdiction:    strings.TrimSpace(strings.ToUpper(h.ctx.Config.CloudJurisdiction)),
	})
	if errors.Is(err, errCheckoutConflict) {
		http.Error(w, errCheckoutConflict.Error(), http.StatusConflict)
		return nil
	}
	if err != nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to create Stripe upgrade checkout session", slog.Group("stripe", stripeErrorLogAttrs(err)...))
		http.Error(w, "Unable to start checkout", http.StatusBadGateway)
		return nil
	}

	if err := h.ctx.Store.UpsertCloudBillingAccount(r.Context(), database.CloudBillingAccount{
		TenantID:             activeTenantID,
		PlanCode:             database.CloudPlanFree,
		PlanName:             planNameForCode(database.CloudPlanFree),
		BillingInterval:      req.BillingInterval,
		SubscriptionStatus:   subscriptionStatusPending,
		StripeCustomerID:     customerID,
		StripeSubscriptionID: "",
		StripePriceID:        priceID,
	}); err != nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to persist Stripe upgrade checkout metadata", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil
	}
	if _, err := h.ctx.Store.RecordCloudConversionEvent(r.Context(), database.CloudConversionEvent{
		TenantID:        activeTenantID,
		EventName:       database.CloudConversionCheckoutStarted,
		PlanCode:        req.PlanCode,
		BillingInterval: req.BillingInterval,
		DedupeKey:       activeTenantID.String() + ":checkout:" + session.ID,
	}); err != nil {
		shared.LoggerFromContext(r.Context()).Warn("Failed to record checkout conversion", "error", err, "team_id", activeTenantID, "checkout_session_id", session.ID)
	}

	return session
}

type billingStateResponse struct {
	TeamID             uuid.UUID `json:"team_id"`
	PlanCode           string    `json:"plan_code"`
	SubscriptionStatus string    `json:"subscription_status"`
	PendingPlanCode    string    `json:"pending_plan_code,omitempty"`
	BillingInterval    string    `json:"billing,omitempty"`
}

func (h *handler) handleGetBillingState() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.ctx.Config.CloudHosted {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil || h.ctx.Store == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		teamID, err := h.ctx.Store.GetActiveTenantID(r.Context(), userID)
		if err != nil {
			http.Error(w, "Unable to resolve active team", http.StatusBadRequest)
			return
		}
		if !shared.CheckAgentTeam(w, r, teamID) {
			return
		}

		state := billingStateResponse{TeamID: teamID, PlanCode: database.CloudPlanFree, SubscriptionStatus: database.CloudSubscriptionStatusFree}
		account, err := h.ctx.Store.GetCloudBillingAccount(r.Context(), teamID)
		if err != nil && !errors.Is(err, database.ErrCloudBillingAccountNotFound) {
			http.Error(w, "Unable to load billing state", http.StatusInternalServerError)
			return
		}
		if account != nil {
			state.PlanCode, _ = effectivePlanCode(account)
			state.SubscriptionStatus = account.SubscriptionStatus
			if account.SubscriptionStatus == subscriptionStatusPending {
				state.PendingPlanCode = planCodeForPrice(h.ctx.Config, account.StripePriceID)
				state.BillingInterval = account.BillingInterval
			}
		}
		shared.WriteJSON(r.Context(), w, http.StatusOK, state)
	}
}

func checkoutCancelURLForIntent(conf *config.Config, plan, billing string, teamID uuid.UUID) string {
	parsed, err := url.Parse(checkoutCancelURL(conf))
	if err != nil {
		return checkoutCancelURL(conf)
	}
	query := parsed.Query()
	query.Set("checkout", "canceled")
	query.Set("plan", plan)
	query.Set("billing", billing)
	query.Set("team", teamID.String())
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func billingPortalReturnURL(conf *config.Config) string {
	return appurl.Path(conf.PublicURL, "/admin/team")
}

func normalizeStripeLocale(raw string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(raw, "_", "-"))
	if trimmed == "" {
		return "auto"
	}

	parts := strings.Split(trimmed, "-")
	normalized := make([]string, 0, len(parts))
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return "auto"
		}
		if index == 0 {
			normalized = append(normalized, strings.ToLower(part))
			continue
		}
		if len(part) == 2 {
			normalized = append(normalized, strings.ToUpper(part))
			continue
		}
		normalized = append(normalized, strings.ToLower(part))
	}
	tag := strings.Join(normalized, "-")

	if supported, ok := supportedStripeLocale(tag); ok {
		return supported
	}
	if base, _, found := strings.Cut(tag, "-"); found {
		if supported, ok := supportedStripeLocale(base); ok {
			return supported
		}
	}
	return "auto"
}
