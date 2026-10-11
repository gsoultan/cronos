package sftp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// server is a real SSH server speaking SFTP over a real socket, serving dir.
// The protocol is the thing under test, so nothing here is a mock of it.
type server struct {
	addr       string
	dir        string
	knownHosts string
	handshakes atomic.Int32
	conns      chan net.Conn
}

func serve(t *testing.T) *server {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
		if string(p) != "right" {
			return nil, errors.New("no")
		}
		return nil, nil
	}}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	s := &server{addr: ln.Addr().String(), dir: t.TempDir(), conns: make(chan net.Conn, 16)}
	s.knownHosts = hostsFile(t, s.addr, signer.PublicKey())
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.conns <- c
			go s.handle(c, cfg)
		}
	}()
	return s
}

func hostsFile(t *testing.T, addr string, key ssh.PublicKey) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{knownhosts.Normalize(addr)}, key)
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (s *server) handle(c net.Conn, cfg *ssh.ServerConfig) {
	_, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		return
	}
	s.handshakes.Add(1)
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		ch, in, err := nc.Accept()
		if err != nil {
			continue
		}
		go func() {
			for req := range in {
				ok := req.Type == "subsystem" && string(req.Payload[4:]) == "sftp"
				req.Reply(ok, nil)
				if ok {
					srv, _ := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(s.dir))
					srv.Serve()
					ch.Close()
				}
			}
		}()
	}
}

// dropAll closes every connection the server has accepted, as a restart or an
// idle timeout on the far side would.
func (s *server) dropAll() {
	for {
		select {
		case c := <-s.conns:
			c.Close()
		default:
			return
		}
	}
}

func channel(t *testing.T, s *server, password string) *Channel {
	t.Helper()
	c, err := New(Config{Addr: s.addr, User: "cronos", Password: password,
		KnownHosts: s.knownHosts, Root: "inbound"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestADocumentArrivesWholeWhereItWasSent(t *testing.T) {
	s := serve(t)
	c := channel(t, s, "right")
	doc := []byte("%PDF-1.7 a statement")

	if err := c.Deliver(context.Background(), burst.Delivery{
		To: "acme/c-1", Filename: "statement-c-1.pdf", Document: doc}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(s.dir, "inbound", "acme", "c-1", "statement-c-1.pdf"))
	if err != nil || string(got) != string(doc) {
		t.Fatalf("got %q, %v", got, err)
	}
	entries, _ := os.ReadDir(filepath.Join(s.dir, "inbound", "acme", "c-1"))
	if len(entries) != 1 {
		t.Errorf("the folder holds %d entries; the part file was left behind", len(entries))
	}
}

// A retried or resumed burst delivers the same name again.
func TestADeliveryReplacesAnEarlierOneOfTheSameName(t *testing.T) {
	s := serve(t)
	c := channel(t, s, "right")
	for _, body := range []string{"first", "second"} {
		if err := c.Deliver(context.Background(), burst.Delivery{
			To: "c-1", Filename: "s.pdf", Document: []byte(body)}); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := os.ReadFile(filepath.Join(s.dir, "inbound", "c-1", "s.pdf"))
	if string(got) != "second" {
		t.Fatalf("got %q", got)
	}
}

// A folder and a filename come from a customer's row. Neither leaves the root.
func TestAResolvedPathCannotClimbOutOfTheRoot(t *testing.T) {
	s := serve(t)
	c := channel(t, s, "right")
	if err := c.Deliver(context.Background(), burst.Delivery{
		To: "../../escape", Filename: "../../x.pdf", Document: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "inbound", "escape", "x.pdf")); err != nil {
		t.Fatalf("not delivered inside the root: %v", err)
	}
}

// One handshake for a burst, and another only when the connection drops.
func TestTheConnectionIsSharedAndRedialled(t *testing.T) {
	s := serve(t)
	c := channel(t, s, "right")
	send := func() error {
		return c.Deliver(context.Background(), burst.Delivery{
			To: "c", Filename: "s.pdf", Document: []byte("x")})
	}
	for range 5 {
		if err := send(); err != nil {
			t.Fatal(err)
		}
	}
	if n := s.handshakes.Load(); n != 1 {
		t.Fatalf("%d handshakes for five deliveries", n)
	}
	s.dropAll()
	// The first attempt after a drop may fail; the burst retries it, and the
	// retry dials afresh.
	if err := send(); err != nil {
		if err := send(); err != nil {
			t.Fatalf("still failing after the connection dropped: %v", err)
		}
	}
	if n := s.handshakes.Load(); n != 2 {
		t.Fatalf("%d handshakes, want a second after the drop", n)
	}
}

// Neither of these improves with a retry, so neither is retried.
func TestAWrongServerOrPasswordIsPermanent(t *testing.T) {
	s := serve(t)
	other := serve(t)

	wrongKey, err := New(Config{Addr: s.addr, User: "cronos", Password: "right",
		KnownHosts: other.knownHosts})
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]*Channel{"host key": wrongKey, "password": channel(t, s, "wrong")} {
		err := c.Deliver(context.Background(), burst.Delivery{To: "c", Filename: "s.pdf", Document: []byte("x")})
		var p burst.Permanent
		if !errors.As(err, &p) {
			t.Errorf("%s: %v is retried", name, err)
		}
	}
}

func TestNoKnownHostsNoChannel(t *testing.T) {
	if _, err := New(Config{Addr: "h", User: "u", Password: "p"}); err == nil {
		t.Fatal("a channel that would trust any server")
	}
}
