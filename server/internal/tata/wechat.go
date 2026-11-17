package tata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// 微信开放平台「移动应用」换票。一个应用同时填 iOS Bundle ID 和 Android 包名，共用 appId/appSecret。
const (
	wechatTokenURL    = "https://api.weixin.qq.com/sns/oauth2/access_token"
	wechatUserInfoURL = "https://api.weixin.qq.com/sns/userinfo"
	wechatNameMax     = 32
)

type WeChatConfig struct {
	Enabled   bool
	AppID     string
	AppSecret string
}

type wechatProfile struct {
	ID       string
	Nickname string
	Avatar   string
}

func (p wechatProfile) empty() bool {
	return strings.TrimSpace(p.Nickname) == "" && strings.TrimSpace(p.Avatar) == ""
}

type wechatExchanger interface {
	Ready() bool
	ExchangeCode(ctx context.Context, code string) (wechatProfile, error)
}

type WeChatClient struct {
	cfg         WeChatConfig
	http        *http.Client
	tokenURL    string
	userInfoURL string
}

func NewWeChatClient(cfg WeChatConfig) *WeChatClient {
	return &WeChatClient{
		cfg:         cfg,
		http:        &http.Client{Timeout: 10 * time.Second},
		tokenURL:    wechatTokenURL,
		userInfoURL: wechatUserInfoURL,
	}
}

func (c *WeChatClient) Ready() bool {
	if c == nil || !c.cfg.Enabled {
		return false
	}
	return strings.TrimSpace(c.cfg.AppID) != "" && strings.TrimSpace(c.cfg.AppSecret) != ""
}

func (c *WeChatClient) ExchangeCode(ctx context.Context, code string) (wechatProfile, error) {
	if c == nil || !c.Ready() {
		return wechatProfile{}, biz("微信登录未配置")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return wechatProfile{}, biz("微信授权码不能为空")
	}
	u, err := url.Parse(c.tokenURL)
	if err != nil {
		return wechatProfile{}, fmt.Errorf("wechat token url: %w", err)
	}
	q := u.Query()
	q.Set("appid", strings.TrimSpace(c.cfg.AppID))
	q.Set("secret", strings.TrimSpace(c.cfg.AppSecret))
	q.Set("code", code)
	q.Set("grant_type", "authorization_code")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return wechatProfile{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return wechatProfile{}, biz("微信授权失败")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return wechatProfile{}, biz("微信授权失败")
	}
	var out struct {
		AccessToken string `json:"access_token"`
		OpenID      string `json:"openid"`
		UnionID     string `json:"unionid"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return wechatProfile{}, biz("微信授权失败")
	}
	if out.ErrCode != 0 {
		switch out.ErrCode {
		case 40029, 40163:
			return wechatProfile{}, biz("微信授权码无效")
		default:
			return wechatProfile{}, biz("微信授权失败")
		}
	}
	// 同一开放平台账号下 unionid 跨应用稳定；iOS/Android 共用一个移动应用时 openid 也相同。
	id := strings.TrimSpace(out.UnionID)
	if id == "" {
		id = strings.TrimSpace(out.OpenID)
	}
	if id == "" {
		return wechatProfile{}, biz("微信授权失败")
	}
	prof := wechatProfile{ID: id}
	if token := strings.TrimSpace(out.AccessToken); token != "" && strings.TrimSpace(out.OpenID) != "" {
		nick, avatar := c.userInfo(ctx, token, out.OpenID)
		prof.Nickname = nick
		prof.Avatar = avatar
	}
	return prof, nil
}

func (c *WeChatClient) userInfo(ctx context.Context, accessToken, openID string) (nickname, avatar string) {
	infoURL := c.userInfoURL
	if strings.TrimSpace(infoURL) == "" {
		infoURL = wechatUserInfoURL
	}
	u, err := url.Parse(infoURL)
	if err != nil {
		return "", ""
	}
	q := u.Query()
	q.Set("access_token", accessToken)
	q.Set("openid", strings.TrimSpace(openID))
	q.Set("lang", "zh_CN")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", ""
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", ""
	}
	var out struct {
		Nickname   string `json:"nickname"`
		HeadImgURL string `json:"headimgurl"`
		ErrCode    int    `json:"errcode"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.ErrCode != 0 {
		return "", ""
	}
	return strings.TrimSpace(out.Nickname), strings.TrimSpace(out.HeadImgURL)
}

func (a *App) wechatIdentity(ctx context.Context, code string) (wechatProfile, error) {
	if a == nil || a.WeChat == nil || !a.WeChat.Ready() {
		return wechatProfile{}, biz("微信登录未配置")
	}
	return a.WeChat.ExchangeCode(ctx, code)
}

func clipWeChatName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if utf8.RuneCountInString(raw) <= wechatNameMax {
		return raw
	}
	runes := []rune(raw)
	return string(runes[:wechatNameMax])
}

