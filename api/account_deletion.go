package api

import (
	"time"

	"github.com/google/uuid"
)

type AccountDeletionSuccessor struct {
	TeamID uuid.UUID `json:"team_id"`
	UserID uuid.UUID `json:"user_id"`
}

type AccountDeletionChoices struct {
	Successors   []AccountDeletionSuccessor `json:"successors"`
	CloseTeamIDs []uuid.UUID                `json:"close_team_ids"`
}

type AccountDeletionBlocker struct {
	Code   string    `json:"code"`
	TeamID uuid.UUID `json:"team_id,omitzero"`
}

type AccountDeletionSite struct {
	ID     uuid.UUID `json:"id"`
	Domain string    `json:"domain"`
	Owned  bool      `json:"owned"`
}

type AccountDeletionOwner struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
}

type AccountDeletionTeam struct {
	ID                   uuid.UUID              `json:"id"`
	Name                 string                 `json:"name"`
	Action               string                 `json:"action"`
	MemberCount          int                    `json:"member_count"`
	ClosureAllowed       bool                   `json:"closure_allowed"`
	Sites                []AccountDeletionSite  `json:"sites"`
	EligibleSuccessors   []AccountDeletionOwner `json:"eligible_successors"`
	SuccessorID          uuid.UUID              `json:"successor_id,omitzero"`
	CancellationRequired bool                   `json:"cancellation_required"`
	BillingFingerprint   string                 `json:"billing_fingerprint,omitempty"`
}

type AccountDeletionPlan struct {
	Teams    []AccountDeletionTeam    `json:"teams"`
	Blockers []AccountDeletionBlocker `json:"blockers"`
}

type AccountDeletionPreparation struct {
	AccountDeletionPlan
	AttemptID       string    `json:"attempt_id"`
	ExpiresAt       time.Time `json:"expires_at"`
	Purpose         string    `json:"purpose"`
	Phase           string    `json:"phase"`
	ConsequenceHash string    `json:"consequence_hash"`
	Reauth          struct {
		ReturnPath string   `json:"return_path"`
		Methods    []string `json:"methods"`
		Verified   bool     `json:"verified"`
	} `json:"reauth"`
	Retention struct {
		BillingRecords bool   `json:"billing_records"`
		Cancellation   string `json:"cancellation"`
	} `json:"retention"`
}

type AccountDeletionConfirmation struct {
	AttemptID       string `json:"attempt_id"`
	ConsequenceHash string `json:"consequence_hash"`
	Confirm         bool   `json:"confirm"`
}
