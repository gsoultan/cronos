package email

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"
)

// message is one email, assembled.
type message struct {
	From        string
	To          string
	Subject     string
	Body        string
	Filename    string
	Attachment  []byte
	ContentType string
	// Now is injected so a test can assert on a fixed Date header.
	Now func() time.Time
}

// build renders the MIME message.
//
// multipart/mixed with a text part and one attachment: the shape every mail
// client has understood for twenty years, and the one least likely to arrive
// as an unreadable blob in whatever a recipient actually uses.
func (m message) build() ([]byte, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	if err := m.text(w); err != nil {
		return nil, err
	}
	if len(m.Attachment) > 0 {
		if err := m.attach(w); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	m.headers(&out, w.Boundary())
	out.WriteString("\r\n")
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

func (m message) headers(out *bytes.Buffer, boundary string) {
	now := time.Now
	if m.Now != nil {
		now = m.Now
	}
	fmt.Fprintf(out, "From: %s\r\n", m.From)
	fmt.Fprintf(out, "To: %s\r\n", m.To)
	// Encoded, because a subject carries a customer's name and "Übersicht"
	// arrives as mojibake otherwise. Q-encoding leaves ASCII untouched.
	fmt.Fprintf(out, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(out, "Date: %s\r\n", now().Format(time.RFC1123Z))
	out.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(out, "Content-Type: multipart/mixed; boundary=%q\r\n", boundary)
	// Marks it as generated so it does not trip an out-of-office loop.
	out.WriteString("Auto-Submitted: auto-generated\r\n")
}

func (m message) text(w *multipart.Writer) error {
	head := textproto.MIMEHeader{}
	head.Set("Content-Type", "text/plain; charset=utf-8")
	head.Set("Content-Transfer-Encoding", "quoted-printable")

	part, err := w.CreatePart(head)
	if err != nil {
		return err
	}
	body := m.Body
	if strings.TrimSpace(body) == "" {
		body = "Your statement is attached."
	}
	return quoted(part, body)
}

func (m message) attach(w *multipart.Writer) error {
	kind := m.ContentType
	if kind == "" {
		kind = "application/pdf"
	}
	head := textproto.MIMEHeader{}
	head.Set("Content-Type", kind)
	head.Set("Content-Transfer-Encoding", "base64")
	// The filename is encoded rather than quoted raw: it is built from a row of
	// somebody's customer table, so it can contain anything a name can.
	head.Set("Content-Disposition",
		mime.FormatMediaType("attachment", map[string]string{"filename": m.Filename}))

	part, err := w.CreatePart(head)
	if err != nil {
		return err
	}
	return wrap(part, base64.StdEncoding.EncodeToString(m.Attachment))
}
