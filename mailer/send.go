package mailer

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/mail"
	"net/smtp"
	"strings"

	"github.com/keur/chillmailer/util"
)

func SendMail(from string, to string, subject string, body string, unsubscribeLink string) error {
	conn, err := getConnection()
	if err != nil {
		return err
	}
	return sendMessage(conn, from, to, subject, body, unsubscribeLink)
}

type TlsSmtpConn struct {
	Conn *tls.Conn
	Host string
	Auth smtp.Auth
}

func getConnection() (*TlsSmtpConn, error) {
	host, err := util.GetenvOrError("SMTP_HOST")
	if err != nil {
		return nil, err
	}
	port := util.GetenvOr("SMTP_PORT", "465")
	server := host + ":" + port

	user, err := util.GetenvOrError("SMTP_USER")
	if err != nil {
		return nil, err
	}
	pass, err := util.GetenvOrError("SMTP_PASS")
	if err != nil {
		return nil, err
	}
	auth := smtp.PlainAuth("", user, pass, host)

	tlsconfig := &tls.Config{
		ServerName: host,
	}

	// Here is the key, you need to call tls.Dial instead of smtp.Dial
	// for smtp servers running on 465 that require an ssl connection
	// from the very beginning (no starttls)
	conn, err := tls.Dial("tcp", server, tlsconfig)
	if err != nil {
		return nil, err
	}

	tlsSmtpConn := &TlsSmtpConn{
		Conn: conn,
		Host: host,
		Auth: auth,
	}
	return tlsSmtpConn, nil
}

type EmailData struct {
	Subject         string
	Body            string
	UnsubscribeLink string
}

func sendMessage(conn *TlsSmtpConn, from string, to string, subject string, body string, unsubscribeLink string) error {
	fromAddr := mail.Address{"", from}
	toAddr := mail.Address{"", to}

	c, err := smtp.NewClient(conn.Conn, conn.Host)
	if err != nil {
		return err
	}
	defer func() {
		if quitErr := c.Quit(); err == nil {
			err = quitErr
		}
	}()

	// Auth
	if err = c.Auth(conn.Auth); err != nil {
		return err
	}

	// To && From
	if err = c.Mail(fromAddr.Address); err != nil {
		return err
	}

	if err = c.Rcpt(toAddr.Address); err != nil {
		return err
	}

	// Data
	w, err := c.Data()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := c.Close(); err == nil {
			err = closeErr
		}
	}()

	// Setup headers
	headers := make(map[string]string)
	headers["From"] = fromAddr.String()
	headers["To"] = toAddr.String()
	headers["Subject"] = subject
	headers["Content-Type"] = "text/html; charset=\"utf-8\""

	// Setup message
	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	htmlBuffer := new(bytes.Buffer)
	tmpl, err := util.NewTemplate("email.html")
	if err != nil {
		return err
	}
	pageData := EmailData{Subject: subject, Body: htmlifyBody(body), UnsubscribeLink: unsubscribeLink}
	if err = tmpl.Execute(htmlBuffer, &pageData); err != nil {
		return err
	}
	message += "\r\n" + htmlBuffer.String()
	_, err = io.WriteString(w, message)
	if err != nil {
		return err
	}

	return err
}

func htmlifyBody(body string) string {
	lines := strings.Split(body, "\n")

	var nonEmptyLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			nonEmptyLines = append(nonEmptyLines, trimmed)
		}
	}

	return "<p>" + strings.Join(nonEmptyLines, "</p><p>") + "</p>"
}
