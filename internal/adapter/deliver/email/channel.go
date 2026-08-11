package email

import (
	"context"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"time"

	"github.com/gsoultan/cronos/internal/app/burst"
)

// Config is where mail goes and who it says it is from.
type Config struct {
	// Host is host:port. Empty disables the channel, which is why it is
	// checked at startup rather than at six on the first of the month.
	Host string
	From string
	// Username and Password are optional: an internal relay often wants
	// neither, and requiring them would mean inventing credentials.
	Username string
	Password string
	// Insecure allows a plaintext session. For a relay on localhost, and
	// named so nobody sets it by accident.
	Insecure bool
	Timeout  time.Duration
}

// DefaultTimeout bounds one send. A mail server that stops answering must not
// hold a burst worker for the rest of the run.
const DefaultTimeout = 30 * time.Second

// Channel sends documents as attachments.
type Channel struct {
	cfg Config
	// send is the transport, replaceable so a test asserts on a real message
	// rather than on a mock's arguments.
	send func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

// New returns a Channel, or an error if it could never send anything.
func New(cfg Config) (*Channel, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("email: no host configured")
	}
	if _, _, err := net.SplitHostPort(cfg.Host); err != nil {
		return nil, fmt.Errorf("email: host must be host:port, got %q", cfg.Host)
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		// An unparseable From is rejected by the relay for every message in a
		// burst, one at a time, five thousand times.
		return nil, fmt.Errorf("email: %q is not a from address", cfg.From)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &Channel{cfg: cfg, send: smtp.SendMail}, nil
}

// Name is what a schedule's `via` matches.
func (c *Channel) Name() string { return "email" }

// Deliver sends one document.
func (c *Channel) Deliver(ctx context.Context, d burst.Delivery) error {
	to, err := mail.ParseAddress(d.To)
	if err != nil {
		// A resolved binding that is not an address means the row's email
		// column was empty or misnamed. Saying so beats a relay's rejection
		// arriving in a log an hour later.
		return fmt.Errorf("email: %q is not an address", d.To)
	}

	msg, err := message{
		From: c.cfg.From, To: to.Address, Subject: d.Subject, Body: d.Body,
		Filename: d.Filename, Attachment: d.Document,
	}.build()
	if err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() { done <- c.send(c.cfg.Host, c.auth(), c.cfg.From, []string{to.Address}, msg) }()

	select {
	case <-ctx.Done():
		// The send is abandoned rather than cancelled: net/smtp has no context.
		// The goroutine finishes into a buffered channel and exits.
		return ctx.Err()
	case err := <-done:
		return err
	case <-time.After(c.cfg.Timeout):
		return fmt.Errorf("email: %s did not answer within %s", c.cfg.Host, c.cfg.Timeout)
	}
}

func (c *Channel) auth() smtp.Auth {
	if c.cfg.Username == "" {
		return nil
	}
	host, _, _ := net.SplitHostPort(c.cfg.Host)
	return smtp.PlainAuth("", c.cfg.Username, c.cfg.Password, host)
}
