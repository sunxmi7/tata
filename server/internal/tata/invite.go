package tata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	inviteRewardAmount = 3
	inviteMaxPeople    = 5
)

func (a *App) appGetInvites(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	var invited int
	if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM invite_rewards WHERE inviter_id=$1`, uid).Scan(&invited); err != nil {
		return err
	}
	remaining, err := inviteBalance(r.Context(), a.Store, uid)
	if err != nil {
		return err
	}
	code := strconv.FormatInt(uid, 10)
	writeOK(w, map[string]any{
		"code": code, "link": "/invite/" + code, "reward_amount": inviteRewardAmount, "reward_unit": "count",
		"invited_count": invited, "max_invites": inviteMaxPeople,
		"invite_remaining": remaining, "can_invite": invited < inviteMaxPeople,
		"amount_label": inviteAmountLabel(r, inviteRewardAmount),
	})
	return nil
}

func (a *App) appAcceptInvite(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	var req struct {
		Code   string `json:"code"`
		Source string `json:"source"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := a.applyInvite(r.Context(), uid, req.Code, normalizeInviteSource(req.Source)); err != nil {
		return err
	}
	writeOK(w, true)
	return nil
}

func normalizeInviteSource(source string) string {
	if strings.TrimSpace(source) == "link" {
		return "link"
	}
	return "code"
}

func (a *App) appOpenInvite(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		DeviceID string `json:"device_id"`
		Code     string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	deviceID := strings.TrimSpace(req.DeviceID)
	if !validDeviceID(deviceID) {
		return biz("缺少设备标识")
	}
	if _, ok := inviteAccountID(req.Code); !ok {
		return biz("邀请码不正确")
	}
	if _, err := a.Store.Exec(r.Context(), `INSERT INTO invite_attributions(device_id, invite_code) VALUES ($1,$2)
		ON CONFLICT (device_id) DO UPDATE SET invite_code=EXCLUDED.invite_code, created_at=now()`, deviceID, strings.TrimSpace(req.Code)); err != nil {
		return err
	}
	writeOK(w, map[string]any{"source": "link"})
	return nil
}

func (a *App) applyDeviceForfeit(ctx context.Context, uid int64, deviceID string) {
	deviceID = strings.TrimSpace(deviceID)
	if uid <= 0 || !validDeviceID(deviceID) {
		return
	}
	var forfeited bool
	if err := a.Store.QueryRow(ctx, `SELECT trial_forfeited FROM device_experiences WHERE device_id=$1`, deviceID).Scan(&forfeited); err != nil || !forfeited {
		return
	}
	_, _ = a.Store.Exec(ctx, `UPDATE app_users SET reading_trial_used=TRUE WHERE id=$1 AND reading_trial_used=FALSE`, uid)
}

func (a *App) bindInvite(ctx context.Context, uid int64, deviceID, code, source string) {
	code = strings.TrimSpace(code)
	source = normalizeInviteSource(source)
	if code == "" {
		pending, ok := a.pendingInviteCode(ctx, deviceID)
		if !ok {
			return
		}
		code = pending
		source = "link"
	}
	_ = a.applyInvite(ctx, uid, code, source)
}

func (a *App) pendingInviteCode(ctx context.Context, deviceID string) (string, bool) {
	deviceID = strings.TrimSpace(deviceID)
	if !validDeviceID(deviceID) {
		return "", false
	}
	var code string
	err := a.Store.QueryRow(ctx, `SELECT invite_code FROM invite_attributions WHERE device_id=$1`, deviceID).Scan(&code)
	if err != nil || strings.TrimSpace(code) == "" {
		return "", false
	}
	return strings.TrimSpace(code), true
}

