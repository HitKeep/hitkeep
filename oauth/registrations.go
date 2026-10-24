package oauth

import (
	"context"
	"encoding/json/v2"
	"strings"
	"uuid"

	"hitkeep/database"
)

// ClientRegistration deliberately exposes only public PKCE metadata, never credentials.
type ClientRegistration struct {
	ID           string   `json:"client_id"`
	Name         string   `json:"client_name"`
	URI          string   `json:"client_uri,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`
}

func (p *Provider) resolveClient(ctx context.Context, id string) (ClientMetadata, error) {
	if _, configured := p.clients.registered[id]; configured {
		return p.clients.resolve(ctx, id)
	}
	if !strings.HasPrefix(id, database.OAuthManagedClientPrefix) {
		return p.clients.resolve(ctx, id)
	}
	c, err := p.store.GetOAuthClient(ctx, id)
	if err != nil {
		return ClientMetadata{}, err
	}
	var metadata ClientMetadata
	if err := json.Unmarshal(c.Metadata, &metadata); err != nil {
		return ClientMetadata{}, err
	}
	metadata.RegistrationVersion = c.Revision
	metadata.RegistrationTeamID = c.TeamID
	return metadata, nil
}

func (p *Provider) ListClients(ctx context.Context, teamID uuid.UUID) ([]ClientRegistration, error) {
	if !p.Ready() {
		return nil, ErrUnavailable
	}
	records, err := p.store.ListOAuthClients(ctx, teamID)
	if err != nil {
		return nil, err
	}
	result := make([]ClientRegistration, 0, len(records))
	for _, record := range records {
		var c ClientRegistration
		if err := json.Unmarshal(record.Metadata, &c); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, nil
}

func (p *Provider) SaveClient(ctx context.Context, actorID, teamID uuid.UUID, c ClientRegistration, create bool) (*ClientRegistration, error) {
	if !p.Ready() {
		return nil, ErrUnavailable
	}
	if create {
		c.ID = database.OAuthManagedClientPrefix + uuid.NewV7().String()
		if _, exists := p.clients.registered[c.ID]; exists {
			return nil, ErrInvalidConsent
		}
	}
	metadata := ClientMetadata{ID: c.ID, Name: c.Name, URI: c.URI, RedirectURIs: c.RedirectURIs, AuthMethod: "none", GrantTypes: []string{"authorization_code", "refresh_token"}, ResponseTypes: []string{"code"}}
	if err := validateClientMetadata(metadata, c.ID, true); err != nil {
		return nil, ErrInvalidConsent
	}
	// Team registrations require HTTPS even though configured clients and HTTPS CIMD support native loopbacks.
	for _, redirect := range c.RedirectURIs {
		if !strings.HasPrefix(redirect, "https://") || strings.Contains(redirect, "*") {
			return nil, ErrInvalidConsent
		}
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	err = p.store.SaveOAuthClient(ctx, actorID, teamID, database.OAuthClientRecord{ID: c.ID, Metadata: payload, Revision: uuid.NewV7().String()}, create, false)
	return &c, err
}

func (p *Provider) RevokeClient(ctx context.Context, actorID, teamID uuid.UUID, id string) error {
	if !p.Ready() {
		return ErrUnavailable
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.store.SaveOAuthClient(ctx, actorID, teamID, database.OAuthClientRecord{ID: id, Revision: uuid.NewV7().String()}, false, true)
}
