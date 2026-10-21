// Package mailer sends plain SMTP email (standard library only, so any provider
// works — Resend, Gmail, a company relay, ...). It supports implicit TLS (port
// 465), STARTTLS (587/25) and, for local relays with no login, no encryption.
package mailer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// Connection security modes.
const (
	SecurityTLS      = "tls"      // implicit TLS from the first byte (usually port 465)
	SecuritySTARTTLS = "starttls" // plaintext greeting upgraded with STARTTLS (587/25)
	SecurityNone     = "none"     // no encryption — only allowed without credentials
)

// Config is everything needed to open an SMTP session.
type Config struct {
	Host     string
	Port     int
	Security string // SecurityTLS | SecuritySTARTTLS | SecurityNone
	Username string // empty = no authentication
	Password string
	From     string // envelope + header sender address
}

// Attachment is one file attached to a message.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Message is one email. Each Send call delivers to exactly the To addresses given.
type Message struct {
	To          []string
	Subject     string
	Body        string // plain text
	RTL         bool   // also send an HTML copy laid out right-to-left (Arabic)
	Attachments []Attachment
}

// ErrInsecureCredentials is returned when credentials would travel unencrypted.
var ErrInsecureCredentials = errors.New("refusing to send credentials over an unencrypted connection")

// Validate checks a Config's shape (not reachability).
func (c Config) Validate() error {
	if strings.TrimSpace(c.Host) == "" || strings.ContainsAny(c.Host, " \r\n\t/") {
		return errors.New("invalid SMTP host")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("invalid SMTP port")
	}
	switch c.Security {
	case SecurityTLS, SecuritySTARTTLS, SecurityNone:
	default:
		return errors.New("invalid SMTP security mode")
	}
	if hasCRLF(c.Username) || hasCRLF(c.Password) {
		return errors.New("invalid SMTP credentials")
	}
	if c.Security == SecurityNone && c.Username != "" {
		return ErrInsecureCredentials
	}
	if _, err := mail.ParseAddress(c.From); err != nil || hasCRLF(c.From) {
		return errors.New("invalid from address")
	}
	return nil
}

func hasCRLF(s string) bool { return strings.ContainsAny(s, "\r\n") }

// Send delivers msg using cfg. It honours ctx's deadline (default 30s overall).
func Send(ctx context.Context, cfg Config, msg Message) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if len(msg.To) == 0 {
		return errors.New("no recipients")
	}
	for _, a := range msg.To {
		if _, err := mail.ParseAddress(a); err != nil || hasCRLF(a) {
			return fmt.Errorf("invalid recipient address %q", a)
		}
	}
	raw, err := BuildMIME(cfg, msg, time.Now(), "")
	if err != nil {
		return err
	}

	timeout := 30 * time.Second
	if len(msg.Attachments) > 0 {
		timeout = 2 * time.Minute // attachments are slow to upload on weak connections
	}
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dialer := &net.Dialer{Deadline: deadline}
	tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}

	var conn net.Conn
	if cfg.Security == SecurityTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("connect to %s: %w", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(deadline)

	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp greeting: %w", err)
	}
	defer c.Close()

	if cfg.Security == SecuritySTARTTLS {
		// Never fall back to plaintext: if the server can't upgrade, stop here.
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("server does not support STARTTLS")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	if cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			return fmt.Errorf("smtp login: %w", err)
		}
	}

	from, _ := mail.ParseAddress(cfg.From)
	if err := c.Mail(from.Address); err != nil {
		return fmt.Errorf("smtp sender: %w", err)
	}
	for _, rcpt := range msg.To {
		a, _ := mail.ParseAddress(rcpt)
		if err := c.Rcpt(a.Address); err != nil {
			return fmt.Errorf("smtp recipient %s: %w", a.Address, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return c.Quit()
}

// BuildMIME renders msg as an RFC 5322 message. boundary may be "" to generate a
// random one (tests pass a fixed value). Header values are rejected if they could
// inject extra headers.
func BuildMIME(cfg Config, msg Message, now time.Time, boundary string) ([]byte, error) {
	if hasCRLF(cfg.From) || hasCRLF(msg.Subject) {
		return nil, errors.New("invalid header value")
	}
	for _, t := range msg.To {
		if hasCRLF(t) {
			return nil, errors.New("invalid header value")
		}
	}
	for _, a := range msg.Attachments {
		if hasCRLF(a.Filename) || hasCRLF(a.ContentType) {
			return nil, errors.New("invalid attachment header")
		}
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if boundary != "" {
		if err := mw.SetBoundary(boundary); err != nil {
			return nil, err
		}
	}

	// Headers go in front of the multipart body.
	var head bytes.Buffer
	fromAddr, _ := mail.ParseAddress(cfg.From)
	domain := "localhost"
	if i := strings.LastIndex(fromAddr.Address, "@"); i >= 0 {
		domain = fromAddr.Address[i+1:]
	}
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	fmt.Fprintf(&head, "From: %s\r\n", cfg.From)
	fmt.Fprintf(&head, "To: %s\r\n", strings.Join(msg.To, ", "))
	fmt.Fprintf(&head, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", msg.Subject))
	fmt.Fprintf(&head, "Date: %s\r\n", now.Format(time.RFC1123Z))
	fmt.Fprintf(&head, "Message-ID: <%s@%s>\r\n", hex.EncodeToString(id), domain)
	fmt.Fprintf(&head, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&head, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", mw.Boundary())

	// Text part (quoted-printable keeps non-ASCII text intact and line-safe). Plain
	// text has no alignment, so right-to-left mail also carries an HTML copy that does.
	if msg.RTL {
		if err := writeAlternative(mw, msg.Body); err != nil {
			return nil, err
		}
	} else if err := writeQP(mw, "text/plain; charset=utf-8", msg.Body); err != nil {
		return nil, err
	}

	for _, a := range msg.Attachments {
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		ap, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {fmt.Sprintf("%s; name=%q", ct, a.Filename)},
			"Content-Disposition":       {fmt.Sprintf("attachment; filename=%q", a.Filename)},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			return nil, err
		}
		enc := base64.StdEncoding.EncodeToString(a.Data)
		for len(enc) > 76 {
			if _, err := ap.Write([]byte(enc[:76] + "\r\n")); err != nil {
				return nil, err
			}
			enc = enc[76:]
		}
		if _, err := ap.Write([]byte(enc + "\r\n")); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return append(head.Bytes(), buf.Bytes()...), nil
}

func writeQP(mw *multipart.Writer, contentType, body string) error {
	tp, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {contentType},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		return err
	}
	qp := quotedprintable.NewWriter(tp)
	if _, err := qp.Write([]byte(body)); err != nil {
		return err
	}
	return qp.Close()
}

// writeAlternative adds a multipart/alternative part: the plain text plus an HTML
// version with dir="rtl" so mail clients right-align it.
func writeAlternative(mw *multipart.Writer, body string) error {
	var inner bytes.Buffer
	aw := multipart.NewWriter(&inner)
	if err := writeQP(aw, "text/plain; charset=utf-8", body); err != nil {
		return err
	}
	page := `<html dir="rtl"><body dir="rtl" style="direction:rtl;text-align:right">` +
		strings.ReplaceAll(html.EscapeString(body), "\n", "<br>\n") + `</body></html>`
	if err := writeQP(aw, "text/html; charset=utf-8", page); err != nil {
		return err
	}
	if err := aw.Close(); err != nil {
		return err
	}
	part, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Type": {"multipart/alternative; boundary=" + aw.Boundary()},
	})
	if err != nil {
		return err
	}
	_, err = part.Write(inner.Bytes())
	return err
}
