package anthropic_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/llm/anthropic"
	"github.com/gsoultan/cronos/internal/app/explain"
)

// api stands in for the Messages API, answering with whatever the test says
// and keeping the request it was sent.
func api(t *testing.T, status int, body string) (*httptest.Server, *http.Request, *[]byte) {
	t.Helper()
	var seen http.Request
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = *r
		raw, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &seen, &raw
}

const answered = `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5",
"content":[{"type":"thinking","thinking":"","signature":"x"},{"type":"text","text":"Revenue rose 4%."}],
"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`

func TestAnAnswerIsItsText(t *testing.T) {
	srv, seen, raw := api(t, http.StatusOK, answered)
	c := anthropic.New("sk-test", "", srv.URL)

	got, err := c.Complete(context.Background(), explain.Prompt{System: "rules", User: "<report>…</report>"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Revenue rose 4%." {
		t.Fatalf("got %q", got)
	}
	if seen.Header.Get("Authorization") != "Bearer sk-test" && seen.Header.Get("X-Api-Key") != "sk-test" {
		t.Error("the key was not sent")
	}
	if !strings.Contains(seen.Header.Get("Anthropic-Beta"), "server-side-fallback-2026-07-01") {
		t.Errorf("beta header %q", seen.Header.Get("Anthropic-Beta"))
	}
	var body map[string]any
	if err := json.Unmarshal(*raw, &body); err != nil {
		t.Fatal(err)
	}
	if body["model"] != anthropic.DefaultModel || body["fallbacks"] != "default" {
		t.Errorf("model %v, fallbacks %v", body["model"], body["fallbacks"])
	}
	if oc, _ := body["output_config"].(map[string]any); oc["effort"] != "low" {
		t.Errorf("output_config %v", body["output_config"])
	}
}

func TestARefusalIsADecline(t *testing.T) {
	srv, _, _ := api(t, http.StatusOK, `{"id":"msg_1","type":"message","role":"assistant",
"model":"claude-opus-5-5","content":[],"stop_reason":"refusal","usage":{"input_tokens":1,"output_tokens":0}}`)
	_, err := anthropic.New("sk", "", srv.URL).Complete(context.Background(), explain.Prompt{User: "x"})
	if !errors.Is(err, explain.ErrDeclined) {
		t.Fatalf("err %v", err)
	}
}

// The provider's error body can echo the prompt, which is somebody's numbers,
// so it does not travel into a log line.
func TestAFailureKeepsTheStatusAndNotTheBody(t *testing.T) {
	srv, _, _ := api(t, http.StatusBadRequest,
		`{"type":"error","error":{"type":"invalid_request_error","message":"revenue was 987654"}}`)
	_, err := anthropic.New("sk", "claude-sonnet-5-5", srv.URL).Complete(context.Background(), explain.Prompt{User: "x"})
	var f *anthropic.Failure
	if !errors.As(err, &f) || f.Status != http.StatusBadRequest {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(err.Error(), "987654") {
		t.Fatalf("the body reached the error: %v", err)
	}
}
