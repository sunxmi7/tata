package tata

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
)

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /demo/openai/v1/chat/completions", a.demoOpenAI)
	mux.HandleFunc("GET /admin", a.adminPage)
	mux.HandleFunc("GET /admin/", a.adminPage)
	mux.HandleFunc("GET /demo", a.demoPage)

	mux.HandleFunc("POST /api/admin/v1/auth/login", a.adminLogin)
	mux.HandleFunc("POST /api/admin/v1/auth/logout", a.adminLogout)
	mux.HandleFunc("POST /api/admin/v1/auth/refresh", a.adminRefresh)
	mux.HandleFunc("GET /api/admin/v1/user/info", a.h(a.adminInfo))
	mux.HandleFunc("GET /api/admin/v1/auth/codes", a.h(a.adminCodes))
	mux.HandleFunc("GET /api/admin/v1/menu/all", a.h(a.adminMenu))

	mux.HandleFunc("GET /api/admin/v1/spreads", a.h(a.adminListSpreads))
	mux.HandleFunc("POST /api/admin/v1/spreads", a.h(a.adminCreateSpread))
	mux.HandleFunc("PUT /api/admin/v1/spreads/{id}", a.h(a.adminUpdateSpread))
	mux.HandleFunc("DELETE /api/admin/v1/spreads/{id}", a.h(a.adminDeleteSpread))

	mux.HandleFunc("GET /api/admin/v1/tarotists", a.h(a.adminListTarotists))
	mux.HandleFunc("POST /api/admin/v1/tarotists", a.h(a.adminCreateTarotist))
	mux.HandleFunc("PUT /api/admin/v1/tarotists/{id}", a.h(a.adminUpdateTarotist))

	mux.HandleFunc("GET /api/admin/v1/vendors", a.h(a.adminListVendors))
	mux.HandleFunc("POST /api/admin/v1/vendors", a.h(a.adminCreateVendor))
	mux.HandleFunc("PUT /api/admin/v1/vendors/{id}", a.h(a.adminUpdateVendor))
	mux.HandleFunc("DELETE /api/admin/v1/vendors/{id}", a.h(a.adminDeleteVendor))
	mux.HandleFunc("POST /api/admin/v1/vendors/{id}/probe", a.h(a.adminProbeVendor))

	mux.HandleFunc("GET /api/admin/v1/bindings", a.h(a.adminListBindings))
	mux.HandleFunc("PUT /api/admin/v1/bindings/{task}", a.h(a.adminPutBinding))

	mux.HandleFunc("GET /api/admin/v1/quota-params", a.h(a.adminQuota))
	mux.HandleFunc("PUT /api/admin/v1/quota-params", a.h(a.rejectQuotaWrite))
	mux.HandleFunc("POST /api/admin/v1/quota-params", a.h(a.rejectQuotaWrite))

	mux.HandleFunc("GET /api/admin/v1/banners", a.h(a.adminListBanners))
	mux.HandleFunc("POST /api/admin/v1/banners", a.h(a.adminCreateBanner))
	mux.HandleFunc("PUT /api/admin/v1/banners/{id}", a.h(a.adminUpdateBanner))

	mux.HandleFunc("GET /api/admin/v1/readings", a.h(a.adminListReadings))
	mux.HandleFunc("GET /api/admin/v1/readings/{id}", a.h(a.adminGetReading))
	mux.HandleFunc("POST /api/admin/v1/readings/{id}/retry", a.h(a.adminRetry))
	mux.HandleFunc("POST /api/admin/v1/workflow/simulate", a.h(a.adminSimulate))

	mux.HandleFunc("GET /api/admin/v1/ledger", a.h(a.adminLedger))
	mux.HandleFunc("POST /api/admin/v1/ledger", a.h(a.rejectLedgerWrite))
	mux.HandleFunc("PUT /api/admin/v1/ledger/{id}", a.h(a.rejectLedgerWrite))

	mux.HandleFunc("GET /api/admin/v1/operators", a.h(a.adminListOperators))
	mux.HandleFunc("POST /api/admin/v1/operators", a.h(a.adminCreateOperator))
	mux.HandleFunc("PUT /api/admin/v1/operators/{id}", a.h(a.adminUpdateOperator))
	mux.HandleFunc("GET /api/admin/v1/users", a.h(a.adminListUsers))
	mux.HandleFunc("GET /api/admin/v1/audit", a.h(a.adminListAudit))
	mux.HandleFunc("GET /api/admin/v1/account-merges", a.h(a.adminListAccountMerges))
	mux.HandleFunc("GET /api/admin/v1/iap/orders", a.h(a.adminListIAPOrders))
	mux.HandleFunc("GET /api/admin/v1/iap/skus", a.h(a.adminListIAPSKUs))
	mux.HandleFunc("GET /api/admin/v1/iap/diamonds", a.h(a.adminIAPDiamonds))
	mux.HandleFunc("GET /api/admin/v1/iap/config", a.h(a.adminIAPConfig))
	mux.HandleFunc("GET /api/admin/v1/messages", a.h(a.adminListMessages))
	mux.HandleFunc("GET /api/admin/v1/calendar", a.h(a.adminListCalendar))
	mux.HandleFunc("POST /api/app/v1/auth/anonymous", a.appAnonymous)
	mux.HandleFunc("POST /api/app/v1/auth/register", a.appRegister)
	mux.HandleFunc("POST /api/app/v1/auth/login", a.appLogin)
	mux.HandleFunc("POST /api/app/v1/auth/wechat", a.appWechat)
	mux.HandleFunc("POST /api/app/v1/auth/apple", a.appApple)
	mux.HandleFunc("POST /api/app/v1/auth/bind-wechat", a.h(a.appBindWechat))
	mux.HandleFunc("POST /api/app/v1/auth/bind-apple", a.h(a.appBindApple))
	mux.HandleFunc("POST /api/app/v1/auth/bind-email", a.h(a.appBindEmail))
	mux.HandleFunc("POST /api/app/v1/auth/bind-phone", a.h(a.appBindPhone))
	mux.HandleFunc("POST /api/app/v1/auth/merge-confirm", a.h(a.appMergeConfirm))
	mux.HandleFunc("POST /api/app/v1/auth/cancel", a.h(a.appCancel))
	mux.HandleFunc("GET /api/app/v1/profile", a.h(a.appGetProfile))
	mux.HandleFunc("PUT /api/app/v1/profile", a.h(a.appPutProfile))
	mux.HandleFunc("GET /api/app/v1/spreads", a.h(a.appSpreads))
	mux.HandleFunc("GET /api/app/v1/banners", a.h(a.appBanners))
	mux.HandleFunc("GET /api/app/v1/tarotists", a.h(a.appTarotists))
	mux.HandleFunc("GET /api/app/v1/quota", a.h(a.appQuota))
	mux.HandleFunc("POST /api/app/v1/readings", a.h(a.appCreateReading))
	mux.HandleFunc("PUT /api/app/v1/readings/{id}/question", a.h(a.appEditQuestion))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/detect", a.h(a.appDetect))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/spread", a.h(a.appSelectSpread))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/shuffle", a.h(a.appShuffle))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/cut", a.h(a.appCut))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/draw", a.h(a.appDraw))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/interpret", a.h(a.appInterpret))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/confirm-quota", a.h(a.appConfirmQuota))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/use-reward", a.h(a.appUseReward))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/paywall", a.h(a.appCancelPaywall))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/followups", a.h(a.appFollowup))
	mux.HandleFunc("GET /api/app/v1/readings/{id}/messages", a.h(a.appMessages))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/workflow", a.h(a.appWorkflow))
	mux.HandleFunc("GET /api/app/v1/readings", a.h(a.appListReadings))
	mux.HandleFunc("GET /api/app/v1/readings/{id}", a.h(a.appGetReading))
	mux.HandleFunc("DELETE /api/app/v1/readings/{id}", a.h(a.appDeleteReading))
	mux.HandleFunc("POST /api/app/v1/daily", a.h(a.appDaily))
	mux.HandleFunc("GET /api/app/v1/calendar", a.h(a.appCalendar))
	mux.HandleFunc("POST /api/app/v1/iap/verify", a.h(a.appIAPVerify))
	mux.HandleFunc("POST /api/app/v1/iap/restore", a.h(a.appIAPRestore))
	mux.HandleFunc("GET /api/app/v1/iap/entitlements", a.h(a.appIAPEntitlements))
	mux.HandleFunc("GET /api/app/v1/iap/skus", a.h(a.appIAPSKUs))
	mux.HandleFunc("GET /api/app/v1/iap/config", a.h(a.appIAPConfig))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (a *App) Bind(s *ghttp.Server) {
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			r.Response.CORSDefault()
			r.Middleware.Next()
		})
		group.ALL("/*", func(r *ghttp.Request) {
			if id := r.Get("id").String(); id != "" {
				r.Request.SetPathValue("id", id)
			}
			if task := r.Get("task").String(); task != "" {
				r.Request.SetPathValue("task", task)
			}
			a.Handler().ServeHTTP(r.Response.Writer, r.Request)
		})
	})
}

