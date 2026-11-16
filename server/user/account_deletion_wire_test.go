package user

import (
	"net/http/httptest"
	"testing"

	"hitkeep/api"
	json "hitkeep/jsonapi"
	"hitkeep/server/shared"
)

func TestAccountDeletionBlockedWireNullability(t *testing.T) {
	response := api.AccountDeletionPreparation{Teams: []api.AccountDeletionTeam{}, Blockers: []api.AccountDeletionBlocker{{Code: "last_operator"}}, Purpose: "delete-current-account", Phase: "blocked"}
	response.Retention.BillingRecords = true
	response.Retention.Cancellation = "period_end"
	w := httptest.NewRecorder()
	shared.WriteJSON(t.Context(), w, 200, response)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["attempt_id"] != "" || body["expires_at"] != "0001-01-01T00:00:00Z" || body["consequence_hash"] != "" {
		t.Fatalf("blocked scalar contract changed: %s", w.Body.String())
	}
	if teams, ok := body["teams"].([]any); !ok || len(teams) != 0 {
		t.Fatalf("teams must be an empty array: %s", w.Body.String())
	}
	if _, exists := body["blockers"].([]any)[0].(map[string]any)["team_id"]; exists {
		t.Fatal("global blocker must omit zero team UUID")
	}
	reauth := body["reauth"].(map[string]any)
	if methods, exists := reauth["methods"]; !exists || methods != nil || reauth["verified"] != false {
		t.Fatalf("blocked reauth contract changed: %s", w.Body.String())
	}
}

func TestAccountDeletionResultAndErrorWire(t *testing.T) {
	w := httptest.NewRecorder()
	shared.WriteJSON(t.Context(), w, 200, api.AccountDeletionResult{Status: "deleted", ProviderRevocations: []api.AccountDeletionProviderRevocation{}})
	var result struct {
		Status    string                                  `json:"status"`
		Providers []api.AccountDeletionProviderRevocation `json:"provider_revocations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "deleted" || result.Providers == nil {
		t.Fatalf("completed result requires200+emptyarray: %s", w.Body.String())
	}
	for _, test := range []struct {
		status int
		code   string
	}{{503, "closure_retry_required"}, {409, "consequences_changed"}} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("DELETE", "/api/user/account", nil)
		accountDeletionError(w, r, test.status, test.code)
		var body struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != test.status || body.Error.Code != test.code || body.Error.Message != test.code {
			t.Fatalf("structured error changed: %d %s", w.Code, w.Body.String())
		}
	}
}
