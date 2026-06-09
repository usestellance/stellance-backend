package mail

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"time"

	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/resend/resend-go/v2"
)

type SendInvoiceEmailData struct {
	PrimaryRecipient string
	PayerName        string
	SenderName       string
	InvoiceURL       string
}

type InvoiceReviewNotificationData struct {
	CreatorEmail   string
	CreatorName    string
	PayerName      string
	InvoiceNumber  string
	Total          string
	Currency       string
	Approved       bool
	DashboardURL   string
}

var (
	verification_Email_Sender = "noreply@usestellance.com"
	//go:embed templates/*.html
	templateFs embed.FS
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
	t, err := template.ParseFS(templateFs, "templates/email_verification.html")
	if err != nil {
		return fmt.Errorf("failed to read welcome email template: %w", err)
	}

	var body bytes.Buffer
	if err := t.Execute(&body, map[string]interface{}{
		"URL": url,
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

func (m *Mailer) SendResetEmail(email, url, otp string) error {
	subject := "Reset Password Request"
	t, err := template.ParseFS(templateFs, "templates/reset_email.html")
	if err != nil {
		return fmt.Errorf("failed to read reset email template: %w", err)
	}
	var body bytes.Buffer
	if err := t.Execute(&body, map[string]interface{}{
		"URL": url,
		"OTP": otp,
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

func RenderInvoicePDF(data map[string]any) ([]byte, error) {
	t, err := template.ParseFS(templateFs, "templates/invoice_pdf.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse invoice pdf template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to render invoice pdf template: %w", err)
	}
	return buf.Bytes(), nil
}

func (m *Mailer) SendInvoiceReviewNotification(data InvoiceReviewNotificationData) error {
	subject := "Invoice Approved"
	if !data.Approved {
		subject = "Invoice Declined"
	}

	t, err := template.ParseFS(templateFs, "templates/invoice_review_notification.html")
	if err != nil {
		return fmt.Errorf("failed to read invoice review notification template: %w", err)
	}

	var body bytes.Buffer
	if err := t.Execute(&body, map[string]interface{}{
		"CREATOR_NAME":    data.CreatorName,
		"PAYER_NAME":      data.PayerName,
		"INVOICE_NUMBER":  data.InvoiceNumber,
		"TOTAL":           data.Total,
		"CURRENCY":        data.Currency,
		"APPROVED":        data.Approved,
		"DASHBOARD_URL":   data.DashboardURL,
	}); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	params := &resend.SendEmailRequest{
		From:    verification_Email_Sender,
		To:      []string{data.CreatorEmail},
		Html:    body.String(),
		Subject: subject,
		ReplyTo: "support@usestellance.com",
	}

	_, err = m.client.Emails.Send(params)
	if err != nil {
		m.log.Error("error sending invoice review notification", "email_error", err, "recipient", data.CreatorEmail)
		return err
	}
	m.log.Debug(fmt.Sprintf("invoice review notification sent to %s", data.CreatorEmail))
	return nil
}

func (m *Mailer) SendInvoiceUrlMail(data SendInvoiceEmailData) error {
	subject := "Invoice Review"

	t, err := template.ParseFS(templateFs, "templates/send_invoice.html")
	if err != nil {
		return fmt.Errorf("failed to read invoice email template: %w", err)
	}

	var body bytes.Buffer
	if err := t.Execute(&body, map[string]interface{}{
		"URL":        data.InvoiceURL,
		"PAYER_NAME": data.PayerName,
		"SENDER":     data.SenderName,
	}); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	params := &resend.SendEmailRequest{
		From:    verification_Email_Sender,
		To:      []string{data.PrimaryRecipient},
		Html:    body.String(),
		Subject: subject,
		ReplyTo: "support@usestellance.com",
	}

	sent, err := m.client.Emails.Send(params)
	if err != nil {
		m.log.Error("error sending invoice email",
			"email_error", err,
			"primary", data.PrimaryRecipient,
		)
		return fmt.Errorf("failed to send email: %w", err)
	}

	m.log.Debug("email sent successfully",
		"email_id", sent.Id,
		"primary_recipient", data.PrimaryRecipient,
	)

	return nil
}
