package tata

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// sseStart 切换响应为 SSE 并返回一个发送函数。每次发送后立即 flush。
// 调用前不得已向 w 写过普通 JSON（额度拦截等前置判断要先于此函数返回）。
func sseStart(w http.ResponseWriter) func(event string, payload any) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // 防止网关/反代缓冲
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	if flusher == nil {
		// 不应发生：ghttp 的 r.Response.Writer 与 httptest 都实现 Flusher。
		// 若未来被包了一层缓冲 writer，这里会退化成"请求结束才整体下发"，失去流式。
		log.Printf("sse: ResponseWriter is not http.Flusher; streaming degraded to buffered")
	}
	if flusher != nil {
		flusher.Flush()
	}
	return func(event string, payload any) {
		b, _ := json.Marshal(payload)
		if event != "" {
			fmt.Fprintf(w, "event: %s\n", event)
		}
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// appInterpretStream 是 appInterpret 的流式版本：
// 额度门槛与非流式一致（拦截时走普通 JSON 的 QuotaBlock），通过后以 SSE 逐段下发解读。
// 事件：delta（{text}）多次、done（{reading}）或 error（{message}）。
func (a *App) appInterpretStream(w http.ResponseWriter, r *http.Request) error {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.ownReading(r.Context(), id, uid, anon); err != nil {
		return err
	}
	var kind, question string
	if err := a.Store.QueryRow(r.Context(), `SELECT type, question FROM readings WHERE id=$1`, id).Scan(&kind, &question); err != nil {
		return err
	}
	if kind == "explore" && strings.TrimSpace(question) == "" {
		return biz("请先填写问题")
	}
	stop, err := a.gatePlay(w, r, resumeInterpret)
	if err != nil || stop {
		return err
	}
	send := sseStart(w)
	item, genErr := a.generateStream(r.Context(), id, func(chunk string) error {
		send("delta", map[string]any{"text": chunk})
		return r.Context().Err()
	})
	if genErr != nil {
		send("error", map[string]any{"message": genErr.Error()})
		return nil
	}
	send("done", map[string]any{"reading": item})
	return nil
}

// appFollowupStream 是追问的流式版本，仅支持延续追问（continuation=true）。
// 额度/轮次/扣费与非流式一致；拦截时走普通 JSON 的 QuotaBlock，通过后以 SSE 逐段下发。
// 事件：delta（{text}）多次、done（{followup}）或 error（{message}）。
func (a *App) appFollowupStream(w http.ResponseWriter, r *http.Request) error {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.ownReading(r.Context(), id, uid, anon); err != nil {
		return err
	}
	var req struct {
		Text         string `json:"text"`
		Continuation *bool  `json:"continuation"`
	}
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Text) == "" {
		return biz("追问不能为空")
	}
	if req.Continuation == nil || !*req.Continuation {
		return biz("流式仅用于延续追问；新建占卜请用非流式接口")
	}
	var kind, status string
	var cards *string
	if err := a.Store.QueryRow(r.Context(), `SELECT type, status, cards FROM readings WHERE id=$1`, id).Scan(&kind, &status, &cards); err != nil {
		return err
	}
	if kind != "normal" && kind != "explore" && kind != "daily" && kind != "tarotist" {
		return biz("当前记录不能追问")
	}
	if status != "COMPLETED" && status != "FAILED_RETRYABLE" {
		return biz("解读完成前不能追问")
	}
	if cards == nil || *cards == "" {
		return biz("牌面尚未固定")
	}
	if uid == 0 {
		return biz("登录后才会扣次数或钻石")
	}
	text := strings.TrimSpace(req.Text)
	if strings.Contains(req.Text, "别这样说") || strings.Contains(req.Text, "不舒服") {
		_, _ = a.Store.Exec(r.Context(), `UPDATE app_users SET tease_off=true WHERE id=$1`, uid)
	}
	tx, err := a.Store.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var locked int64
	if err := tx.QueryRow(r.Context(), `SELECT id FROM readings WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		return err
	}
	turn, err := nextFollowupTurn(r.Context(), tx, id)
	if err != nil {
		return err
	}
	if err := enforceFollowupLimit(r.Context(), tx, id, turn); err != nil {
		return err
	}
	key := "followup:" + fmtID(turn)
	decision, err := settleSharedQuota(r.Context(), tx, chargeOpts{
		UserID: uid, ReadingID: id, ChargeKey: key, Cycle: a.userCycle(r, uid),
		AllowTrial: true, ResumeAction: resumeFollowup,
	})
	if err != nil {
		return err
	}
	if !decision.Continue {
		_, _ = tx.Exec(r.Context(), `UPDATE quota_paywall SET resume_payload=$1, updated_at=now() WHERE reading_id=$2 AND charge_key=$3`,
			text, id, key)
		if err := tx.Commit(r.Context()); err != nil {
			return err
		}
		writeQuotaBlock(w, decision, id)
		return nil
	}
	if err := ensureUserTurn(r.Context(), tx, id, turn, text); err != nil {
		return err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}
	send := sseStart(w)
	body, genErr := a.answerFollowupStreaming(r.Context(), id, uid, turn, text, func(chunk string) error {
		send("delta", map[string]any{"text": chunk})
		return r.Context().Err()
	})
	if genErr != nil {
		send("error", map[string]any{"message": genErr.Error()})
		return nil
	}
	body["quota_outcome"] = decision.Status
	body["entitlement"] = decision.Entitlement
	body["charged"] = decision.Status == quotaCharged && decision.Entitlement != entTrial
	send("done", map[string]any{"followup": body})
	return nil
}
