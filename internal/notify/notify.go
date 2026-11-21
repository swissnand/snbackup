// Package notify sends e-mail (SMTP) and webhook notifications
// (Slack, Microsoft Teams, Discord, ntfy, Gotify, generic JSON).
package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/swissnand/snbackup/internal/store"
)

// Message is a notification.
type Message struct {
	Title  string `json:"title"`
	Text   string `json:"text"`
	Status string `json:"status"` // success | warning | failed | info
	Job    string `json:"job,omitempty"`
	Server string `json:"server"`
	Time   string `json:"time"`
}

// Email sends a plain-text e-mail.
func Email(cfg store.SMTP, to []string, m Message) error {
	if cfg.Host == "" || len(to) == 0 {
		return errors.New("SMTP nicht konfiguriert oder keine Empfänger")
	}
	from := cfg.From
	if from == "" {
		from = cfg.User
	}
	for _, a := range append([]string{from}, to...) {
		if strings.ContainsAny(a, "\r\n") {
			return errors.New("ungültige E-Mail-Adresse")
		}
	}
	icon := map[string]string{"success": "✅", "warning": "⚠️", "failed": "❌"}[m.Status]
	subject := strings.TrimSpace(icon + " " + m.Title)
	body := fmt.Sprintf("%s\r\n\r\nServer: %s\r\nZeit: %s\r\n\r\n-- \r\nSWISSNAND Backup\r\n", m.Text, m.Server, m.Time)
	msg := "From: SWISSNAND Backup <" + from + ">\r\nTo: " + strings.Join(to, ", ") + "\r\nSubject: " + subject +
		"\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nDate: " + time.Now().Format(time.RFC1123Z) + "\r\n\r\n" + body
	addr := net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port))
	tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	var c *smtp.Client
	var err error
	if cfg.TLS == "tls" {
		conn, e := tls.DialWithDialer(&net.Dialer{Timeout: 20 * time.Second}, "tcp", addr, tlsCfg)
		if e != nil {
			return e
		}
		c, err = smtp.NewClient(conn, cfg.Host)
	} else {
		conn, e := net.DialTimeout("tcp", addr, 20*time.Second)
		if e != nil {
			return e
		}
		c, err = smtp.NewClient(conn, cfg.Host)
	}
	if err != nil {
		return err
	}
	defer c.Close()
	if cfg.TLS == "starttls" {
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}
	if cfg.User != "" {
		if err := c.Auth(smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, r := range to {
		if err := c.Rcpt(strings.TrimSpace(r)); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// Webhook posts the message in the format the target understands.
func Webhook(ctx context.Context, url string, m Message) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://localhost") && !strings.HasPrefix(url, "http://127.0.0.1") {
		return errors.New("Webhooks müssen https verwenden")
	}
	color := map[string]string{"success": "#1f9d55", "warning": "#d9822b", "failed": "#d64545"}[m.Status]
	if color == "" {
		color = "#004984"
	}
	text := "*" + m.Title + "*\n" + m.Text + "\n_" + m.Server + " · " + m.Time + "_"
	var payload any
	hdr := map[string]string{"Content-Type": "application/json"}
	switch {
	case strings.Contains(url, "hooks.slack.com"):
		payload = map[string]any{"text": text, "attachments": []any{map[string]any{"color": color, "text": m.Text}}}
	case strings.Contains(url, "discord.com/api/webhooks"):
		payload = map[string]any{"embeds": []any{map[string]any{"title": m.Title, "description": m.Text, "color": hexColor(color), "footer": map[string]string{"text": m.Server + " · " + m.Time}}}}
	case strings.Contains(url, "office.com") || strings.Contains(url, "webhook.office") || strings.Contains(url, "logic.azure.com"):
		payload = map[string]any{"@type": "MessageCard", "@context": "http://schema.org/extensions", "themeColor": strings.TrimPrefix(color, "#"), "summary": m.Title, "title": m.Title, "text": strings.ReplaceAll(m.Text, "\n", "<br>")}
	case strings.Contains(url, "ntfy"):
		hdr["Content-Type"] = "text/plain"
		hdr["Title"] = m.Title
		hdr["Tags"] = map[string]string{"success": "white_check_mark", "warning": "warning", "failed": "x"}[m.Status]
		payload = m.Text
	case strings.Contains(url, "/message?token="): // Gotify
		payload = map[string]any{"title": m.Title, "message": m.Text, "priority": map[string]int{"failed": 8, "warning": 5}[m.Status]}
	default:
		payload = m
	}
	var body []byte
	if s, ok := payload.(string); ok {
		body = []byte(s)
	} else {
		body, _ = json.Marshal(payload)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Webhook HTTP %d", resp.StatusCode)
	}
	return nil
}

func hexColor(c string) int {
	var v int
	fmt.Sscanf(strings.TrimPrefix(c, "#"), "%x", &v)
	return v
}
