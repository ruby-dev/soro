package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

type Transport interface {
	Send(context.Context, *Message) error
}

type namedTransport interface{ Name() string }

type ConsoleTransport struct {
	Writer io.Writer
	mu     sync.Mutex
}

func (transport *ConsoleTransport) Name() string { return "console" }

func (transport *ConsoleTransport) Send(ctx context.Context, message *Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := message.Validate(); err != nil {
		return err
	}
	if transport.Writer == nil {
		return fmt.Errorf("mail console writer is required")
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if err := json.NewEncoder(transport.Writer).Encode(message); err != nil {
		return fmt.Errorf("mail console transport: %w", err)
	}
	return nil
}

type CaptureTransport struct {
	mu       sync.RWMutex
	messages []*Message
}

func NewCaptureTransport() *CaptureTransport { return &CaptureTransport{} }

func (transport *CaptureTransport) Name() string { return "capture" }

func (transport *CaptureTransport) Send(ctx context.Context, message *Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := message.Validate(); err != nil {
		return err
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.messages = append(transport.messages, message.Clone())
	return nil
}

func (transport *CaptureTransport) Messages() []*Message {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	result := make([]*Message, len(transport.messages))
	for index, message := range transport.messages {
		result[index] = message.Clone()
	}
	return result
}

func (transport *CaptureTransport) Reset() {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	transport.messages = nil
}
