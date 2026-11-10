package mailer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"
)

const brevoSendURL = "https://api.brevo.com/v3/smtp/email"

type Mailer struct {
	host        string
	port        string
	username    string
	password    string
	from        string
	fromName    string
	brevoAPIKey string
	appName     string
	httpClient  *http.Client
}

func NewFromEnv() *Mailer {
	m := &Mailer{
		host:        strings.TrimSpace(os.Getenv("SMTP_HOST")),
		port:        firstNonEmpty(os.Getenv("SMTP_PORT"), "587"),
		username:    strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		password:    strings.TrimSpace(os.Getenv("SMTP_PASSWORD")),
		from:        firstNonEmpty(os.Getenv("SMTP_FROM"), os.Getenv("BREVO_FROM"), os.Getenv("SMTP_USERNAME"), "noreply@localhost"),
		fromName:    firstNonEmpty(os.Getenv("SMTP_FROM_NAME"), os.Getenv("APP_NAME"), "LIFE+"),
		brevoAPIKey: strings.TrimSpace(os.Getenv("BREVO_API_KEY")),
		appName:     firstNonEmpty(os.Getenv("APP_NAME"), "LIFE+"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}

	// Incomplete SMTP config stays unused so local/API-only setups keep working.
	if m.host != "" && (m.username == "" || m.password == "") {
		log.Printf("[mailer] SMTP host set but username/password incomplete — ignoring SMTP")
		m.host = ""
	}

	switch {
	case m.brevoAPIKey != "":
		log.Printf("[mailer] Brevo API enabled from=%s name=%s", m.from, m.fromName)
	case m.host != "":
		log.Printf("[mailer] SMTP enabled host=%s port=%s from=%s", m.host, m.port, m.from)
	default:
		log.Printf("[mailer] email not configured — emails will be logged only")
	}

	return m
}

func (m *Mailer) SendPasswordReset(to string, data PasswordResetEmailData) error {
	if data.AppName == "" {
		data.AppName = m.appName
	}

	rendered, err := renderPasswordResetEmail(data)
	if err != nil {
		return err
	}

	return m.send(to, rendered.Subject, rendered.TextBody, rendered.HTMLBody)
}

func (m *Mailer) SendEmailVerification(to string, data EmailVerificationData) error {
	if data.AppName == "" {
		data.AppName = m.appName
	}

	rendered, err := renderEmailVerificationEmail(data)
	if err != nil {
		return err
	}

	return m.send(to, rendered.Subject, rendered.TextBody, rendered.HTMLBody)
}

func (m *Mailer) send(to, subject, textBody, htmlBody string) error {
	if m.brevoAPIKey != "" {
		if err := m.sendViaBrevoAPI(to, subject, textBody, htmlBody); err != nil {
			return err
		}
		log.Printf("[mailer] sent email via Brevo API to=%s subject=%q", to, subject)
		return nil
	}

	if m.host == "" {
		log.Printf(
			"[mailer] email not configured — email to=%s subject=%q\n%s\n--- html ---\n%s",
			to,
			subject,
			textBody,
			htmlBody,
		)
		return nil
	}

	if err := m.sendViaSMTP(to, subject, textBody, htmlBody); err != nil {
		return err
	}

	log.Printf("[mailer] sent email via SMTP to=%s subject=%q", to, subject)
	return nil
}

type brevoAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type brevoSendRequest struct {
	Sender      brevoAddress   `json:"sender"`
	To          []brevoAddress `json:"to"`
	Subject     string         `json:"subject"`
	HTMLContent string         `json:"htmlContent"`
	TextContent string         `json:"textContent,omitempty"`
}

func (m *Mailer) sendViaBrevoAPI(to, subject, textBody, htmlBody string) error {
	payload := brevoSendRequest{
		Sender: brevoAddress{
			Email: m.from,
			Name:  m.fromName,
		},
		To: []brevoAddress{
			{Email: to},
		},
		Subject:     subject,
		HTMLContent: htmlBody,
		TextContent: textBody,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("brevo encode failed: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, brevoSendURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("brevo request failed: %w", err)
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("api-key", m.brevoAPIKey)

	res, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("brevo send failed: %w", err)
	}
	defer res.Body.Close()

	resBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("brevo send failed: status=%d body=%s", res.StatusCode, strings.TrimSpace(string(resBody)))
	}

	return nil
}

func (m *Mailer) sendViaSMTP(to, subject, textBody, htmlBody string) error {
	encodedSubject := mime.QEncoding.Encode("UTF-8", subject)
	boundary := "life-plus-boundary"

	var msgBuilder strings.Builder
	msgBuilder.WriteString(fmt.Sprintf("From: %s\r\n", formatFromHeader(m.fromName, m.from)))
	msgBuilder.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msgBuilder.WriteString(fmt.Sprintf("Subject: %s\r\n", encodedSubject))
	msgBuilder.WriteString("MIME-Version: 1.0\r\n")
	msgBuilder.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
	msgBuilder.WriteString("\r\n")

	msgBuilder.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msgBuilder.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	msgBuilder.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	msgBuilder.WriteString(textBody)
	msgBuilder.WriteString("\r\n")

	msgBuilder.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msgBuilder.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	msgBuilder.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	msgBuilder.WriteString(htmlBody)
	msgBuilder.WriteString("\r\n")

	msgBuilder.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	addr := fmt.Sprintf("%s:%s", m.host, m.port)
	var auth smtp.Auth
	if m.username != "" {
		auth = smtp.PlainAuth("", m.username, m.password, m.host)
	}

	if err := smtp.SendMail(addr, auth, m.from, []string{to}, []byte(msgBuilder.String())); err != nil {
		return fmt.Errorf("smtp send failed: %w", err)
	}

	return nil
}

func formatFromHeader(name, email string) string {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" {
		return email
	}
	return fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("UTF-8", name), email)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
