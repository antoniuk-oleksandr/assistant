package notification

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

type smtpSender interface {
	Send(context.Context, string, string, string, string) error
}

type googleSMTPClient struct {
	host     string
	port     int
	username string
	password string
}

func newGoogleSMTPClient(cfgHost string, cfgPort int, username, password string) *googleSMTPClient {
	host := cfgHost
	if host == "" {
		host = "smtp.gmail.com"
	}
	if cfgPort == 0 {
		cfgPort = 587
	}
	return &googleSMTPClient{host: host, port: cfgPort, username: username, password: password}
}

func (c *googleSMTPClient) Send(ctx context.Context, from, to, subject, body string) error {
	if c.username == "" || c.password == "" {
		return errors.New("SMTP credentials are not configured")
	}
	for field, value := range map[string]string{"from": from, "to": to, "subject": subject} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("SMTP %s contains a newline", field)
		}
	}
	address := net.JoinHostPort(c.host, strconv.Itoa(c.port))
	dialer := &net.Dialer{}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(connection, c.host)
	if err != nil {
		_ = connection.Close()
		return err
	}
	defer client.Close()
	if err := client.StartTLS(&tls.Config{ServerName: c.host, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("start SMTP TLS: %w", err)
	}
	if err := client.Auth(smtp.PlainAuth("", c.username, c.password, c.host)); err != nil {
		return fmt.Errorf("authenticate with SMTP: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintf(writer, "From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", from, to, subject, body)
	closeErr := writer.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
