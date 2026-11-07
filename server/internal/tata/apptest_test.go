package tata

import (
	"strings"
	"sync"
)

type captureMailer struct {
	mu   sync.Mutex
	last map[string]string
}

func (m *captureMailer) Ready() bool { return true }

func (m *captureMailer) SendCode(email, code, scene string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.last == nil {
		m.last = map[string]string{}
	}
	m.last[strings.ToLower(email)] = code
	return nil
}

func (m *captureMailer) Last(email string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last[strings.ToLower(email)]
}

var testMail = &captureMailer{}

func testApp(store *Store) *App {
	return &App{Store: store, JWTSecret: []byte("test-secret"), Mail: testMail, KV: newMemoryKV()}
}
