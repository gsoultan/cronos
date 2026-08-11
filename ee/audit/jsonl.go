// Package audit provides a durable audit sink for the Enterprise Edition.
//
// Licensed under ee/LICENSE, not the repository's BSL.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/gsoultan/cronos/internal/extension"
)

// JSONL appends one JSON object per audit event to w. Writes are serialized so
// concurrent report runs cannot interleave partial records.
type JSONL struct {
	mu  sync.Mutex
	enc *json.Encoder
}

// NewJSONL returns a sink that appends events to w.
func NewJSONL(w io.Writer) *JSONL {
	return &JSONL{enc: json.NewEncoder(w)}
}

func (j *JSONL) Name() string { return "jsonl" }

func (j *JSONL) Record(ctx context.Context, e extension.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.enc.Encode(e); err != nil {
		return fmt.Errorf("audit: record %s: %w", e.Action, err)
	}
	return nil
}
