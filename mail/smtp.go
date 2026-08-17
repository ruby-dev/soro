package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	stdmail "net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host               string
	Port               int
	Username           string
	Password           string
	StartTLS           bool
	ImplicitTLS        bool
	InsecureSkipVerify bool
	Timeout            time.Duration
}

type SMTPTransport struct{ config SMTPConfig }

func NewSMTPTransport(config SMTPConfig) (*SMTPTransport, error) {
	if config.Host == "" || config.Port < 1 || config.Port > 65535 {
		return nil, fmt.Errorf("mail SMTP host and valid port are required")
	}
	if config.StartTLS && config.ImplicitTLS {
		return nil, fmt.Errorf("mail SMTP STARTTLS and implicit TLS are mutually exclusive")
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.Timeout < 0 {
		return nil, fmt.Errorf("mail SMTP timeout cannot be negative")
	}
	if (config.Username == "") != (config.Password == "") {
		return nil, fmt.Errorf("mail SMTP username and password must be configured together")
	}
	return &SMTPTransport{config: config}, nil
}

func (transport *SMTPTransport) Name() string { return "smtp" }

func (transport *SMTPTransport) Send(ctx context.Context, message *Message) error {
	if err := message.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, transport.config.Timeout)
	defer cancel()
	address := net.JoinHostPort(transport.config.Host, strconv.Itoa(transport.config.Port))
	dialer := net.Dialer{}
	if deadline, ok := ctx.Deadline(); ok {
		dialer.Deadline = deadline
	}
	var connection net.Conn
	var err error
	tlsConfig := &tls.Config{ServerName: transport.config.Host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: transport.config.InsecureSkipVerify} //nolint:gosec
	if transport.config.ImplicitTLS {
		connection, err = tls.DialWithDialer(&dialer, "tcp", address, tlsConfig)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("mail SMTP connect: %w", err)
	}
	defer connection.Close()
	stopCancellation := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stopCancellation()
	client, err := smtp.NewClient(connection, transport.config.Host)
	if err != nil {
		return fmt.Errorf("mail SMTP initialize: %w", err)
	}
	defer client.Close()
	if transport.config.StartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("mail SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("mail SMTP STARTTLS: %w", err)
		}
	}
	if transport.config.Username != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return fmt.Errorf("mail SMTP server does not support authentication")
		}
		if err := client.Auth(smtp.PlainAuth("", transport.config.Username, transport.config.Password, transport.config.Host)); err != nil {
			return fmt.Errorf("mail SMTP authentication failed")
		}
	}
	from, _ := netMailAddress(message.From)
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("mail SMTP sender rejected: %w", err)
	}
	for _, recipient := range message.recipients() {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("mail SMTP recipient rejected: %w", err)
		}
	}
	data, err := client.Data()
	if err != nil {
		return fmt.Errorf("mail SMTP begin message: %w", err)
	}
	encoded, err := encodeMessage(message)
	if err == nil {
		_, err = data.Write(encoded)
	}
	closeErr := data.Close()
	if err != nil {
		return fmt.Errorf("mail SMTP write message: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("mail SMTP finish message: %w", closeErr)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("mail SMTP quit: %w", err)
	}
	return nil
}

func encodeMessage(message *Message) ([]byte, error) {
	var output bytes.Buffer
	writeHeader := func(name, value string) { fmt.Fprintf(&output, "%s: %s\r\n", name, value) }
	writeHeader("From", message.From)
	writeHeader("To", strings.Join(message.To, ", "))
	if len(message.CC) > 0 {
		writeHeader("Cc", strings.Join(message.CC, ", "))
	}
	writeHeader("Subject", mime.QEncoding.Encode("utf-8", message.Subject))
	writeHeader("MIME-Version", "1.0")
	if message.Text != "" && message.HTML != "" {
		multipartWriter := multipart.NewWriter(&output)
		writeHeader("Content-Type", `multipart/alternative; boundary="`+multipartWriter.Boundary()+`"`)
		output.WriteString("\r\n")
		if err := writePart(multipartWriter, "text/plain; charset=utf-8", message.Text); err != nil {
			return nil, err
		}
		if err := writePart(multipartWriter, "text/html; charset=utf-8", message.HTML); err != nil {
			return nil, err
		}
		if err := multipartWriter.Close(); err != nil {
			return nil, err
		}
		return output.Bytes(), nil
	}
	contentType, body := "text/plain; charset=utf-8", message.Text
	if message.HTML != "" {
		contentType, body = "text/html; charset=utf-8", message.HTML
	}
	writeHeader("Content-Type", contentType)
	writeHeader("Content-Transfer-Encoding", "quoted-printable")
	output.WriteString("\r\n")
	writer := quotedprintable.NewWriter(&output)
	if _, err := io.WriteString(writer, body); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func writePart(writer *multipart.Writer, contentType, body string) error {
	header := make(textproto.MIMEHeader)
	header["Content-Type"] = []string{contentType}
	header["Content-Transfer-Encoding"] = []string{"quoted-printable"}
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	encoded := quotedprintable.NewWriter(part)
	if _, err := io.WriteString(encoded, body); err != nil {
		return err
	}
	return encoded.Close()
}

func netMailAddress(value string) (string, error) {
	address, err := stdmail.ParseAddress(value)
	if err != nil {
		return "", err
	}
	return address.Address, nil
}