// wechatAvatarTestHook 仅测试注入：允许本机 httptest 头像地址。生产不设置。
var wechatAvatarTestHook func(*url.URL) bool

func wechatAvatarURLOK(raw string) (*url.URL, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, false
	}
	if wechatAvatarTestHook != nil && wechatAvatarTestHook(u) {
		return u, true
	}
	if u.Scheme == "http" {
		u.Scheme = "https"
	}
	if u.Scheme != "https" {
		return nil, false
	}
	host := strings.ToLower(u.Hostname())
	if host == "thirdwx.qlogo.cn" || host == "wx.qlogo.cn" || strings.HasSuffix(host, ".qlogo.cn") {
		return u, true
	}
	return u, false
}

func imageExtFrom(contentType string, data []byte) (string, string) {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch ct {
	case "image/jpeg":
		return ".jpg", "image/jpeg"
	case "image/png":
		return ".png", "image/png"
	case "image/webp":
		return ".webp", "image/webp"
	case "image/gif":
		return ".gif", "image/gif"
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return ".jpg", "image/jpeg"
	}
	if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4e, 0x47}) {
		return ".png", "image/png"
	}
	if bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")) {
		return ".gif", "image/gif"
	}
	if bytes.HasPrefix(data, []byte("RIFF")) && len(data) >= 12 && bytes.Equal(data[8:12], []byte("WEBP")) {
		return ".webp", "image/webp"
	}
	return "", ""
}

func (a *App) applyWeChatProfile(r *http.Request, uid int64, prof wechatProfile) {
	if a == nil || a.Store == nil || uid <= 0 || prof.empty() {
		return
	}
	ctx := r.Context()
	var name, avatar string
	if err := a.Store.QueryRow(ctx, `SELECT COALESCE(name,''), COALESCE(avatar_url,'') FROM app_users WHERE id=$1`, uid).Scan(&name, &avatar); err != nil {
		return
	}
	if strings.TrimSpace(name) == "" {
		if nick := clipWeChatName(prof.Nickname); nick != "" {
			_, _ = a.Store.Exec(ctx, `UPDATE app_users SET name=$1 WHERE id=$2 AND name=''`, nick, uid)
		}
	}
	if strings.TrimSpace(avatar) != "" {
		return
	}
	stored := a.storeWeChatAvatar(r, prof.Avatar)
	if stored == "" {
		return
	}
	_, _ = a.Store.Exec(ctx, `UPDATE app_users SET avatar_url=$1 WHERE id=$2 AND avatar_url=''`, stored, uid)
}

func (a *App) storeWeChatAvatar(r *http.Request, rawURL string) string {
	if a == nil || a.Upload == nil {
		return ""
	}
	u, ok := wechatAvatarURLOK(rawURL)
	if !ok {
		return ""
	}
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ""
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, uploadMaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > uploadMaxBytes {
		return ""
	}
	ext, contentType := imageExtFrom(resp.Header.Get("Content-Type"), data)
	if ext == "" {
		return ""
	}
	name, err := randomHex(16)
	if err != nil {
		return ""
	}
	stored := "misc/" + name + ext
	if _, err := a.Upload.Save(r, stored, data, contentType); err != nil {
		return ""
	}
	return "/uploads/" + stored
}
