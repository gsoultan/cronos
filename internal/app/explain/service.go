package explain

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gsoultan/cronos/internal/app/run"
)

// mostQuestion is a question, not a document pasted into the box.
const mostQuestion = 500

// Service explains views.
type Service struct {
	model Model
}

// New wires a Service around whichever model the deployment configured.
func New(m Model) *Service { return &Service{model: m} }

// Explain answers a question about a view the caller already rendered for the
// person asking. See the package doc for why it is handed a view rather than
// asked to render one.
func (s *Service) Explain(ctx context.Context, v run.View, req Request) (Answer, error) {
	question := strings.TrimSpace(req.Question)
	if utf8.RuneCountInString(question) > mostQuestion {
		return Answer{}, fmt.Errorf("%w: a question is at most %d characters", ErrInvalid, mostQuestion)
	}
	if req.Block != nil && (*req.Block < 0 || *req.Block >= len(v.Blocks)) {
		return Answer{}, fmt.Errorf("%w: this report has no block %d", ErrInvalid, *req.Block)
	}

	text, err := s.model.Complete(ctx, Prompt{System: system, User: turn(v, req.Block, question)})
	if err != nil {
		return Answer{}, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Answer{}, ErrDeclined
	}
	return Answer{Text: text, Block: req.Block}, nil
}

// turn is the user message: the report as data, then what is being asked.
func turn(v run.View, block *int, question string) string {
	var b strings.Builder
	b.WriteString("<report>\n")
	b.WriteString(digest(v))
	b.WriteString("</report>\n\n")
	if block != nil {
		fmt.Fprintf(&b, "Focus on block %d (%s).\n", *block, clip(v.Blocks[*block].Title))
	}
	if question == "" {
		if block != nil {
			b.WriteString("Explain what this block measures and what its numbers say.")
		} else {
			b.WriteString("Explain what this report is for and what its numbers say.")
		}
		return b.String()
	}
	b.WriteString("<question>\n")
	b.WriteString(question)
	b.WriteString("\n</question>")
	return b.String()
}
