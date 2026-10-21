package server

import (
	"encoding/json/v2"
	"errors"
	"strings"

	"hitkeep/config"
	"hitkeep/database"
	"hitkeep/oauth"
)

func newOAuthProvider(conf *config.Config, store *database.Store, isLeader func() bool) (*oauth.Provider, error) {
	var clients []oauth.ClientMetadata
	registry := strings.TrimSpace(conf.OAuthClients)
	if registry != "" {
		if len(registry) > 64<<10 || !strings.HasPrefix(registry, "[") {
			return nil, errors.New("invalid OAuth client registry")
		}
		if err := json.Unmarshal([]byte(registry), &clients, json.RejectUnknownMembers(true)); err != nil {
			return nil, errors.New("invalid OAuth client registry")
		}
	}
	return oauth.New(store, oauth.Options{
		Issuer: conf.PublicURL, MCPPath: conf.MCPPath, MCPEnabled: conf.MCPEnabled,
		BrowserSigningKey: []byte(conf.JWTSecret), Clients: clients, IsLeader: isLeader,
	})
}
