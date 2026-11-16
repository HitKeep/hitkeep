package shared

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var ErrNativeAccountMissing = errors.New("account unavailable")

// NativeAccountSecurity excludes usage timestamps: factor use must not revoke consent.
func (c *Context) NativeAccountSecurity(ctx context.Context, id uuid.UUID) (string, string, error) {
	available, err := c.HumanSessionUserAvailable(ctx, id)
	if err != nil {
		return "", "", err
	}
	if !available {
		return "", "", ErrNativeAccountMissing
	}
	user, err := c.Store.GetUserByID(ctx, id)
	if err != nil {
		return "", "", err
	}
	if user == nil {
		return "", "", ErrNativeAccountMissing
	}
	secret, enabled, err := c.Store.GetUserTOTPSecret(ctx, id)
	if err != nil {
		return "", "", err
	}
	keys, err := c.Store.ListUserPasskeys(ctx, id)
	if err != nil {
		return "", "", err
	}
	identities, err := c.Store.ListUserSocialIdentities(ctx, id)
	if err != nil {
		return "", "", err
	}
	var material strings.Builder
	material.WriteString(fmt.Sprintf("%s\x00%s\x00%t\x00%s\x00%t", user.Email, user.Password, user.PasswordLoginEnabled, secret, enabled))
	for _, key := range keys {
		material.WriteString("\x00" + key.ID.String())
	}
	for _, identity := range identities {
		material.WriteString("\x00" + identity.Provider + "\x00" + identity.LinkedAt.UTC().String())
	}
	sum := sha256.Sum256([]byte(material.String()))
	return user.Email, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}
