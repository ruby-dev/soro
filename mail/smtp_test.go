package mail

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

type received struct {
	recipients []string
	data       string
	err        error
}

func TestSMTPTransportSendsEnvelopeAndHidesBCC(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	result := make(chan received, 1)
	go serveSMTP(listener, result)
	port := listener.Addr().(*net.TCPAddr).Port
	transport, err := NewSMTPTransport(SMTPConfig{Host: "127.0.0.1", Port: port, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if transport.Name() != "smtp" {
		t.Fatalf("name = %q", transport.Name())
	}
	message := &Message{
		From: "sender@example.com", To: []string{"to@example.com"}, BCC: []string{"hidden@example.com"},
		Subject: "Hello", Text: "Body",
	}
	if err := transport.Send(t.Context(), message); err != nil {
		t.Fatal(err)
	}
	response := <-result
	if response.err != nil {
		t.Fatal(response.err)
	}
	if len(response.recipients) != 2 || !strings.Contains(strings.Join(response.recipients, " "), "hidden@example.com") {
		t.Fatalf("recipients = %v", response.recipients)
	}
	if strings.Contains(response.data, "hidden@example.com") {
		t.Fatalf("BCC leaked into data:\n%s", response.data)
	}
}

func serveSMTP(listener net.Listener, result chan<- received) {
	connection, err := listener.Accept()
	if err != nil {
		result <- received{err: err}
		return
	}
	defer connection.Close()
	reader := bufio.NewReader(connection)
	writer := bufio.NewWriter(connection)
	write := func(line string) error {
		if _, err := writer.WriteString(line + "\r\n"); err != nil {
			return err
		}
		return writer.Flush()
	}
	if err := write("220 localhost ESMTP"); err != nil {
		result <- received{err: err}
		return
	}
	var response received
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			response.err = err
			result <- response
			return
		}
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "EHLO"):
			if err := write("250 localhost"); err != nil {
				response.err = err
				result <- response
				return
			}
		case strings.HasPrefix(line, "MAIL FROM"):
			if err := write("250 OK"); err != nil {
				response.err = err
				result <- response
				return
			}
		case strings.HasPrefix(line, "RCPT TO"):
			response.recipients = append(response.recipients, line)
			if err := write("250 OK"); err != nil {
				response.err = err
				result <- response
				return
			}
		case line == "DATA":
			if err := write("354 End data with <CR><LF>.<CR><LF>"); err != nil {
				response.err = err
				result <- response
				return
			}
			var data strings.Builder
			for {
				dataLine, readErr := reader.ReadString('\n')
				if readErr != nil {
					response.err = readErr
					result <- response
					return
				}
				if strings.TrimSpace(dataLine) == "." {
					break
				}
				data.WriteString(dataLine)
			}
			response.data = data.String()
			if err := write("250 queued"); err != nil {
				response.err = err
				result <- response
				return
			}
		case line == "QUIT":
			_ = write("221 bye")
			result <- response
			return
		default:
			response.err = fmt.Errorf("unexpected SMTP command %q on port %s", line, strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
			result <- response
			return
		}
	}
}
