package tata

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type jobEvent struct {
	Event   string
	Text    string
	Payload any
}

type jobHub struct {
	mu   sync.Mutex
	runs map[string]*jobRun
}

type jobRun struct {
	key   string
	jobID int64

	worker  int32
	mu      sync.Mutex
	status  string
	partial string
	err     error
	payload any
	subs    []chan jobEvent
	done    chan struct{}
}

func (a *App) jobs() *jobHub {
	a.jobsOnce.Do(func() {
		a.jobsHub = &jobHub{runs: map[string]*jobRun{}}
	})
	return a.jobsHub
}

func (h *jobHub) ensure(key string) *jobRun {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r := h.runs[key]; r != nil {
		return r
	}
	r := &jobRun{key: key, status: "running", done: make(chan struct{})}
	h.runs[key] = r
	return r
}

func (h *jobHub) liveOrNew(key string) (*jobRun, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r := h.runs[key]; r != nil {
		r.mu.Lock()
		live := r.status == "running"
		r.mu.Unlock()
		if live {
			return r, false
		}
	}
	r := &jobRun{key: key, status: "running", done: make(chan struct{})}
	h.runs[key] = r
	return r, true
}

func (h *jobHub) forgetLater(key string, run *jobRun) {
	go func() {
		<-run.done
		time.Sleep(2 * time.Minute)
		h.mu.Lock()
		if h.runs[key] == run {
			delete(h.runs, key)
		}
		h.mu.Unlock()
	}()
}

func (r *jobRun) tryBecomeWorker() bool {
	return atomic.CompareAndSwapInt32(&r.worker, 0, 1)
}

func (r *jobRun) running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status == "running"
}

func (r *jobRun) wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.err
	}
}

func (r *jobRun) appendDelta(chunk string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.partial += chunk
	text := chunk
	for _, ch := range r.subs {
		select {
		case ch <- jobEvent{Event: "delta", Text: text}:
		default:
		}
	}
	return r.partial
}

func (r *jobRun) snapshot() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.partial
}

func (r *jobRun) result() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.payload
}

func (r *jobRun) finish(err error, payload any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.status != "running" {
		return
	}
	r.payload = payload
	r.err = err
	ev := jobEvent{Event: "done", Payload: payload}
	if err != nil {
		r.status = "failed"
		ev = jobEvent{Event: "error", Text: err.Error()}
	} else {
		r.status = "succeeded"
	}
	for _, ch := range r.subs {
		select {
		case ch <- ev:
		default:
		}
		close(ch)
	}
	r.subs = nil
	close(r.done)
}

func (r *jobRun) subscribe() (<-chan jobEvent, func()) {
	ch := make(chan jobEvent, 128)
	r.mu.Lock()
	if r.partial != "" {
		ch <- jobEvent{Event: "delta", Text: r.partial}
	}
	if r.status != "running" {
		if r.status == "succeeded" {
			ch <- jobEvent{Event: "done", Payload: r.payload}
		} else {
			msg := "生成失败，可重试，未重新扣费"
			if r.err != nil {
				msg = r.err.Error()
			}
			ch <- jobEvent{Event: "error", Text: msg}
		}
		close(ch)
		r.mu.Unlock()
		return ch, func() {}
	}
	r.subs = append(r.subs, ch)
	r.mu.Unlock()
	return ch, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		out := r.subs[:0]
		for _, s := range r.subs {
			if s != ch {
				out = append(out, s)
			}
		}
		r.subs = out
	}
}

func finishedRun(err error, payload any) *jobRun {
	r := &jobRun{status: "succeeded", payload: payload, done: make(chan struct{})}
	if err != nil {
		r.status = "failed"
		r.err = err
	}
	close(r.done)
	return r
}

