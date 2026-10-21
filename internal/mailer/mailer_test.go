package mailer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"
)

var okCfg = Config{Host: "smtp.example.com", Port: 587, Security: SecuritySTARTTLS, Username: "u", Password: "p", From: "Reports <reports@example.com>"}

func TestBuildMIMEStructure(t *testing.T) {
	data := bytes.Repeat([]byte("PK\x03\x04binary"), 50)
	raw, err := BuildMIME(okCfg, Message{
		To:      []string{"admin@example.com"},
		Subject: "Payments — October 2026",
		Body:    "Hello — attached.\nSecond line.",
		Attachments: []Attachment{{
			Filename: "payments-2026-10.xlsx", ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Data: data,
		}},
	}, time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC), "BOUNDARY")
	if err != nil {
		t.Fatalf("BuildMIME: %v", err)
	}

	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Header.Get("To"); got != "admin@example.com" {
		t.Fatalf("To = %q", got)
	}
	subj, err := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if err != nil || subj != "Payments — October 2026" {
		t.Fatalf("Subject = %q, %v", subj, err)
	}
	_, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || params["boundary"] != "BOUNDARY" {
		t.Fatalf("content-type params = %v, %v", params, err)
	}

	mr := multipart.NewReader(m.Body, params["boundary"])
	text, err := mr.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(quotedprintable.NewReader(text))
	if !strings.Contains(string(body), "Hello — attached.") {
		t.Fatalf("body = %q", body)
	}
	att, err := mr.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if att.FileName() != "payments-2026-10.xlsx" {
		t.Fatalf("filename = %q", att.FileName())
	}
	enc, _ := io.ReadAll(att)
	got, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(enc), "\r\n", ""))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("attachment round-trip failed: %v", err)
	}
}

func TestBuildMIMERejectsHeaderInjection(t *testing.T) {
	cases := map[string]Message{
		"subject":  {To: []string{"a@example.com"}, Subject: "hi\r\nBcc: evil@example.com", Body: "x"},
		"to":       {To: []string{"a@example.com\r\nBcc: evil@example.com"}, Subject: "hi", Body: "x"},
		"filename": {To: []string{"a@example.com"}, Subject: "hi", Body: "x", Attachments: []Attachment{{Filename: "a\r\nX: y", Data: []byte("d")}}},
	}
	for name, msg := range cases {
		if _, err := BuildMIME(okCfg, msg, time.Now(), ""); err == nil {
			t.Errorf("%s: header injection was not rejected", name)
		}
	}
	bad := okCfg
	bad.From = "a@example.com\r\nBcc: evil@example.com"
	if _, err := BuildMIME(bad, Message{To: []string{"a@example.com"}, Subject: "s"}, time.Now(), ""); err == nil {
		t.Error("from: header injection was not rejected")
	}
}

func TestConfigValidate(t *testing.T) {
	if err := okCfg.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	mutate := func(f func(*Config)) Config { c := okCfg; f(&c); return c }
	bad := map[string]Config{
		"empty host":       mutate(func(c *Config) { c.Host = "" }),
		"host with space":  mutate(func(c *Config) { c.Host = "smtp example.com" }),
		"port zero":        mutate(func(c *Config) { c.Port = 0 }),
		"port too big":     mutate(func(c *Config) { c.Port = 70000 }),
		"unknown security": mutate(func(c *Config) { c.Security = "ssl3" }),
		"bad from":         mutate(func(c *Config) { c.From = "not an address" }),
		"crlf in password": mutate(func(c *Config) { c.Password = "a\r\nb" }),
		"creds over plain": mutate(func(c *Config) { c.Security = SecurityNone }),
	}
	for name, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if err := mutate(func(c *Config) { c.Security = SecurityNone }).Validate(); !errors.Is(err, ErrInsecureCredentials) {
		t.Errorf("creds over plaintext should be ErrInsecureCredentials, got %v", err)
	}
	// A local relay with no login may use no encryption.
	if err := mutate(func(c *Config) { c.Security = SecurityNone; c.Username = ""; c.Password = "" }).Validate(); err != nil {
		t.Errorf("no-auth plaintext relay should be allowed: %v", err)
	}
}

// fakeSMTP is a tiny single-connection plaintext SMTP server that records the
// envelope and message so Send can be tested end to end without any network.
type fakeSMTP struct {
	ln   net.Listener
	done chan struct{}
	from string
	rcpt []string
	data string
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln, done: make(chan struct{})}
	go func() {
		defer close(f.done)
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		w := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
		w("220 fake ESMTP")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			up := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
				w("250 fake")
			case strings.HasPrefix(up, "MAIL FROM:"):
				f.from = line[len("MAIL FROM:"):]
				w("250 ok")
			case strings.HasPrefix(up, "RCPT TO:"):
				f.rcpt = append(f.rcpt, line[len("RCPT TO:"):])
				w("250 ok")
			case up == "DATA":
				w("354 go ahead")
				var sb strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil || l == ".\r\n" {
						break
					}
					sb.WriteString(l)
				}
				f.data = sb.String()
				w("250 queued")
			case up == "QUIT":
				w("221 bye")
				return
			default:
				w("250 ok")
			}
		}
	}()
	return f
}

func TestSendEndToEndPlaintextRelay(t *testing.T) {
	f := newFakeSMTP(t)
	defer f.ln.Close()
	host, portStr, _ := net.SplitHostPort(f.ln.Addr().String())
	var port int
	for _, ch := range portStr {
		port = port*10 + int(ch-'0')
	}

	cfg := Config{Host: host, Port: port, Security: SecurityNone, From: "reports@example.com"}
	err := Send(context.Background(), cfg, Message{
		To: []string{"admin@example.com"}, Subject: "Test", Body: "hello",
		Attachments: []Attachment{{Filename: "a.txt", ContentType: "text/plain", Data: []byte("data")}},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	<-f.done
	if !strings.Contains(f.from, "reports@example.com") {
		t.Errorf("MAIL FROM = %q", f.from)
	}
	if len(f.rcpt) != 1 || !strings.Contains(f.rcpt[0], "admin@example.com") {
		t.Errorf("RCPT TO = %v", f.rcpt)
	}
	if !strings.Contains(f.data, "Subject: Test") || !strings.Contains(f.data, `filename="a.txt"`) {
		t.Errorf("message missing expected parts:\n%s", f.data)
	}
}

func TestSendRefusesCredentialsOverPlaintext(t *testing.T) {
	cfg := Config{Host: "127.0.0.1", Port: 1, Security: SecurityNone, Username: "u", Password: "p", From: "a@example.com"}
	err := Send(context.Background(), cfg, Message{To: []string{"b@example.com"}, Subject: "s", Body: "b"})
	if !errors.Is(err, ErrInsecureCredentials) {
		t.Fatalf("expected ErrInsecureCredentials, got %v", err)
	}
}

// Right-to-left mail carries an HTML copy that mail clients right-align; the plain
// part stays available and left-to-right mail is unchanged (single plain part).
func TestBuildMIMERightToLeftHasHTMLAlternative(t *testing.T) {
	raw, err := BuildMIME(okCfg, Message{To: []string{"a@example.com"}, Subject: "s", Body: "مرحبا <b>\nسطر", RTL: true}, time.Now(), "")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{"multipart/alternative", "text/plain", "text/html", "dir=3D\"rtl\"", "&lt;b&gt;"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	plain, _ := BuildMIME(okCfg, Message{To: []string{"a@example.com"}, Subject: "s", Body: "hi"}, time.Now(), "")
	if strings.Contains(string(plain), "text/html") {
		t.Fatal("left-to-right mail should not get an HTML part")
	}
}