func (a *App) applyInvite(ctx context.Context, inviteeID int64, code, source string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return biz("邀请码不正确")
	}
	inviterID, ok := inviteAccountID(code)
	if !ok {
		return biz("邀请码不正确")
	}
	err := a.Store.QueryRow(ctx, `SELECT id FROM app_users WHERE id=$1 AND merged_into IS NULL AND cancelled_at IS NULL`, inviterID).Scan(&inviterID)
	if errors.Is(err, sql.ErrNoRows) {
		return biz("邀请码不正确")
	}
	if err != nil {
		return err
	}
	if inviterID == inviteeID {
		return biz("不能填写自己的邀请码")
	}
	blocked, err := rewardBlocked(ctx, a.Store, inviteeID)
	if err != nil {
		return err
	}
	if blocked {
		return biz("该身份已注册过，不能再领邀请奖励")
	}
	var exists int
	if err := a.Store.QueryRow(ctx, `SELECT count(*) FROM invite_rewards WHERE invitee_id=$1`, inviteeID).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return biz("已经绑定过邀请人")
	}
	var name, avatar string
	_ = a.Store.QueryRow(ctx, `SELECT COALESCE(name,''), COALESCE(avatar_url,'') FROM app_users WHERE id=$1`, inviteeID).Scan(&name, &avatar)
	tx, err := a.Store.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM app_users WHERE id=$1 FOR UPDATE`, inviterID); err != nil {
		return err
	}
	var invited int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM invite_rewards WHERE inviter_id=$1`, inviterID).Scan(&invited); err != nil {
		return err
	}
	if invited >= inviteMaxPeople {
		return biz("邀请人数已达上限")
	}
	var relationshipID int64
	err = tx.QueryRow(ctx, `INSERT INTO invite_rewards(inviter_id, invitee_id, invite_code, invitee_name, invitee_avatar, amount, source)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, inviterID, inviteeID, strconv.FormatInt(inviterID, 10), strings.TrimSpace(name), strings.TrimSpace(avatar), inviteRewardAmount, normalizeInviteSource(source)).Scan(&relationshipID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return biz("已经绑定过邀请人")
		}
		return err
	}
	expires := time.Now().AddDate(1, 0, 0)
	for _, side := range []struct {
		id   int64
		role string
	}{{inviterID, "inviter"}, {inviteeID, "invitee"}} {
		if _, err := tx.Exec(ctx, `INSERT INTO invite_grants(user_id, relationship_id, role, total, remaining, expires_at)
			VALUES ($1,$2,$3,$4,$4,$5)`, side.id, relationshipID, side.role, inviteRewardAmount, expires); err != nil {
			return err
		}
		if err := creditInviteRemaining(ctx, tx, side.id, inviteRewardAmount); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func inviteAccountID(code string) (int64, bool) {
	code = strings.TrimSpace(code)
	id, err := strconv.ParseInt(code, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != code {
		return 0, false
	}
	return id, true
}

func (a *App) adminListInvites(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	uid, err := adminUserFilter(r)
	if err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT r.id, r.inviter_id, r.invitee_id, r.invite_code, r.source, r.created_at,
		gi.total, gi.remaining, gi.granted_at, gi.expires_at,
		ge.total, ge.remaining, ge.granted_at, ge.expires_at
		FROM invite_rewards r
		LEFT JOIN invite_grants gi ON gi.relationship_id=r.id AND gi.role='inviter'
		LEFT JOIN invite_grants ge ON ge.relationship_id=r.id AND ge.role='invitee'
		WHERE ($1 = 0 OR r.inviter_id = $1 OR r.invitee_id = $1)
		ORDER BY r.id DESC LIMIT 200`, uid)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, inviterID, inviteeID int64
		var code, source string
		var created time.Time
		var inviterTotal, inviterLeft, inviteeTotal, inviteeLeft sql.NullInt64
		var inviterAt, inviterExp, inviteeAt, inviteeExp sql.NullTime
		if err := rows.Scan(&id, &inviterID, &inviteeID, &code, &source, &created,
			&inviterTotal, &inviterLeft, &inviterAt, &inviterExp,
			&inviteeTotal, &inviteeLeft, &inviteeAt, &inviteeExp); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"id": id, "inviter_id": inviterID, "invitee_id": inviteeID, "invite_code": code, "source": source,
			"source_label": inviteSourceLabel(source), "created_at": created,
			"inviter_total": nullInt(inviterTotal), "inviter_remaining": nullInt(inviterLeft),
			"inviter_granted_at": nullTime(inviterAt), "inviter_expires_at": nullTime(inviterExp),
			"invitee_total": nullInt(inviteeTotal), "invitee_remaining": nullInt(inviteeLeft),
			"invitee_granted_at": nullTime(inviteeAt), "invitee_expires_at": nullTime(inviteeExp),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeOK(w, map[string]any{"items": list})
	return nil
}

func inviteSourceLabel(source string) string {
	if source == "link" {
		return "链接"
	}
	return "邀请码"
}

func inviteFriendName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "新朋友"
	}
	return name
}

func inviteRewardTitle(r *http.Request, name string) string {
	return fmt.Sprintf(T(r.Context(), "好友 %s 通过你的邀请加入了塔塔"), inviteFriendName(name))
}

func inviteAmountLabel(r *http.Request, amount int) string {
	return fmt.Sprintf("+%d %s", amount, T(r.Context(), "次"))
}
