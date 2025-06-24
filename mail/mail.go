package mail

import (
	"bytes"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/pkg/config"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
	"github.com/resend/resend-go/v2"
)

var (
	verification_Email_Sender = "noreply@usestellance.com"
)

type Mailer struct {
	client *resend.Client
	log    slog.Logger
}

func NewMailer() *Mailer {
	client := resend.NewClient(os.Getenv("RESEND_KEY"))
	return &Mailer{
		client: client,
		log:    *config.GetAppContainer().Log,
	}
}

func CreateEmailToken(email string, userID string) (string, error) {
	payload := fmt.Sprintf("%s|%s|%d", email, userID, time.Now().Unix())
	return utils.EncryptEmail(payload)
}

func ParseVerificationToken(token string) (email string, userID string, err error) {
	payload, err := utils.DecryptEmail(token)
	if err != nil {
		return "", "", err
	}
	var timestamp int64
	_, err = fmt.Sscanf(payload, "%s|%s|%d", &email, &userID, &timestamp)
	if err != nil {
		return "", "", fmt.Errorf("invalid token payload")
	}

	return email, userID, nil
}

func (m *Mailer) SendVerificationEmail(email, url string) error {
	subject := "Complete Email Verification"
	t, err := template.ParseFiles("mail/templates/email_verification.html")
	if err != nil {
		return fmt.Errorf("failed to read welcome email template: %w", err)
	}

	var body bytes.Buffer
	if err := t.Execute(&body, map[string]interface{}{
		"URL":  url,
	}); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	params := &resend.SendEmailRequest{
		From:    verification_Email_Sender,
		To:      []string{email},
		Html:    body.String(),
		Subject: subject,
		ReplyTo: "support@usestellance.com",
	}

	_, err = m.client.Emails.Send(params)
	if err != nil {
		m.log.Error("error sending verification email", "email_error", err)
		return err
	}
	m.log.Debug(fmt.Sprintf("email sent successfully to %s", email))
	return nil
}
