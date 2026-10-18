//go:build billing

package cloud

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stripe/stripe-go/v87/webhook"

	"hitkeep/database"
)

func TestStripeWebhookInvoiceRelevance(t *testing.T) {
	for _, tc := range []struct {
		name        string
		version     string
		invoice     string
		badSecret   bool
		wantStatus  int
		wantPastDue bool
	}{
		{"legacy manual invoice", "2026-02-25.clover", `{"billing_reason":"manual","customer":"cus_invoice","subscription":null}`, false, http.StatusOK, false},
		{"current manual invoice", stripeAPIVersion, `{"billing_reason":"manual","customer":"cus_invoice","parent":null}`, false, http.StatusOK, false},
		{"invalid signature on manual invoice", "2026-02-25.clover", `{"billing_reason":"manual","customer":"cus_invoice"}`, true, http.StatusBadRequest, false},
		{"legacy subscription invoice", "2026-02-25.clover", `{"billing_reason":"subscription_cycle","customer":"cus_invoice","subscription":"sub_invoice"}`, false, http.StatusBadRequest, false},
		{"manual invoice with subscription", "2026-02-25.clover", `{"billing_reason":"manual","customer":"cus_invoice","subscription":"sub_invoice"}`, false, http.StatusBadRequest, false},
		{"manual invoice with subscription parent", "2026-02-25.clover", `{"billing_reason":"manual","customer":"cus_invoice","parent":{"type":"subscription_details","subscription_details":{"subscription":{"id":"sub_invoice"}}}}`, false, http.StatusBadRequest, false},
		{"current subscription invoice", stripeAPIVersion, `{"billing_reason":"subscription_cycle","customer":"cus_invoice","parent":{"type":"subscription_details","subscription_details":{"subscription":"sub_invoice"}}}`, false, http.StatusOK, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, store, _ := setupSignedCloudWebhookTestHandler(t)
			t.Cleanup(func() { _ = store.Close() })
			account, err := store.CreateManagedCloudAccount(t.Context(), database.CreateManagedCloudAccountInput{
				Email: "invoice-relevance@example.com", HashedPassword: "hashed", TeamName: "Invoice Team",
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.UpsertCloudBillingAccount(t.Context(), database.CloudBillingAccount{
				TenantID: account.TenantID, PlanCode: database.CloudPlanPro, PlanName: "Pro",
				SubscriptionStatus: database.CloudSubscriptionStatusActive,
				StripeCustomerID:   "cus_invoice", StripeSubscriptionID: "sub_previous", StripePriceID: "price_pro",
			}); err != nil {
				t.Fatal(err)
			}

			payload := []byte(fmt.Sprintf(`{"id":"evt_invoice_relevance","object":"event","api_version":%q,"type":"invoice.payment_failed","data":{"object":%s}}`, tc.version, tc.invoice))
			secret := "whsec_test_123"
			if tc.badSecret {
				secret = "whsec_wrong"
			}
			signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: secret})
			for range 2 {
				req := httptest.NewRequest(http.MethodPost, "/api/cloud/webhooks/stripe", bytes.NewReader(payload))
				req.Header.Set("Stripe-Signature", signed.Header)
				response := httptest.NewRecorder()
				h.handleStripeWebhook().ServeHTTP(response, req)
				if response.Code != tc.wantStatus {
					t.Errorf("HTTP status = %d, want %d", response.Code, tc.wantStatus)
				}
				if tc.wantStatus == http.StatusBadRequest {
					wantError := "Unsupported webhook API version"
					if tc.badSecret {
						wantError = "Invalid webhook signature"
					}
					if got := strings.TrimSpace(response.Body.String()); got != wantError {
						t.Errorf("HTTP error = %q, want %q", got, wantError)
					}
				}
			}
			billing, err := store.GetCloudBillingAccount(t.Context(), account.TenantID)
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := database.CloudSubscriptionStatusActive
			wantSubscription := "sub_previous"
			if tc.wantPastDue {
				wantStatus = database.CloudSubscriptionStatusPastDue
				wantSubscription = "sub_invoice"
			}
			if billing.SubscriptionStatus != wantStatus || billing.StripeSubscriptionID != wantSubscription {
				t.Errorf("billing state = %s / %s, want %s / %s", billing.SubscriptionStatus, billing.StripeSubscriptionID, wantStatus, wantSubscription)
			}
		})
	}
}

func TestStripeWebhookStoreFailure(t *testing.T) {
	for _, tc := range []struct {
		name       string
		version    string
		invoice    string
		wantStatus int
	}{
		{"unrelated manual invoice", "2026-02-25.clover", `{"billing_reason":"manual","customer":"cus_invoice","subscription":null}`, http.StatusOK},
		{"subscription invoice", stripeAPIVersion, `{"billing_reason":"subscription_cycle","customer":"cus_invoice","parent":{"type":"subscription_details","subscription_details":{"subscription":"sub_invoice"}}}`, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, store, _ := setupSignedCloudWebhookTestHandler(t)
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			payload := []byte(fmt.Sprintf(`{"id":"evt_store_failure","object":"event","api_version":%q,"type":"invoice.payment_failed","data":{"object":%s}}`, tc.version, tc.invoice))
			signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: payload, Secret: "whsec_test_123"})
			req := httptest.NewRequest(http.MethodPost, "/api/cloud/webhooks/stripe", bytes.NewReader(payload))
			req.Header.Set("Stripe-Signature", signed.Header)
			response := httptest.NewRecorder()
			h.handleStripeWebhook().ServeHTTP(response, req)
			if response.Code != tc.wantStatus {
				t.Errorf("HTTP status = %d, want %d", response.Code, tc.wantStatus)
			}
		})
	}
}
