package tata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PushConfig 个推（GeTui）凭证与开关。凭证未配置或 Enabled=false 时，
// 推送调用为空操作（只记日志），站内信仍照常落库，App 的消息中心不受影响。
type PushConfig struct {
	AppID        string
	AppKey       string
	AppSecret    string
	MasterSecret string
	BaseURL      string
	Enabled      bool
}

// Pusher 封装个推 v2 REST。token 按返回有效期在进程内缓存。
type Pusher struct {
	cfg  PushConfig
	http *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

func NewPusher(cfg PushConfig) *Pusher {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://restapi.getui.com"
	}
	return &Pusher{cfg: cfg, http: &http.Client{Timeout: 10 * time.Second}}
}

// Ready 表示凭证齐全且开关打开，可以真正请求个推。
func (p *Pusher) Ready() bool {
	return p != nil && p.cfg.Enabled &&
		p.cfg.AppID != "" && p.cfg.AppKey != "" && p.cfg.MasterSecret != ""
}

func (p *Pusher) authToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Now().Before(p.tokenExp) {
		return p.token, nil
	}
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	sum := sha256.Sum256([]byte(p.cfg.AppKey + ts + p.cfg.MasterSecret))
	body := map[string]string{
		"sign":      hex.EncodeToString(sum[:]),
		"timestamp": ts,
		"appkey":    p.cfg.AppKey,
	}
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := p.do(ctx, "/v2/"+p.cfg.AppID+"/auth", "", body, &out); err != nil {
		return "", err
	}
	if out.Code != 0 || out.Data.Token == "" {
		return "", fmt.Errorf("getui auth failed: code=%d msg=%s", out.Code, out.Msg)
	}
	p.token = out.Data.Token
	p.tokenExp = time.Now().Add(23 * time.Hour)
	return p.token, nil
}

func (p *Pusher) do(ctx context.Context, path, token string, in, out any) error {
	buf, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json;charset=utf-8")
	if token != "" {
		req.Header.Set("token", token)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("getui decode: %w (body=%s)", err, string(data))
		}
	}
	return nil
}

func pushMessage(title, body, target string) map[string]any {
	n := map[string]any{"title": title, "body": body}
	if target != "" {
		n["click_type"] = "url"
		n["url"] = target
	} else {
		n["click_type"] = "startapp"
	}
	return map[string]any{"notification": n}
}

// Broadcast 走个推 /push/all 做真·全量下发，不看用户的消息通知偏好。
// 仅在管理端明确要求 to_app 时使用；默认群发走 ToCIDs 以尊重 messages_enabled。
func (p *Pusher) Broadcast(ctx context.Context, title, body, target string) error {
	if !p.Ready() {
		log.Printf("push: skip broadcast (disabled/unconfigured): %q", title)
		return nil
	}
	token, err := p.authToken(ctx)
	if err != nil {
		return err
	}
	reqID, _ := randomID()
	payload := map[string]any{
		"request_id":   reqID,
		"group_name":   "campaign",
		"settings":     map[string]any{"ttl": 3600000},
		"audience":     "all",
		"push_message": pushMessage(title, body, target),
	}
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := p.do(ctx, "/v2/"+p.cfg.AppID+"/push/all", token, payload, &out); err != nil {
		return err
	}
	if out.Code != 0 {
		return fmt.Errorf("getui push/all code=%d msg=%s", out.Code, out.Msg)
	}
	return nil
}

// ToCIDs 按设备 CID 定向下发。调用方负责只传入已开启消息通知的 CID。
func (p *Pusher) ToCIDs(ctx context.Context, cids []string, title, body, target string) error {
	if !p.Ready() {
		log.Printf("push: skip targeted push (disabled/unconfigured): %q (%d cids)", title, len(cids))
		return nil
	}
	if len(cids) == 0 {
		return nil
	}
	token, err := p.authToken(ctx)
	if err != nil {
		return err
	}
	msg := pushMessage(title, body, target)
	for _, cid := range cids {
		reqID, _ := randomID()
		payload := map[string]any{
			"request_id":   reqID,
			"settings":     map[string]any{"ttl": 3600000},
			"audience":     map[string]any{"cid": []string{cid}},
			"push_message": msg,
		}
		var out struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := p.do(ctx, "/v2/"+p.cfg.AppID+"/push/single/cid", token, payload, &out); err != nil {
			return err
		}
		if out.Code != 0 {
			return fmt.Errorf("getui push/single/cid code=%d msg=%s", out.Code, out.Msg)
		}
	}
	return nil
}

// ---- App 端：设备注册与站内信 ----

