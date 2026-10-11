package sftp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Config is the server documents go to. One per deployment, as the mail relay
// is: a schedule's `to:` names a folder on it, never a host, so a value
// resolved from a customer row cannot send a statement somewhere else.
type Config struct {
	// Addr is host:port. A missing port means 22.
	Addr     string
	User     string
	Password string
	// Key is a PEM private key, for servers that refuse passwords.
	Key []byte
	// KnownHosts is the path of a known_hosts file holding the server's key.
	KnownHosts string
	// Root is the folder every `to:` is under. Empty means the login folder.
	Root string
	// Timeout bounds connecting. Zero means ten seconds.
	Timeout time.Duration
}

// unsafe matches what must not reach a remote path. A folder and a filename
// are resolved from a customer's row, so they are cleaned as the file channel
// cleans them: separators and dots become dashes rather than refusals.
var unsafe = regexp.MustCompile(`[^\w.\-]+`)

// Channel writes documents over SFTP.
type Channel struct {
	cfg    Config
	ssh    *ssh.ClientConfig
	mu     sync.Mutex
	client *sftp.Client
	conn   *ssh.Client
}

// New returns a Channel, or an error if it could never deliver anything:
// checked at startup rather than once per recipient.
func New(cfg Config) (*Channel, error) {
	if cfg.Addr == "" || cfg.User == "" {
		return nil, errors.New("sftp: an address and a user are required")
	}
	if !strings.Contains(cfg.Addr, ":") {
		cfg.Addr += ":22"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	hosts, err := knownhosts.New(cfg.KnownHosts)
	if err != nil {
		return nil, fmt.Errorf("sftp: a known_hosts file is required to know the server: %w", err)
	}
	var auth []ssh.AuthMethod
	if len(cfg.Key) > 0 {
		signer, err := ssh.ParsePrivateKey(cfg.Key)
		if err != nil {
			return nil, fmt.Errorf("sftp: private key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if cfg.Password != "" {
		auth = append(auth, ssh.Password(cfg.Password))
	}
	if len(auth) == 0 {
		return nil, errors.New("sftp: a password or a private key is required")
	}
	return &Channel{cfg: cfg, ssh: &ssh.ClientConfig{
		User: cfg.User, Auth: auth, HostKeyCallback: hosts, Timeout: cfg.Timeout,
	}}, nil
}

// Name is what a schedule's `via` matches.
func (c *Channel) Name() string { return "sftp" }

// Deliver writes the document to root/<to>/<filename>, whole or not at all.
func (c *Channel) Deliver(ctx context.Context, d burst.Delivery) error {
	name := clean(d.Filename)
	if name == "" {
		return burst.Fatal(errors.New("sftp: delivery has no filename"))
	}
	dir := path.Join(c.cfg.Root, folder(d.To))

	client, err := c.connect(ctx)
	if err != nil {
		return err
	}
	if err := put(client, dir, name, d.Document); err != nil {
		// The connection may be what failed. Dropped so the retry the burst
		// makes dials afresh rather than writing into a dead session.
		c.drop(client)
		return fmt.Errorf("sftp: writing %s: %w", path.Join(dir, name), err)
	}
	return nil
}

// put writes under a hidden name and renames into place, so a folder being
// polled never shows half a document.
func put(client *sftp.Client, dir, name string, doc []byte) error {
	if err := client.MkdirAll(dir); err != nil {
		return err
	}
	part := path.Join(dir, "."+name+".part")
	f, err := client.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	if _, err := f.Write(doc); err != nil {
		f.Close()
		client.Remove(part)
		return err
	}
	if err := f.Close(); err != nil {
		client.Remove(part)
		return err
	}
	final := path.Join(dir, name)
	// POSIX rename replaces an earlier delivery of the same name, which is
	// what a retried or resumed burst means. Plain SFTP rename refuses to,
	// so it is the fallback for servers without the extension.
	if err := client.PosixRename(part, final); err != nil {
		client.Remove(final)
		if err := client.Rename(part, final); err != nil {
			client.Remove(part)
			return err
		}
	}
	return nil
}

// connect returns the shared client, dialling if there is none.
func (c *Channel) connect(ctx context.Context) (*sftp.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		return c.client, nil
	}
	dialer := net.Dialer{Timeout: c.cfg.Timeout}
	raw, err := dialer.DialContext(ctx, "tcp", c.cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("sftp: %w", err)
	}
	sc, chans, reqs, err := ssh.NewClientConn(raw, c.cfg.Addr, c.ssh)
	if err != nil {
		raw.Close()
		// A host key that does not match, or credentials refused, will be
		// refused again on every retry of every recipient.
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) || strings.Contains(err.Error(), "unable to authenticate") {
			return nil, burst.Fatal(fmt.Errorf("sftp: %w", err))
		}
		return nil, fmt.Errorf("sftp: %w", err)
	}
	conn := ssh.NewClient(sc, chans, reqs)
	client, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("sftp: %w", err)
	}
	c.conn, c.client = conn, client
	return client, nil
}

// drop closes a client that failed, if it is still the shared one.
func (c *Channel) drop(failed *sftp.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != failed {
		return
	}
	c.client.Close()
	c.conn.Close()
	c.client, c.conn = nil, nil
}

// Close ends the shared connection.
func (c *Channel) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return nil
	}
	err := errors.Join(c.client.Close(), c.conn.Close())
	c.client, c.conn = nil, nil
	return err
}

// folder reduces a resolved `to:` to a relative path: each segment cleaned,
// empty ones and dot ones dropped, so it cannot climb out of the root.
func folder(to string) string {
	var segs []string
	for _, s := range strings.Split(to, "/") {
		if s = clean(s); s != "" {
			segs = append(segs, s)
		}
	}
	return path.Join(segs...)
}

// clean reduces a string to one safe path segment: no separator, no leading
// dot, never "..".
func clean(s string) string {
	s = unsafe.ReplaceAllString(strings.TrimSpace(s), "-")
	s = strings.Trim(s, ".-")
	return strings.ReplaceAll(s, "..", "-")
}
