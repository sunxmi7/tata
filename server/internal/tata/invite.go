package tata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
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
	var publicID string
	if err := a.Store.QueryRow(r.Context(), `SELECT COALESCE(public_id,'') FROM app_users WHERE id=$1`, uid).Scan(&publicID); err != nil {
		return err
	}
	publicID, err = a.ensurePublicID(r.Context(), uid, publicID)
	if err != nil {
		return err
	}
	var invited int
	if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM invite_rewards WHERE inviter_id=$1`, uid).Scan(&invited); err != nil {
		return err
	}
	var remaining int
	_ = a.Store.QueryRow(r.Context(), `SELECT invite_remaining FROM rewards WHERE user_id=$1`, uid).Scan(&remaining)
	writeOK(w, map[string]any{
		"code": publicID, "reward_amount": inviteRewardAmount, "reward_unit": "count",
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
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := a.applyInvite(r.Context(), uid, req.Code); err != nil {
		return err
	}
	writeOK(w, true)
	return nil
}

func (a *App) applyInvite(ctx context.Context, inviteeID int64, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return biz("邀请码不正确")
	}
	var inviterID int64
	err := a.Store.QueryRow(ctx, `SELECT id FROM app_users WHERE public_id=$1 AND merged_into IS NULL AND cancelled_at IS NULL`, code).Scan(&inviterID)
	if errors.Is(err, sql.ErrNoRows) {
		return biz("邀请码不正确")
	}
	if err != nil {
		return err
	}
	if inviterID == inviteeID {
		return biz("不能填写自己的邀请码")
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
	if _, err := tx.Exec(ctx, `INSERT INTO invite_rewards(inviter_id, invitee_id, invite_code, invitee_name, invitee_avatar, amount)
		VALUES ($1,$2,$3,$4,$5,$6)`, inviterID, inviteeID, code, strings.TrimSpace(name), strings.TrimSpace(avatar), inviteRewardAmount); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return biz("已经绑定过邀请人")
		}
		return err
	}
	if err := creditInviteRemaining(ctx, tx, inviterID, inviteRewardAmount); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *App) tryApplyInvite(ctx context.Context, inviteeID int64, code string) {
	code = strings.TrimSpace(code)
	if inviteeID == 0 || code == "" {
		return
	}
	_ = a.applyInvite(ctx, inviteeID, code)
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