func (a *App) appRegisterPush(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能注册推送")
	}
	var req struct {
		CID      string `json:"cid"`
		Platform string `json:"platform"`
		Enabled  *bool  `json:"enabled"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	cid := strings.TrimSpace(req.CID)
	if cid == "" {
		return biz("缺少推送 CID")
	}
	platform := strings.TrimSpace(req.Platform)
	if platform != "" && platform != "ios" && platform != "android" {
		return biz("平台不正确")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if _, err := a.Store.Exec(r.Context(), `INSERT INTO push_devices(user_id, cid, platform, enabled, updated_at)
		VALUES ($1,$2,$3,$4,now())
		ON CONFLICT (user_id, cid) DO UPDATE SET platform=EXCLUDED.platform, enabled=EXCLUDED.enabled, updated_at=now()`,
		uid, cid, platform, enabled); err != nil {
		return err
	}
	writeOK(w, map[string]any{"cid": cid, "platform": platform, "enabled": enabled})
	return nil
}

func (a *App) appListInbox(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能查看消息")
	}
	rows, err := a.Store.Query(r.Context(), `
		SELECT m.id, m.category, m.title, m.body, m.target, m.created_at,
		       (rd.user_id IS NOT NULL) AS read
		FROM app_messages m
		LEFT JOIN app_message_reads rd ON rd.message_id = m.id AND rd.user_id = $1
		WHERE m.audience = 'all' OR m.user_id = $1
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT 100`, uid)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	unread := 0
	for rows.Next() {
		var id int64
		var category, title, body, target string
		var createdAt time.Time
		var read bool
		if err := rows.Scan(&id, &category, &title, &body, &target, &createdAt, &read); err != nil {
			return err
		}
		if !read {
			unread++
		}
		items = append(items, map[string]any{
			"id": id, "category": category, "title": title, "body": body,
			"target": target, "created_at": createdAt, "read": read,
		})
	}
	writeOK(w, map[string]any{"items": items, "unread_count": unread})
	return nil
}

func (a *App) appReadMessage(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能操作消息")
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var exists bool
	if err := a.Store.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM app_messages WHERE id=$1 AND (audience='all' OR user_id=$2))`,
		id, uid).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return biz("消息不存在")
	}
	if _, err := a.Store.Exec(r.Context(),
		`INSERT INTO app_message_reads(message_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
		id, uid); err != nil {
		return err
	}
	writeOK(w, true)
	return nil
}

// ---- 下发：落站内信 + 推送 ----

type broadcastIn struct {
	audience string // all | user
	userID   int64
	category string
	title    string
	body     string
	target   string
	cids     []string // 显式定向 CID；为空时按 audience 聚合已开启消息通知的设备
	toApp    bool     // true=真·全量 /push/all（忽略偏好）；否则按 messages_enabled 定向
}

// optInCIDs 返回已启用且用户开启了消息通知的设备 CID。userID=0 表示全部用户。
func (a *App) optInCIDs(ctx context.Context, userID int64) ([]string, error) {
	rows, err := a.Store.Query(ctx, `
		SELECT d.cid FROM push_devices d
		LEFT JOIN notification_preferences np ON np.user_id = d.user_id
		WHERE d.enabled = true AND COALESCE(np.messages_enabled, true) = true
		  AND ($1 = 0 OR d.user_id = $1)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cids []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			return nil, err
		}
		cids = append(cids, cid)
	}
	return cids, nil
}

// deliverMessage 把一条消息写入站内信，并按配置下发系统推送。返回站内信 id。
// 推送失败只记日志，不影响站内信落库。
func (a *App) deliverMessage(ctx context.Context, in broadcastIn) (int64, error) {
	audience := in.audience
	if audience == "" {
		audience = "all"
	}
	category := in.category
	if category == "" {
		category = "campaign"
	}
	var userArg any
	if audience == "user" {
		userArg = in.userID
	}
	var msgID int64
	if err := a.Store.QueryRow(ctx, `INSERT INTO app_messages(audience, user_id, category, title, body, target)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		audience, userArg, category, in.title, in.body, in.target).Scan(&msgID); err != nil {
		return 0, err
	}
	if a.Push == nil {
		return msgID, nil
	}
	if in.toApp && audience == "all" {
		if err := a.Push.Broadcast(ctx, in.title, in.body, in.target); err != nil {
			log.Printf("push broadcast(all) failed: %v", err)
		}
		return msgID, nil
	}
	cids := in.cids
	if len(cids) == 0 {
		var target int64
		if audience == "user" {
			target = in.userID
		}
		c, err := a.optInCIDs(ctx, target)
		if err != nil {
			log.Printf("push optInCIDs failed: %v", err)
		}
		cids = c
	}
	if len(cids) > 0 {
		if err := a.Push.ToCIDs(ctx, cids, in.title, in.body, in.target); err != nil {
			log.Printf("push to cids failed: %v", err)
		}
	}
	return msgID, nil
}

// ---- 管理端：手动群发 ----

func (a *App) adminBroadcastPush(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	var req struct {
		Title    string   `json:"title"`
		Body     string   `json:"body"`
		Target   string   `json:"target"`
		Audience string   `json:"audience"`
		UserID   int64    `json:"user_id"`
		CIDs     []string `json:"cids"`
		Category string   `json:"category"`
		ToApp    bool     `json:"to_app"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return biz("缺少标题")
	}
	audience := req.Audience
	if audience == "" {
		audience = "all"
	}
	if audience != "all" && audience != "user" {
		return biz("audience 不正确")
	}
	if audience == "user" && req.UserID <= 0 {
		return biz("缺少 user_id")
	}
	id, err := a.deliverMessage(r.Context(), broadcastIn{
		audience: audience, userID: req.UserID, category: req.Category,
		title: title, body: req.Body, target: req.Target, cids: req.CIDs, toApp: req.ToApp,
	})
	if err != nil {
		return err
	}
	a.Store.audit(r.Context(), act.id, "broadcast", "push", fmtID(id), title)
	writeOK(w, map[string]any{"id": id, "push_enabled": a.Push.Ready()})
	return nil
}