func (a *App) insertAIJob(ctx context.Context, kind string, readingID int64, userID int64, year, turn int) int64 {
	var id int64
	_ = a.Store.QueryRow(ctx, `INSERT INTO ai_jobs(kind, reading_id, user_id, yearly_year, followup_turn, status, started_at)
		VALUES ($1,$2,$3,$4,$5,'running',now()) RETURNING id`,
		kind, nullIfZero(readingID), nullIfZero(userID), nullIfZeroInt(year), nullIfZeroInt(turn)).Scan(&id)
	return id
}

func (a *App) persistJobPartial(ctx context.Context, jobID int64, text string) {
	if jobID == 0 {
		return
	}
	_, _ = a.Store.Exec(ctx, `UPDATE ai_jobs SET partial_text=$1, updated_at=now() WHERE id=$2`, text, jobID)
}

func (a *App) persistJobDone(ctx context.Context, jobID int64, err error, text string) {
	if jobID == 0 {
		return
	}
	if err != nil {
		_, _ = a.Store.Exec(ctx, `UPDATE ai_jobs SET status='failed', error_text=$1, result_text=$2, finished_at=now(), updated_at=now() WHERE id=$3`,
			err.Error(), text, jobID)
		return
	}
	_, _ = a.Store.Exec(ctx, `UPDATE ai_jobs SET status='succeeded', result_text=$1, error_text=NULL, finished_at=now(), updated_at=now() WHERE id=$2`,
		text, jobID)
}

func nullIfZero(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullIfZeroInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func (a *App) attachJobSSE(ctx context.Context, send func(string, any), run *jobRun, onDone func() error) {
	ch, unsub := run.subscribe()
	defer unsub()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			switch ev.Event {
			case "delta":
				send("delta", map[string]any{"text": ev.Text})
			case "done":
				if onDone != nil {
					if err := onDone(); err != nil {
						send("error", map[string]any{"message": T(ctx, err.Error())})
					}
					return
				}
				send("done", ev.Payload)
				return
			case "error":
				send("error", map[string]any{"message": T(ctx, ev.Text)})
				return
			}
		}
	}
}

func (a *App) ensureInterpretJob(ctx context.Context, id int64, adminRetry bool) (*jobRun, error) {
	var status, kind string
	var userID *int64
	var cards, question *string
	var quotaConsumed, first, rewardConsumed bool
	err := a.Store.QueryRow(ctx, `SELECT status, type, user_id, cards, question, quota_consumed, first_experience, reward_consumed FROM readings WHERE id=$1 AND deleted_at IS NULL`, id).
		Scan(&status, &kind, &userID, &cards, &question, &quotaConsumed, &first, &rewardConsumed)
	if err != nil {
		return nil, biz("记录不存在")
	}
	_ = first
	if status == "COMPLETED" && !adminRetry {
		return finishedRun(nil, nil), nil
	}
	if adminRetry && status != "FAILED_RETRYABLE" && status != "INTERPRETING" {
		return nil, biz("只有失败可重试的记录能重试")
	}
	if !adminRetry && status != "CARDS_FIXED" && status != "FAILED_RETRYABLE" && status != "INTERPRETING" {
		return nil, biz("当前状态不能生成解读")
	}
	key := "interpret:" + fmtID(id)
	run, isNew := a.jobs().liveOrNew(key)
	if !isNew {
		return run, nil
	}
	if !run.tryBecomeWorker() {
		return run, nil
	}
	a.jobs().forgetLater(key, run)
	go a.executeInterpret(id)
	return run, nil
}

