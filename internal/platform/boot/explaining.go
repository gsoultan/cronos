package boot

import (
	"fmt"
	"log/slog"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/adapter/llm/anthropic"
	"github.com/gsoultan/cronos/internal/app/explain"
	"github.com/gsoultan/cronos/internal/platform/config"
)

/*
explainingFor builds the model that explains reports, where one is configured.

Nil without a key, so the route is not mounted and the portal offers nothing.
A provider this build does not know is an error rather than a quiet nil: the
deployment asked for explanations, and starting without them would be a
setting that silently did nothing.
*/
func explainingFor(cfg config.AI, log *slog.Logger) (api.Explaining, string, error) {
	if !cfg.Configured() {
		return nil, "", nil
	}
	switch cfg.Provider {
	case "", "anthropic":
		c := anthropic.New(cfg.APIKey, cfg.Model, cfg.BaseURL)
		log.Info("reports can be explained", "provider", "anthropic", "model", c.Model())
		return explain.New(c), c.Model(), nil
	default:
		return nil, "", fmt.Errorf("CRONOS_AI_PROVIDER %q is not one this build has; use anthropic", cfg.Provider)
	}
}
