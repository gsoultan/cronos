package anthropic

import (
	"context"
	"errors"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/gsoultan/cronos/internal/app/explain"
)

// DefaultModel is used when the deployment names none.
const DefaultModel = "claude-opus-5-5"

/*
The limits on one answer.

maxTokens is generous for a 150-word answer because thinking counts against it
too, and an answer cut off mid-sentence is worse than a slow one. The timeout is
the reader's patience: past it they have closed the panel.
*/
const (
	maxTokens = 4000
	timeout   = 60 * time.Second
)

// Client is one configured connection to the Messages API.
type Client struct {
	api   sdk.Client
	model string
}

// New builds a Client. An empty model is DefaultModel; an empty base URL is
// Anthropic's own API.
func New(key, model, baseURL string) *Client {
	// Without the environment's defaults: cronos is configured through its
	// own CRONOS_AI_* settings, and a stray ANTHROPIC_BASE_URL in a
	// container's environment should not quietly send readers' figures
	// somewhere else.
	opts := []option.RequestOption{
		option.WithoutEnvironmentDefaults(),
		option.WithAPIKey(key),
		option.WithRequestTimeout(timeout),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if model == "" {
		model = DefaultModel
	}
	return &Client{api: sdk.NewClient(opts...), model: model}
}

// Model is the model this client asks, for the startup line.
func (c *Client) Model() string { return c.model }

/*
Complete sends one prompt and returns the text of the answer.

Low effort: an explanation is a short reading of numbers already on the page,
and the reader is waiting. The server-side fallback lets a request the model
declines be answered by another model rather than failing; a decline the whole
chain makes is ErrDeclined, so the reader is told it was refused rather than
that something broke.
*/
func (c *Client) Complete(ctx context.Context, p explain.Prompt) (string, error) {
	msg, err := c.api.Beta.Messages.New(ctx, sdk.BetaMessageNewParams{
		Model:        c.model,
		MaxTokens:    maxTokens,
		System:       []sdk.BetaTextBlockParam{{Text: p.System, CacheControl: sdk.NewBetaCacheControlEphemeralParam()}},
		Messages:     messages(p.Turns),
		OutputConfig: sdk.BetaOutputConfigParam{Effort: sdk.BetaOutputConfigEffortLow},
		Fallbacks:    sdk.BetaFallbacksParamOfDefault(),
		Betas:        []sdk.AnthropicBeta{sdk.AnthropicBetaServerSideFallback2026_07_01},
	})
	if err != nil {
		return "", provider(err)
	}
	if msg.StopReason == sdk.BetaStopReasonRefusal {
		return "", explain.ErrDeclined
	}
	var b strings.Builder
	for _, block := range msg.Content {
		if text, ok := block.AsAny().(sdk.BetaTextBlock); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String(), nil
}

/*
messages is the conversation in the API's shape.

The first turn carries the report and is marked for caching: in a conversation
it is the same on every request, so a follow-up is not billed the whole report
again. A short single question falls under the API's minimum and the mark is
ignored.
*/
func messages(turns []explain.Message) []sdk.BetaMessageParam {
	out := make([]sdk.BetaMessageParam, 0, len(turns))
	for i, t := range turns {
		text := sdk.BetaTextBlockParam{Text: t.Text}
		if i == 0 {
			text.CacheControl = sdk.NewBetaCacheControlEphemeralParam()
		}
		content := []sdk.BetaContentBlockParamUnion{{OfText: &text}}
		if t.Role == explain.RoleAssistant {
			out = append(out, sdk.BetaMessageParam{OfAssistant: &sdk.BetaAssistantMessageParam{Content: content}})
			continue
		}
		out = append(out, sdk.BetaMessageParam{OfUser: &sdk.BetaUserMessageParam{Content: content}})
	}
	return out
}

// provider keeps the status and drops the body: an API error can echo the
// request, and the request is somebody's numbers.
func provider(err error) error {
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		return &Failure{Status: apiErr.StatusCode}
	}
	return &Failure{Cause: err}
}
