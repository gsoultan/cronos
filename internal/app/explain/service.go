package explain

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gsoultan/cronos/internal/app/run"
)

/*
The limits on what a reader can send.

A question is a question, not a document pasted into the box. An earlier
answer is one the model wrote, which the system prompt keeps to a few hundred
words; one far longer than that was not. Twelve exchanges is a long
conversation about one report, and the cap keeps every request's size bounded
however long a panel stays open.
*/
const (
	mostQuestion = 500
	mostAnswer   = 4000
	mostHistory  = 12
)

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
	req.Question = strings.TrimSpace(req.Question)
	if err := check(v, req); err != nil {
		return Answer{}, err
	}
	text, err := s.model.Complete(ctx, Prompt{System: system, Turns: turns(v, req)})
	if err != nil {
		return Answer{}, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Answer{}, ErrDeclined
	}
	return Answer{Text: text, Block: req.Block}, nil
}

// check refuses a request before anything is spent on it.
func check(v run.View, req Request) error {
	if len(req.History) > mostHistory {
		return fmt.Errorf("%w: a conversation is at most %d questions; start a new one", ErrInvalid, mostHistory+1)
	}
	for _, x := range req.History {
		if err := asked(v, x.Block, x.Question); err != nil {
			return err
		}
		if a := strings.TrimSpace(x.Answer); a == "" || utf8.RuneCountInString(a) > mostAnswer {
			return fmt.Errorf("%w: an earlier answer is empty or too long", ErrInvalid)
		}
	}
	return asked(v, req.Block, req.Question)
}

func asked(v run.View, block *int, question string) error {
	if utf8.RuneCountInString(strings.TrimSpace(question)) > mostQuestion {
		return fmt.Errorf("%w: a question is at most %d characters", ErrInvalid, mostQuestion)
	}
	if block != nil && (*block < 0 || *block >= len(v.Blocks)) {
		return fmt.Errorf("%w: this report has no block %d", ErrInvalid, *block)
	}
	return nil
}

/*
turns is the conversation as the model reads it.

The report comes once, at the top of the first turn, rendered now as the person
asking: an earlier answer may have been about other filters, but the figures
the model is given are always the ones on screen. Every question after it is a
turn of its own, so an earlier answer reads as the model's own words and never
as the reader's.
*/
func turns(v run.View, req Request) []Message {
	var b strings.Builder
	b.WriteString("<report>\n")
	b.WriteString(digest(v))
	b.WriteString("</report>\n\n")

	out := make([]Message, 0, 2*len(req.History)+1)
	for _, x := range req.History {
		out = append(out,
			Message{Role: RoleUser, Text: b.String() + ask(v, x.Block, strings.TrimSpace(x.Question))},
			Message{Role: RoleAssistant, Text: strings.TrimSpace(x.Answer)})
		b.Reset()
	}
	return append(out, Message{Role: RoleUser, Text: b.String() + ask(v, req.Block, req.Question)})
}

// ask is one question: what it is about, then the reader's words as data.
func ask(v run.View, block *int, question string) string {
	var b strings.Builder
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
