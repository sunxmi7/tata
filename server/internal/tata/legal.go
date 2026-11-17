package tata

import (
	"net/http"
	"strings"
)

const (
	legalUserAgreementKey = "legal.user_agreement"
	legalPrivacyPolicyKey = "legal.privacy_policy"
	legalUserAgreement    = "用户协议"
	legalPrivacyPolicy    = "隐私政策"
)

func (a *App) appLegal(w http.ResponseWriter, r *http.Request) error {
	cfg, err := a.Store.configMap(r.Context(), "legal.")
	if err != nil {
		return err
	}
	writeOK(w, map[string]any{
		"user_agreement": legalDoc(r, legalUserAgreement, cfg[legalUserAgreementKey]),
		"privacy_policy": legalDoc(r, legalPrivacyPolicy, cfg[legalPrivacyPolicyKey]),
	})
	return nil
}

func legalDoc(r *http.Request, title, body string) map[string]any {
	status := copyStatus(body)
	if status == "pending" {
		body = T(r.Context(), helpBodyPending)
	}
	return map[string]any{
		"title": T(r.Context(), title), "body": body, "body_status": status,
	}
}

func (a *App) copyLegal(w http.ResponseWriter, r *http.Request) error {
	cfg, err := a.Store.configMap(r.Context(), "legal.")
	if err != nil {
		return err
	}
	writeOK(w, map[string]any{
		"kind": "legal",
		"user_agreement": map[string]any{
			"body": cfg[legalUserAgreementKey], "body_status": copyStatus(cfg[legalUserAgreementKey]),
		},
		"privacy_policy": map[string]any{
			"body": cfg[legalPrivacyPolicyKey], "body_status": copyStatus(cfg[legalPrivacyPolicyKey]),
		},
	})
	return nil
}

func (a *App) putCopyLegal(r *http.Request) error {
	var req struct {
		UserAgreement string `json:"user_agreement"`
		PrivacyPolicy string `json:"privacy_policy"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if _, err := a.Store.Exec(r.Context(), `INSERT INTO app_config(key, value, updated_at) VALUES ($1,$2,now())
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`,
		legalUserAgreementKey, strings.TrimSpace(req.UserAgreement)); err != nil {
		return err
	}
	if _, err := a.Store.Exec(r.Context(), `INSERT INTO app_config(key, value, updated_at) VALUES ($1,$2,now())
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`,
		legalPrivacyPolicyKey, strings.TrimSpace(req.PrivacyPolicy)); err != nil {
		return err
	}
	return nil
}
