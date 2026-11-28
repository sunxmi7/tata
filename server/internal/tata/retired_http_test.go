package tata

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// 这些路径已从服务端拿掉（自定义牌组、旧 reading 扣费步等）。
// 测试仍直接调用原来的处理函数，避免把既有校验一起拆掉。
// App IAP（entitlements/skus/verify/restore/config）已重新挂回 mux，不再走这里。
var liveTestApps sync.Map

func rememberTestApp(url string, app *App, t *testing.T) {
	t.Helper()
	liveTestApps.Store(url, app)
	t.Cleanup(func() { liveTestApps.Delete(url) })
}

var (
	retiredReading = regexp.MustCompile(`^/api/app/v1/readings/(\d+)/(question|confirm-quota|use-reward|paywall)$`)
	retiredCustom  = regexp.MustCompile(`^/api/app/v1/custom-decks/(\d+)(/use)?$`)
)

func dispatchRetired(app *App, method, rawPath string, body []byte, token string) (int, []byte, bool) {
	path := rawPath
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	var fn func(http.ResponseWriter, *http.Request) error
	vars := map[string]string{}
	switch {
	case method == http.MethodGet && path == "/api/app/v1/wallet/ledger":
		fn = app.appWalletLedger
	case method == http.MethodGet && path == "/api/app/v1/custom-decks":
		fn = app.appListCustomDecks
	case method == http.MethodPost && path == "/api/app/v1/custom-decks":
		fn = app.appCreateCustomDeck
	default:
		if m := retiredReading.FindStringSubmatch(path); m != nil {
			vars["id"] = m[1]
			switch m[2] {
			case "question":
				if method == http.MethodPut {
					fn = app.appEditQuestion
				}
			case "confirm-quota":
				if method == http.MethodPost {
					fn = app.appConfirmQuota
				}
			case "use-reward":
				if method == http.MethodPost {
					fn = app.appUseReward
				}
			case "paywall":
				if method == http.MethodPost {
					fn = app.appCancelPaywall
				}
			}
		}
		if fn == nil {
			if m := retiredCustom.FindStringSubmatch(path); m != nil {
				vars["id"] = m[1]
				switch {
				case m[2] == "/use" && method == http.MethodPost:
					fn = app.appUseCustomDeck
				case m[2] == "" && method == http.MethodGet:
					fn = app.appGetCustomDeck
				case m[2] == "" && method == http.MethodPut:
					fn = app.appPutCustomDeck
				}
			}
		}
	}
	if fn == nil {
		return 0, nil, false
	}
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, "http://app.test"+rawPath, rdr)
	for k, v := range vars {
		req.SetPathValue(k, v)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	app.h(fn)(rec, req)
	return rec.Code, rec.Body.Bytes(), true
}
