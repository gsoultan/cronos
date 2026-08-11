package email

import (
	"context"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/app/burst"
)

func channel(t *testing.T) (*Channel, *sent) {
	t.Helper()
	c, err := New(Config{Host: "smtp.example:587", From: "billing@acme.example"})
	if err != nil {
		t.Fatal(err)
	}
	record := &sent{}
	c.send = record.capture
	return c, record
}

type sent struct {
	addr string
	from string
	to   []string
	msg  []byte
}

func (s *sent) capture(addr string, _ smtp.Auth, from string, to []string, msg []byte) error {
	s.addr, s.from, s.to, s.msg = addr, from, to, msg
	return nil
}

func delivery() burst.Delivery {
	return burst.Delivery{
		To: "ap@baltic.example", Subject: "Your July 2026 statement",
		Filename: "statement-c-1-July-2026.pdf", Body: "Attached.",
		Document: []byte("%PDF-1.7 pretend"),
	}
}

// The message is parsed back with net/mail rather than string-matched, because
// what matters is that a mail client can read it — and a client is a parser.
func TestTheMessageParsesAsMail(t *testing.T) {
	c, record := channel(t)
	if err := c.Deliver(context.Background(), delivery()); err != nil {
		t.Fatal(err)
	}

	msg, err := mail.ReadMessage(strings.NewReader(string(record.msg)))
	if err != nil {
		t.Fatalf("the message does not parse: %v", err)
	}
	if got := msg.Header.Get("To"); got != "ap@baltic.example" {
		t.Errorf("To = %q", got)
	}
	if !strings.HasPrefix(msg.Header.Get("Content-Type"), "multipart/mixed") {
		t.Errorf("Content-Type = %q", msg.Header.Get("Content-Type"))
	}
	// So a statement does not trigger somebody's out-of-office, which then
	// replies to the sender, five thousand times.
	if msg.Header.Get("Auto-Submitted") != "auto-generated" {
		t.Error("a generated statement should say so")
	}
}

func TestTheDocumentArrivesIntact(t *testing.T) {
	c, record := channel(t)
	d := delivery()
	d.Document = []byte(strings.Repeat("PDF bytes that need wrapping. ", 200))
	if err := c.Deliver(context.Background(), d); err != nil {
		t.Fatal(err)
	}

	decoded := attachment(t, record.msg)
	if string(decoded) != string(d.Document) {
		t.Error("the attachment came back different")
	}
}

// attachment reads the message the way a mail client would, rather than by
// slicing strings: part headers are written in sorted order, so anything that
// assumes a layout is testing multipart's implementation and not ours.
func attachment(t *testing.T, raw []byte) []byte {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}

	parts := multipart.NewReader(msg.Body, params["boundary"])
	for {
		part, err := parts.NextPart()
		if err != nil {
			t.Fatal("no attachment in the message")
		}
		if part.Header.Get("Content-Transfer-Encoding") != "base64" {
			continue
		}
		body, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		out, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(body), "\r\n", ""))
		if err != nil {
			t.Fatalf("the attachment does not decode: %v", err)
		}
		return out
	}
}

// A relay silently folds an over-long line, and the attachment arrives corrupt
// rather than rejected.
func TestNoLineExceedsWhatSMTPCarries(t *testing.T) {
	c, record := channel(t)
	d := delivery()
	d.Document = []byte(strings.Repeat("x", 5000))
	d.Body = strings.Repeat("a long sentence with no line breaks at all ", 30)
	if err := c.Deliver(context.Background(), d); err != nil {
		t.Fatal(err)
	}

	for i, line := range strings.Split(string(record.msg), "\r\n") {
		if len(line) > 998 {
			t.Fatalf("line %d is %d characters", i, len(line))
		}
	}
}

// A subject carries a customer's name, and "Übersicht" arrives as mojibake if
// it is not encoded.
func TestNonASCIISurvivesTheSubjectAndTheFilename(t *testing.T) {
	c, record := channel(t)
	d := delivery()
	d.Subject = "Ihre Übersicht – Juli"
	d.Filename = "übersicht-Ø.pdf"
	if err := c.Deliver(context.Background(), d); err != nil {
		t.Fatal(err)
	}

	msg, err := mail.ReadMessage(strings.NewReader(string(record.msg)))
	if err != nil {
		t.Fatal(err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil {
		t.Fatal(err)
	}
	if subject != d.Subject {
		t.Errorf("subject decoded to %q", subject)
	}
	if strings.Contains(string(record.msg), "filename=\"übersicht") {
		t.Error("a non-ASCII filename was written raw into a header")
	}
}

// A resolved binding that is not an address means the row's email column was
// empty or misnamed. Saying so beats a relay's rejection an hour later.
func TestAnUnusableAddressIsRefusedBeforeSending(t *testing.T) {
	c, record := channel(t)
	d := delivery()
	d.To = ""
	if err := c.Deliver(context.Background(), d); err == nil {
		t.Error("an empty address was sent")
	}
	if record.msg != nil {
		t.Error("it tried anyway")
	}
}

// Every message in a burst would be rejected by the relay, one at a time, five
// thousand times.
func TestABrokenConfigurationFailsAtStartup(t *testing.T) {
	cases := map[string]Config{
		"no host":                 {From: "a@b.example"},
		"a host with no port":     {Host: "smtp.example", From: "a@b.example"},
		"a from nobody can parse": {Host: "smtp.example:587", From: "not an address"},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := New(cfg); err == nil {
				t.Error("accepted")
			}
		})
	}
}

// A mail server that stops answering must not hold a burst worker for the rest
// of the run.
func TestASilentServerTimesOut(t *testing.T) {
	c, _ := channel(t)
	c.cfg.Timeout = 30 * time.Millisecond
	c.send = func(string, smtp.Auth, string, []string, []byte) error {
		time.Sleep(2 * time.Second)
		return nil
	}

	began := time.Now()
	err := c.Deliver(context.Background(), delivery())
	if err == nil {
		t.Fatal("a server that never answered was reported as delivered")
	}
	if time.Since(began) > time.Second {
		t.Errorf("waited %s", time.Since(began))
	}
}
