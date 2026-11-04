package shared

import (
	"context"

	"github.com/google/uuid"
	"hitkeep/api"
	"hitkeep/config"
	"hitkeep/database"
	"hitkeep/socialauth"
	"hitkeep/sso"
)

// NewAccountDeletionProviderRevoker may be called under account lifecycle locks before identity cleanup.
// Provider failures require manual revocation but never prevent local account removal.
func NewAccountDeletionProviderRevoker(conf *config.Config, store *database.Store) func(context.Context, uuid.UUID) []api.AccountDeletionProviderRevocation {
	client := socialauth.NewClient(sso.NewRuntimeClient(conf != nil && conf.CloudHosted))
	return func(ctx context.Context, userID uuid.UUID) []api.AccountDeletionProviderRevocation {
		outcome := api.AccountDeletionProviderRevocation{Provider: socialauth.ProviderApple, Status: "manual_revocation_required"}
		if store == nil {
			return []api.AccountDeletionProviderRevocation{outcome}
		}
		identity, err := store.GetUserAppleIdentity(ctx, userID)
		if err == nil && identity == nil {
			return []api.AccountDeletionProviderRevocation{}
		}
		if err != nil || conf == nil {
			return []api.AccountDeletionProviderRevocation{outcome}
		}
		cfg, err := socialauth.ConfigForProvider(conf, socialauth.ProviderApple)
		if err == nil {
			token, openErr := socialauth.OpenAppleRefreshToken(conf.SocialAppleTokenEncryptionKey, identity.AppleClientID, identity.Subject, identity.AppleRefreshTokenCiphertext)
			if openErr == nil && client.RevokeApple(ctx, cfg, identity.AppleClientID, token) == nil {
				outcome.Status = "revoked"
			}
		}
		return []api.AccountDeletionProviderRevocation{outcome}
	}
}