func (a *App) h(fn func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := fn(w, r)
		if err == nil {
			return
		}
		var ae authFail
		if errors.As(err, &ae) {
			writeErr(w, http.StatusUnauthorized, "未登录或令牌无效")
			return
		}
		var be *bizError
		if errors.As(err, &be) {
			writeErr(w, http.StatusOK, be.msg)
			return
		}
		log.Printf("internal: %v", err)
		writeErr(w, http.StatusOK, "内部错误")
	}
}

type actor struct {
	id   int64
	role string
}

func (a *App) adminActor(r *http.Request, write bool) (actor, error) {
	id, _, role, err := a.parseAccess(r, "admin")
	if err != nil {
		return actor{}, err
	}
	var disabled bool
	err = a.Store.QueryRow(r.Context(), `SELECT disabled FROM operators WHERE id=$1`, id).Scan(&disabled)
	if err != nil || disabled {
		return actor{}, authFail{}
	}
	if write && role != "admin" {
		return actor{}, biz("只读账号不能修改")
	}
	return actor{id: id, role: role}, nil
}

func (a *App) appActor(r *http.Request) (userID int64, anon string, err error) {
	userID, anon, _, err = a.parseAccess(r, "app")
	if err != nil || userID == 0 {
		return userID, anon, err
	}
	var merged *int64
	scanErr := a.Store.QueryRow(r.Context(), `SELECT merged_into FROM app_users WHERE id=$1`, userID).Scan(&merged)
	if scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return 0, "", authFail{}
		}
		return 0, "", scanErr
	}
	if merged != nil {
		return 0, "", authFail{}
	}
	return userID, anon, nil
}

