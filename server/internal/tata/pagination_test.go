package tata

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestReadingsPagination(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://tata:tata@127.0.0.1:54329/tata?sslmode=disable"
	}
	store, err := Open(t.Context(), dsn, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	protectLiveAIConfig(t, store)
	app := testApp(store)
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	token, _ := registerUser(t, ts, store, "page")
	for i := 0; i < 3; i++ {
		create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": fmt.Sprintf("问题%d", i), "type": "normal"})
	}

	// 第一页 limit=2
	p1 := listOne(t, ts, token, "/api/app/v1/readings?limit=2")
	items := p1["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("page1 size: %d", len(items))
	}
	if p1["has_more"] != true {
		t.Fatalf("page1 has_more: %#v", p1["has_more"])
	}
	cursor := int64(p1["next_cursor"].(float64))
	if cursor <= 0 {
		t.Fatalf("page1 cursor: %v", cursor)
	}
	// id 降序：第一页两条 id 都应大于游标
	for _, it := range items {
		if int64(it.(map[string]any)["id"].(float64)) < cursor {
			t.Fatalf("page1 item below cursor")
		}
	}

	// 第二页
	p2 := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings?limit=2&before_id=%d", cursor))
	items2 := p2["items"].([]any)
	if len(items2) < 1 {
		t.Fatalf("page2 empty")
	}
	for _, it := range items2 {
		if int64(it.(map[string]any)["id"].(float64)) >= cursor {
			t.Fatalf("page2 item not older than cursor")
		}
	}

	// 类型筛选：没有 tarotist 记录
	pf := listOne(t, ts, token, "/api/app/v1/readings?type=tarotist")
	if len(pf["items"].([]any)) != 0 {
		t.Fatalf("type filter leaked: %#v", pf["items"])
	}

	// 非法 type
	if msg, _ := call(t, ts, token, http.MethodGet, "/api/app/v1/readings?type=bogus", nil); msg != "type 不正确" {
		t.Fatalf("bad type: %s", msg)
	}
	// 非法 before_id
	if msg, _ := call(t, ts, token, http.MethodGet, "/api/app/v1/readings?before_id=-1", nil); msg != "before_id 不正确" {
		t.Fatalf("bad before_id: %s", msg)
	}
	// limit 超过上限被截断（不报错）
	big := listOne(t, ts, token, "/api/app/v1/readings?limit=9999")
	if int(big["limit"].(float64)) != 100 {
		t.Fatalf("limit clamp: %#v", big["limit"])
	}
}
