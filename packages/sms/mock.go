package sms

import (
	"context"
	"sync"
)

// FakeProvider is an explicit in-memory test double. It never logs message
// contents and must never be selected by production construction.
type FakeProvider struct {
	mu       sync.Mutex
	lastCode string
	err      error
}

// NewFake creates an explicit test-only SMS provider.
func NewFake() *FakeProvider {
	return &FakeProvider{}
}

// SendOTP captures the code only in process memory for an isolated test.
func (m *FakeProvider) SendOTP(_ context.Context, _ string, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastCode = code
	return m.err
}

// SendMessage discards test message content.
func (m *FakeProvider) SendMessage(_ context.Context, _ string, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.err
}

// LastCode returns the last OTP code captured by the test double.
func (m *FakeProvider) LastCode() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastCode
}

// SetError configures the fake to reject delivery.
func (m *FakeProvider) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// HealthCheck reports the configured fake status.
func (m *FakeProvider) HealthCheck() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.err
}