func (a *App) executeInterpret(id int64) {
	key := "interpret:" + fmtID(id)
	run := a.jobs().ensure(key)
	bg := context.Background()
	fail := func(err error) {
		a.persistJobDone(bg, run.jobID, err, run.snapshot())
		run.finish(err, nil)
	}

	var status, kind string
	var userID *int64
	var cards, question *string
	var quotaConsumed, rewardConsumed bool
	err := a.Store.QueryRow(bg, `SELECT status, type, user_id, cards, question, quota_consumed, reward_consumed FROM readings WHERE id=$1 AND deleted_at IS NULL`, id).
		Scan(&status, &kind, &userID, &cards, &question, &quotaConsumed, &rewardConsumed)
	if err != nil {
		fail(biz("记录不存在"))
		return
	}
	if userID == nil {
		fail(biz("获取答案前必须登录"))
		return
	}
	var settled bool
	if err := a.Store.QueryRow(bg, `SELECT quota_settled FROM readings WHERE id=$1`, id).Scan(&settled); err != nil {
		fail(err)
		return
	}
	if !quotaConsumed && !rewardConsumed && !settled {
		fail(biz("额度未扣减，记录已保留，不继续生成"))
		return
	}
	if cards == nil || *cards == "" {
		fail(biz("牌面尚未固定"))
		return
	}
	vendorName, model, _, _, _, err := a.binding(bg, taskFor(kind))
	if err != nil {
		_, _ = a.Store.Exec(bg, `UPDATE readings SET status='FAILED_RETRYABLE', fail_reason=$1, updated_at=now() WHERE id=$2 AND status IN ('CARDS_FIXED','FAILED_RETRYABLE','INTERPRETING')`, "未绑定供应商", id)
		fail(err)
		return
	}
	tag, err := a.Store.Exec(bg, `UPDATE readings SET status='INTERPRETING', vendor_name=$1, model_name=$2, fail_reason=NULL, updated_at=now() WHERE id=$3 AND status IN ('CARDS_FIXED','FAILED_RETRYABLE','INTERPRETING')`,
		vendorName, model, id)
	if err != nil {
		fail(err)
		return
	}
	if tag.RowsAffected() == 0 {
		item, loadErr := a.loadReading(bg, id)
		if loadErr == nil {
			if st, _ := item["status"].(string); st == "COMPLETED" {
				run.finish(nil, item)
				return
			}
		}
		fail(biz("生成进行中"))
		return
	}
	run.jobID = a.insertAIJob(bg, "interpret", id, *userID, 0, 0)

	prompt := buildPrompt(kind, deref(question), *cards)
	if kind == "tarotist" {
		var tid *int64
		_ = a.Store.QueryRow(bg, `SELECT tarotist_id FROM readings WHERE id=$1`, id).Scan(&tid)
		if tid != nil {
			var name, persona, voice, banned, ask string
			_ = a.Store.QueryRow(bg, `SELECT name, persona, voice, banned, ask_style FROM tarotists WHERE id=$1`, *tid).Scan(&name, &persona, &voice, &banned, &ask)
			prompt = "共用解牌：先按牌面、牌位、正逆位解释，再按人设说话。不得改牌。\n人设：" + name + "。" + persona + "\n语音：" + voice + "。禁止：" + banned + "。追问：" + ask + "\n" + buildPrompt(kind, deref(question), *cards)
		}
	}

	spec, specErr := a.modelSpec(bg, taskFor(kind))
	wait := 120 * time.Second
	if specErr == nil && spec.Timeout > 0 {
		wait = spec.Timeout + 15*time.Second
	}
	jobCtx, cancel := context.WithTimeout(bg, wait)
	defer cancel()

	lastWrite := time.Now().Add(-time.Second)
	_, _, text, callErr := a.runEinoStream(jobCtx, taskFor(kind), prompt, func(chunk string) error {
		partial := run.appendDelta(chunk)
		if time.Since(lastWrite) >= 200*time.Millisecond {
			lastWrite = time.Now()
			a.persistJobPartial(bg, run.jobID, partial)
			_, _ = a.Store.Exec(bg, `UPDATE readings SET interpretation=$1, updated_at=now() WHERE id=$2 AND status='INTERPRETING'`, partial, id)
		}
		return nil
	})
	if callErr != nil {
		_, _ = a.Store.Exec(bg, `UPDATE readings SET status='FAILED_RETRYABLE', fail_reason=$1, updated_at=now() WHERE id=$2`, callErr.Error(), id)
		fail(biz("解读失败，可重试，未重新扣费"))
		return
	}
	if _, err := a.Store.Exec(bg, `UPDATE readings SET status='COMPLETED', interpretation=$1, fail_reason=NULL, updated_at=now() WHERE id=$2`, text, id); err != nil {
		fail(err)
		return
	}
	item, err := a.loadReading(bg, id)
	if err != nil {
		fail(err)
		return
	}
	a.persistJobDone(bg, run.jobID, nil, text)
	run.finish(nil, item)
}

