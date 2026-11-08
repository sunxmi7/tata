package tata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 微信开放平台「移动应用」换票。一个应用同时填 iOS Bundle ID 和 Android 包名，共用 appId/appSecret。
const wechatTokenURL = "https://api.weixin.qq.com/sns/oauth2/access_token"

type WeChatConfig struct {
	Enabled   bool
	AppID     string
	AppSecret string
}

type wechatExchanger interface {
	Ready() bool
	ExchangeCode(ctx context.Context, code string) (string, error)
}

type WeChatClient struct {
	cfg      WeChatConfig
	http     *http.Client
	tokenURL string
}

func NewWeChatClient(cfg WeChatConfig) *WeChatClient {
	return &WeChatClient{
		cfg:      cfg,
		http:     &http.Client{Timeout: 10 * time.Second},
		tokenURL: wechatTokenURL,
	}
}

func (c *WeChatClient) Ready() bool {
	if c == nil || !c.cfg.Enabled {
		return false
	}
	return strings.TrimSpace(c.cfg.AppID) != "" && strings.TrimSpace(c.cfg.AppSecret) != ""
}

func (c *WeChatClient) ExchangeCode(ctx context.Context, code string) (string, error) {
	if c == nil || !c.Ready() {
		return "", biz("微信登录未配置")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return "", biz("微信授权码不能为空")
	}
	u, err := url.Parse(c.tokenURL)
	if err != nil {
		return "", fmt.Errorf("wechat token url: %w", err)
	}
	q := u.Query()
	q.Set("appid", strings.TrimSpace(c.cfg.AppID))
	q.Set("secret", strings.TrimSpace(c.cfg.AppSecret))
	q.Set("code", code)
	q.Set("grant_type", "authorization_code")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", biz("微信授权失败")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", biz("微信授权失败")
	}
	var out struct {
		OpenID  string `json:"openid"`
		UnionID string `json:"unionid"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", biz("微信授权失败")
	}
	if out.ErrCode != 0 {
		switch out.ErrCode {
		case 40029, 40163:
			return "", biz("微信授权码无效")
		default:
			return "", biz("微信授权失败")
		}
	}
	// 同一开放平台账号下 unionid 跨应用稳定；iOS/Android 共用一个移动应用时 openid 也相同。
	id := strings.TrimSpace(out.UnionID)
	if id == "" {
		id = strings.TrimSpace(out.OpenID)
	}
	if id == "" {
		return "", biz("微信授权失败")
	}
	return id, nil
}

func (a *App) wechatIdentity(ctx context.Context, code string) (string, error) {
	if a == nil || a.WeChat == nil || !a.WeChat.Ready() {
		return "", biz("微信登录未配置")
	}
	return a.WeChat.ExchangeCode(ctx, code)
}