func (a *App) lockGen(id int64) (func(), error) {
	if a.Redis == nil {
		return func() {}, nil
	}
	key := "gen:" + fmtID(id)
	ok, err := a.Redis.Lock(key, 60*time.Second)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, biz("生成进行中")
	}
	return func() { _ = a.Redis.Unlock(key) }, nil
}

func beijingNow() time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return time.Now().In(loc)
}

func validBase(raw string) error {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "https://") {
		return nil
	}
	if strings.HasPrefix(raw, "http://127.0.0.1") || strings.HasPrefix(raw, "http://localhost") {
		return nil
	}
	return biz("供应商地址只允许 HTTPS；本机演示可使用 http://127.0.0.1 或 http://localhost")
}

func (a *App) ownReading(ctx context.Context, id, userID int64, anon string) error {
	var owner *int64
	var anonID *string
	var deleted *time.Time
	err := a.Store.QueryRow(ctx, `SELECT user_id, anonymous_id, deleted_at FROM readings WHERE id=$1`, id).Scan(&owner, &anonID, &deleted)
	if err != nil {
		return biz("记录不存在")
	}
	if deleted != nil {
		return biz("记录已删除")
	}
	if userID > 0 {
		if owner == nil || *owner != userID {
			return biz("记录不属于当前用户")
		}
		return nil
	}
	if anonID == nil || *anonID != anon {
		return biz("记录不属于当前用户")
	}
	return nil
}
