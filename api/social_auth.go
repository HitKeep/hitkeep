package api

// AccountDeletionProviderRevocation reports only the result of a provider grant revocation.
type AccountDeletionProviderRevocation struct {
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

type SocialUnlinkResponse struct {
	Status              string                              `json:"status"`
	ProviderRevocations []AccountDeletionProviderRevocation `json:"provider_revocations"`
}
