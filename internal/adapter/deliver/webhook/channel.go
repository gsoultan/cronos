package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gsoultan/cronos/internal/app/burst"
)

// Config is who may be posted to, and the key requests are signed with.
type Config struct {
	// Secret signs every request. 32 bytes or more.
	Secret []byte
	// Hosts are the only hosts a delivery may go to, matched exactly and
	// without a port. Empty means the channel is not registered.
	Hosts []string
	// Timeout bounds one request. Zero means thirty seconds.
	Timeout time.Duration
	// Now is the clock the signature's timestamp is read from. Nil means
	// time.Now.
	Now func() time.Time
}

// Channel posts documents to allowed hosts.
type Channel struct {
	secret []byte
	hosts  map[string]bool
	client *http.Client
	now    func() time.Time
}

// New returns a Channel, or an error if it could never deliver anything.
func New(cfg Config) (*Channel, error) {
	if len(cfg.Secret) < 32 {
		return nil, errors.New("webhook: the signing secret must be 32 bytes or more")
	}
	if len(cfg.Hosts) == 0 {
		return nil, errors.New("webhook: no host is allowed, so nothing could be delivered")
	}
	hosts := map[string]bool{}
	for _, h := range cfg.Hosts {
		hosts[strings.ToLower(strings.TrimSpace(h))] = true
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Channel{secret: cfg.Secret, hosts: hosts, now: cfg.Now, client: &http.Client{
		Timeout: cfg.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}, nil
}

// Name is what a schedule's `via` matches.
func (c *Channel) Name() string { return "webhook" }

// Deliver posts the document to the URL `to:` resolved to.
func (c *Channel) Deliver(ctx context.Context, d burst.Delivery) error {
	target, err := c.allowed(d.To)
	if err != nil {
		return burst.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(),
		bytes.NewReader(d.Document))
	if err != nil {
		return burst.Fatal(fmt.Errorf("webhook: %w", err))
	}
	c.sign(req, d)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: posting to %s: %w", target.Host, err)
	}
	defer resp.Body.Close()
	said, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return outcome(target.Host, resp.StatusCode, said)
}

// allowed parses a destination and refuses one the deployment did not list.
func (c *Channel) allowed(to string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(to))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("webhook: %q is not a URL", to)
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case u.User != nil:
		return nil, fmt.Errorf("webhook: %q carries credentials; the signature is the credential", u.Redacted())
	case !c.hosts[host]:
		return nil, fmt.Errorf("webhook: %s is not in CRONOS_WEBHOOK_HOSTS", host)
	case u.Scheme == "https":
	case u.Scheme == "http" && loopback(host):
	default:
		return nil, fmt.Errorf("webhook: %q must be https", u.Redacted())
	}
	return u, nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// sign sets the headers a receiver checks and files by.
func (c *Channel) sign(req *http.Request, d burst.Delivery) {
	ts := strconv.FormatInt(c.now().Unix(), 10)
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(ts + "."))
	mac.Write(d.Document)

	h := req.Header
	h.Set("Content-Type", contentType(d))
	h.Set("Content-Disposition", mime.FormatMediaType("attachment",
		map[string]string{"filename": d.Filename}))
	h.Set("Cronos-Signature", "t="+ts+",v1="+hex.EncodeToString(mac.Sum(nil)))
	h.Set("Idempotency-Key", idempotencyKey(d))
	if d.Subject != "" {
		h.Set("Cronos-Subject", mime.QEncoding.Encode("utf-8", d.Subject))
	}
	h.Set("User-Agent", "cronos-webhook")
}

// idempotencyKey is the same for every attempt at one delivery and different
// for any other: where it went, what it is called, and what it says.
func idempotencyKey(d burst.Delivery) string {
	sum := sha256.New()
	for _, part := range []string{d.To, d.Filename} {
		sum.Write([]byte(part))
		sum.Write([]byte{0})
	}
	sum.Write(d.Document)
	return hex.EncodeToString(sum.Sum(nil))
}

func contentType(d burst.Delivery) string {
	if t := d.Options["contentType"]; t != "" {
		return t
	}
	// The two cronos renders, spelt here rather than looked up: the system's
	// MIME table differs between a laptop and a container.
	switch strings.ToLower(path.Ext(d.Filename)) {
	case ".pdf":
		return "application/pdf"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	return "application/octet-stream"
}

// outcome reads a response: delivered, worth retrying, or final.
func outcome(host string, status int, said []byte) error {
	if status >= 200 && status < 300 {
		return nil
	}
	err := fmt.Errorf("webhook: %s answered %d: %s", host, status,
		strings.TrimSpace(string(bytes.ToValidUTF8(said, nil))))
	if status >= 500 || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests {
		return err
	}
	// A redirect lands here too: it was not followed, and following it is
	// not something a retry would do differently.
	return burst.Fatal(err)
}
