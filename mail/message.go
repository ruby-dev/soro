// Package mail provides Soro messages, templates, transports, and queued delivery.
package mail

import (
	"fmt"
	stdmail "net/mail"
	"strings"
)

type Message struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	CC      []string `json:"cc,omitempty"`
	BCC     []string `json:"bcc,omitempty"`
	Subject string   `json:"subject"`
	Text    string   `json:"text,omitempty"`
	HTML    string   `json:"html,omitempty"`
}

func (message *Message) Validate() error {
	if message == nil {
		return fmt.Errorf("mail message is required")
	}
	if err := validAddress("from", message.From); err != nil {
		return err
	}
	if len(message.To)+len(message.CC)+len(message.BCC) == 0 {
		return fmt.Errorf("mail message requires a recipient")
	}
	for _, group := range []struct {
		name   string
		values []string
	}{{"to", message.To}, {"cc", message.CC}, {"bcc", message.BCC}} {
		for _, value := range group.values {
			if err := validAddress(group.name, value); err != nil {
				return err
			}
		}
	}
	if strings.TrimSpace(message.Subject) == "" || strings.ContainsAny(message.Subject, "\r\n") {
		return fmt.Errorf("mail subject is required and cannot contain newlines")
	}
	if message.Text == "" && message.HTML == "" {
		return fmt.Errorf("mail message requires text or HTML content")
	}
	return nil
}

func (message *Message) Clone() *Message {
	if message == nil {
		return nil
	}
	cloned := *message
	cloned.To = append([]string(nil), message.To...)
	cloned.CC = append([]string(nil), message.CC...)
	cloned.BCC = append([]string(nil), message.BCC...)
	return &cloned
}

func (message *Message) recipients() []string {
	result := make([]string, 0, len(message.To)+len(message.CC)+len(message.BCC))
	for _, values := range [][]string{message.To, message.CC, message.BCC} {
		for _, value := range values {
			address, _ := stdmail.ParseAddress(value)
			result = append(result, address.Address)
		}
	}
	return result
}

func validAddress(field, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("mail %s address cannot contain newlines", field)
	}
	address, err := stdmail.ParseAddress(value)
	if err != nil || address.Address == "" {
		return fmt.Errorf("mail %s address %q is invalid", field, value)
	}
	return nil
}
