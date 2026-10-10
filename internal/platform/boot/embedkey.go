package boot

import (
	"bytes"
	"errors"
	"log/slog"

	"github.com/gsoultan/cronos/internal/platform/config"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
embedSignerFor builds the signer embed tokens are minted and verified with.

CRONOS_EMBED_KEY is the key a host application is given. Its backend signs a
token for each of its own users, so whoever holds it chooses every claim it
signs — and when that was the signing key, one of the claims it could choose
was `aud: portal` with a platform role. The returned signer is confined to
embed tokens, and the portal's never sees this key.

Without one, embed tokens are signed with the signing key as they always were.
That keeps a deployment that upgraded working, and it is said at startup every
time, because it is the configuration in which a host still holds the key to
the portal.
*/
func embedSignerFor(cfg config.Server, signer *token.Signer, log *slog.Logger) (*token.Signer, error) {
	if len(cfg.EmbedKey) == 0 {
		if len(cfg.PreviousEmbedKeys) > 0 {
			return nil, errors.New("CRONOS_EMBED_KEY_PREVIOUS is set and CRONOS_EMBED_KEY is not: " +
				"there is no embed key for those to be previous to")
		}
		log.Warn("embed tokens are signed with CRONOS_SIGNING_KEY — a host application that " +
			"holds it can sign a portal session. Set CRONOS_EMBED_KEY and give hosts that " +
			"instead; see \"The embed key\" in docs/deploying.md")
		return signer.For(token.Embed), nil
	}

	/* The same key under both names is the old arrangement with a second
	   variable, and one equal to a retired signing key is worse: the portal
	   still accepts that key, so a host would hold it for as long as the
	   rotation runs. */
	for _, portal := range append([][]byte{cfg.SigningKey}, cfg.PreviousKeys...) {
		if bytes.Equal(cfg.EmbedKey, portal) {
			return nil, errors.New("CRONOS_EMBED_KEY is the same as a signing key: " +
				"a host given it could sign a portal session, which is what a separate key is for")
		}
	}

	embed, err := token.NewSigner(cfg.EmbedKey)
	if err != nil {
		return nil, err
	}
	if len(cfg.PreviousEmbedKeys) > 0 {
		if embed, err = embed.Accepting(cfg.PreviousEmbedKeys...); err != nil {
			return nil, err
		}
	}
	return embed.For(token.Embed), nil
}