func (a *App) ensureDetectJob(ctx context.Context, id int64, question string) (*jobRun, error) {
	key := "detect:" + fmtID(id)
	run, isNew := a.jobs().liveOrNew(key)
	if !isNew {
		return run, nil
	}
	if !run.tryBecomeWorker() {
		return run, nil
	}
	a.jobs().forgetLater(key, run)
	go a.executeDetect(id, question)
	return run, nil
}

func (a *App) executeDetect(id int64, question string) {
	key := "detect:" + fmtID(id)
	run := a.jobs().ensure(key)
	bg := context.Background()
	run.jobID = a.insertAIJob(bg, "detect", id, 0, 0, 0)
	lastWrite := time.Now().Add(-time.Second)
	vendor, model, output, callErr := a.runEinoStream(bg, "detect", question, func(chunk string) error {
		partial := run.appendDelta(chunk)
		if time.Since(lastWrite) >= 200*time.Millisecond {
			lastWrite = time.Now()
			a.persistJobPartial(bg, run.jobID, partial)
		}
		return nil
	})
	if callErr != nil {
		a.persistJobDone(bg, run.jobID, callErr, run.snapshot())
		run.finish(callErr, nil)
		return
	}
	_, _ = a.Store.Exec(bg, `UPDATE readings SET skip_detect=false, updated_at=now() WHERE id=$1`, id)
	payload := map[string]any{"text": output, "vendor": vendor, "model": model, "suggestions": 3}
	a.persistJobDone(bg, run.jobID, nil, output)
	run.finish(nil, payload)
}

func (a *App) ensureFollowupJob(ctx context.Context, readingID, uid, turn int64, text string) (*jobRun, error) {
	var existing int
	_ = a.Store.QueryRow(ctx, `SELECT count(*) FROM reading_messages WHERE reading_id=$1 AND turn_id=$2 AND role='assistant'`, readingID, turn).Scan(&existing)
	if existing > 0 {
		body, err := a.answerFollowup(ctx, readingID, uid, turn, text)
		if err != nil {
			return nil, err
		}
		return finishedRun(nil, body), nil
	}
	key := fmt.Sprintf("followup:%d:%d", readingID, turn)
	run, isNew := a.jobs().liveOrNew(key)
	if !isNew {
		return run, nil
	}
	if !run.tryBecomeWorker() {
		return run, nil
	}
	a.jobs().forgetLater(key, run)
	go a.executeFollowup(readingID, uid, turn, text)
	return run, nil
}

func (a *App) executeFollowup(readingID, uid, turn int64, text string) {
	key := fmt.Sprintf("followup:%d:%d", readingID, turn)
	run := a.jobs().ensure(key)
	bg := context.Background()
	run.jobID = a.insertAIJob(bg, "followup", readingID, uid, 0, int(turn))
	lastWrite := time.Now().Add(-time.Second)
	body, err := a.answerFollowupStreaming(bg, readingID, uid, turn, text, func(chunk string) error {
		partial := run.appendDelta(chunk)
		if time.Since(lastWrite) >= 200*time.Millisecond {
			lastWrite = time.Now()
			a.persistJobPartial(bg, run.jobID, partial)
		}
		return nil
	})
	if err != nil {
		a.persistJobDone(bg, run.jobID, err, run.snapshot())
		run.finish(err, nil)
		return
	}
	if msg, _ := body["error"].(string); msg != "" {
		a.persistJobDone(bg, run.jobID, fmt.Errorf("%s", msg), run.snapshot())
		run.finish(nil, body)
		return
	}
	result, _ := body["bubbles"].([]string)
	a.persistJobDone(bg, run.jobID, nil, strings.Join(result, "\n\n"))
	run.finish(nil, body)
}

