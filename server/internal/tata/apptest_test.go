package tata

import (
	"context"
	"strings"
	"sync"
	"testing"
)

type passthroughWeChat struct{}

func (passthroughWeChat) Ready() bool { return true }

func (passthroughWeChat) ExchangeCode(_ context.Context, code string) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", biz("微信授权码不能为空")
	}
	return code, nil
}

type passthroughApple struct{}

func (passthroughApple) Ready() bool { return true }

func (passthroughApple) VerifyIdentityToken(_ context.Context, identityToken string) (string, error) {
	identityToken = strings.TrimSpace(identityToken)
	if identityToken == "" {
		return "", biz("苹果身份令牌不能为空")
	}
	return identityToken, nil
}

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

func protectLiveAIConfig(t *testing.T, store *Store) {
	t.Helper()
	type bind struct {
		task     string
		vendorID int64
		model    string
	}
	ctx := t.Context()
	rows, err := store.Query(ctx, `SELECT task, vendor_id, model FROM bindings`)
	if err != nil {
		t.Fatal(err)
	}
	var saved []bind
	for rows.Next() {
		var item bind
		if err := rows.Scan(&item.task, &item.vendorID, &item.model); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		saved = append(saved, item)
	}
	rows.Close()
	idRows, err := store.Query(ctx, `SELECT id FROM vendors`)
	if err != nil {
		t.Fatal(err)
	}
	keep := map[int64]struct{}{}
	for idRows.Next() {
		var id int64
		if err := idRows.Scan(&id); err != nil {
			idRows.Close()
			t.Fatal(err)
		}
		keep[id] = struct{}{}
	}
	idRows.Close()
	t.Cleanup(func() {
		c := context.Background()
		for _, item := range saved {
			_, _ = store.Exec(c, `INSERT INTO bindings(task, vendor_id, model, updated_at) VALUES ($1,$2,$3,now())
				ON CONFLICT (task) DO UPDATE SET vendor_id=EXCLUDED.vendor_id, model=EXCLUDED.model, updated_at=now()`,
				item.task, item.vendorID, item.model)
		}
		if len(keep) == 0 {
			return
		}
		cur, err := store.Query(c, `SELECT id FROM vendors`)
		if err != nil {
			return
		}
		var extra []int64
		for cur.Next() {
			var id int64
			if err := cur.Scan(&id); err != nil {
				cur.Close()
				return
			}
			if _, ok := keep[id]; !ok {
				extra = append(extra, id)
			}
		}
		cur.Close()
		for _, id := range extra {
			_, _ = store.Exec(c, `DELETE FROM vendors WHERE id=$1`, id)
		}
	})
}

func testApp(store *Store) *App {
	return &App{
		Store:     store,
		JWTSecret: []byte("test-secret"),
		Mail:      testMail,
		WeChat:    passthroughWeChat{},
		Apple:     passthroughApple{},
		KV:        newMemoryKV(),
	}
}