func (a *App) ensureYearlyJob(ctx context.Context, uid int64, year int, row yearlyRow) (*jobRun, error) {
	if row.reportStatus == "ready" && strings.TrimSpace(row.report) != "" {
		return finishedRun(nil, nil), nil
	}
	if _, _, _, _, _, err := a.binding(ctx, "interpret"); err != nil {
		_, _ = a.Store.Exec(ctx, `UPDATE yearly_reports SET report_status='failed', fail_reason=$1 WHERE user_id=$2 AND year=$3`, "未绑定供应商", uid, year)
		return nil, err
	}
	key := fmt.Sprintf("yearly:%d:%d", uid, year)
	run, isNew := a.jobs().liveOrNew(key)
	if !isNew {
		return run, nil
	}
	if !run.tryBecomeWorker() {
		return run, nil
	}
	a.jobs().forgetLater(key, run)
	go a.executeYearly(uid, year, row)
	return run, nil
}

func (a *App) executeYearly(uid int64, year int, row yearlyRow) {
	key := fmt.Sprintf("yearly:%d:%d", uid, year)
	run := a.jobs().ensure(key)
	bg := context.Background()
	fail := func(err error) {
		_, _ = a.Store.Exec(bg, `UPDATE yearly_reports SET report_status='failed', fail_reason=$1 WHERE user_id=$2 AND year=$3`, err.Error(), uid, year)
		a.persistJobDone(bg, run.jobID, err, run.snapshot())
		run.finish(biz("解读失败，可重试"), nil)
	}
	tag, err := a.Store.Exec(bg, `UPDATE yearly_reports SET report_status='generating', fail_reason=NULL
		WHERE user_id=$1 AND year=$2 AND report_status IN ('pending','failed','generating')`, uid, year)
	if err != nil {
		fail(err)
		return
	}
	if tag.RowsAffected() == 0 {
		run.finish(nil, nil)
		return
	}
	run.jobID = a.insertAIJob(bg, "yearly", 0, uid, year, 0)
	prompt := buildYearlyPrompt(year, row.birthDate, row.gender, row.rhythmJSON, row.domainJSON)
	lastWrite := time.Now().Add(-time.Second)
	_, _, text, callErr := a.runEinoStream(bg, "interpret", prompt, func(chunk string) error {
		partial := run.appendDelta(chunk)
		if time.Since(lastWrite) >= 200*time.Millisecond {
			lastWrite = time.Now()
			a.persistJobPartial(bg, run.jobID, partial)
			_, _ = a.Store.Exec(bg, `UPDATE yearly_reports SET report=$1 WHERE user_id=$2 AND year=$3 AND report_status='generating'`, partial, uid, year)
		}
		return nil
	})
	if callErr != nil {
		fail(callErr)
		return
	}
	if _, err := a.Store.Exec(bg, `UPDATE yearly_reports SET report_status='ready', report=$1, fail_reason=NULL, generated_at=now()
		WHERE user_id=$2 AND year=$3`, text, uid, year); err != nil {
		fail(err)
		return
	}
	a.persistJobDone(bg, run.jobID, nil, text)
	run.finish(nil, nil)
}

func (a *App) streamInterpretJob(w http.ResponseWriter, r *http.Request, id int64, wrap func(map[string]any) map[string]any) error {
	run, err := a.ensureInterpretJob(r.Context(), id, false)
	if err != nil {
		return err
	}
	send := sseStart(w)
	a.attachJobSSE(r.Context(), send, run, func() error {
		item, err := a.loadReadingAbs(r.Context(), r, id)
		if err != nil {
			return err
		}
		if wrap != nil {
			item = wrap(item)
		}
		send("done", map[string]any{"reading": item})
		return nil
	})
	return nil
}
