package tata

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
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

	mux.HandleFunc("GET /api/admin/v1/deck-products", a.h(a.adminListDeckProducts))
	mux.HandleFunc("PUT /api/admin/v1/deck-products/{code}", a.h(a.adminUpdateDeckProduct))
	mux.HandleFunc("POST /api/admin/v1/deck-unlocks", a.h(a.adminGrantDeckUnlock))

	mux.HandleFunc("GET /api/admin/v1/spreads", a.h(a.adminListSpreads))
	mux.HandleFunc("POST /api/admin/v1/spreads", a.h(a.adminCreateSpread))
	mux.HandleFunc("PUT /api/admin/v1/spreads/{id}", a.h(a.adminUpdateSpread))
	mux.HandleFunc("DELETE /api/admin/v1/spreads/{id}", a.h(a.adminDeleteSpread))

	mux.HandleFunc("GET /api/admin/v1/cards", a.h(a.adminListCards))
	mux.HandleFunc("POST /api/admin/v1/cards", a.h(a.adminCreateCard))
	mux.HandleFunc("PUT /api/admin/v1/cards/{id}", a.h(a.adminUpdateCard))
	mux.HandleFunc("PUT /api/admin/v1/card-settings", a.h(a.adminPutCardSettings))

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
	mux.HandleFunc("PUT /api/admin/v1/quota-params", a.h(a.adminUpdateQuota))
	mux.HandleFunc("POST /api/admin/v1/quota-params", a.h(a.rejectQuotaWrite))

	mux.HandleFunc("GET /api/admin/v1/copy/{kind}", a.h(a.adminGetCopy))
	mux.HandleFunc("POST /api/admin/v1/copy/{kind}", a.h(a.adminPostCopy))
	mux.HandleFunc("PUT /api/admin/v1/copy/{kind}", a.h(a.adminPutCopy))
	mux.HandleFunc("DELETE /api/admin/v1/copy/{kind}/{id}", a.h(a.adminDeleteCopy))
	mux.HandleFunc("POST /api/admin/v1/uploads", a.h(a.adminUploads))

	mux.HandleFunc("GET /api/admin/v1/banners", a.h(a.adminListBanners))
	mux.HandleFunc("POST /api/admin/v1/banners", a.h(a.adminCreateBanner))
	mux.HandleFunc("PUT /api/admin/v1/banners/{id}", a.h(a.adminUpdateBanner))
	mux.HandleFunc("POST /api/admin/v1/push/broadcast", a.h(a.adminBroadcastPush))

	mux.HandleFunc("GET /api/admin/v1/readings", a.h(a.adminListReadings))
	mux.HandleFunc("GET /api/admin/v1/readings/{id}", a.h(a.adminGetReading))
	mux.HandleFunc("POST /api/admin/v1/readings/{id}/retry", a.h(a.adminRetry))
	mux.HandleFunc("POST /api/admin/v1/workflow/simulate", a.h(a.adminSimulate))

	mux.HandleFunc("GET /api/admin/v1/ledger", a.h(a.adminLedger))
	mux.HandleFunc("POST /api/admin/v1/ledger", a.h(a.rejectLedgerWrite))
	mux.HandleFunc("PUT /api/admin/v1/ledger/{id}", a.h(a.rejectLedgerWrite))
	mux.HandleFunc("GET /api/admin/v1/grants", a.h(a.adminListGrants))
	mux.HandleFunc("POST /api/admin/v1/grants", a.h(a.adminCreateGrant))
	mux.HandleFunc("GET /api/admin/v1/invites", a.h(a.adminListInvites))
	mux.HandleFunc("GET /api/admin/v1/registration-gifts", a.h(a.adminListRegistrationGifts))
	mux.HandleFunc("GET /api/admin/v1/quota-lots", a.h(a.adminListQuotaLots))
	mux.HandleFunc("GET /api/admin/v1/yearly-qualifications", a.h(a.adminListYearlyQualifications))
	mux.HandleFunc("GET /api/admin/v1/redeem-codes", a.h(a.adminListRedeemCodes))
	mux.HandleFunc("POST /api/admin/v1/redeem-codes", a.h(a.adminCreateRedeemCodes))
	mux.HandleFunc("PUT /api/admin/v1/redeem-codes/{id}", a.h(a.adminUpdateRedeemCode))

	mux.HandleFunc("GET /api/admin/v1/operators", a.h(a.adminListOperators))
	mux.HandleFunc("POST /api/admin/v1/operators", a.h(a.adminCreateOperator))
	mux.HandleFunc("PUT /api/admin/v1/operators/{id}", a.h(a.adminUpdateOperator))
	mux.HandleFunc("GET /api/admin/v1/users", a.h(a.adminListUsers))
	mux.HandleFunc("GET /api/admin/v1/onboarding", a.h(a.adminListOnboarding))
	mux.HandleFunc("GET /api/admin/v1/users/stats", a.h(a.adminUserStats))
	mux.HandleFunc("GET /api/admin/v1/audit", a.h(a.adminListAudit))
	mux.HandleFunc("GET /api/admin/v1/account-merges", a.h(a.adminListAccountMerges))
	mux.HandleFunc("GET /api/admin/v1/iap/orders", a.h(a.adminListIAPOrders))
	mux.HandleFunc("GET /api/admin/v1/iap/skus", a.h(a.adminListIAPSKUs))
	mux.HandleFunc("POST /api/admin/v1/iap/skus", a.h(a.adminCreateIAPSKU))
	mux.HandleFunc("PUT /api/admin/v1/iap/skus/{id}", a.h(a.adminUpdateIAPSKU))
	mux.HandleFunc("GET /api/admin/v1/iap/quota", a.h(a.adminIAPQuota))
	mux.HandleFunc("GET /api/admin/v1/iap/config", a.h(a.adminIAPConfig))
	mux.HandleFunc("GET /api/admin/v1/messages", a.h(a.adminListMessages))
	mux.HandleFunc("GET /api/admin/v1/calendar", a.h(a.adminListCalendar))
	mux.HandleFunc("GET /api/admin/v1/wishlists", a.h(a.adminListWishlists))
	mux.HandleFunc("GET /api/admin/v1/feedback", a.h(a.adminListFeedback))
	mux.HandleFunc("POST /api/app/v1/auth/email/code", a.h(a.appSendEmailCode))
	mux.HandleFunc("POST /api/app/v1/auth/login", a.appLogin)
	mux.HandleFunc("POST /api/app/v1/auth/wechat", a.appWechat)
	mux.HandleFunc("POST /api/app/v1/auth/apple", a.appApple)
	mux.HandleFunc("POST /api/app/v1/auth/bind-wechat", a.h(a.appBindWechat))
	mux.HandleFunc("POST /api/app/v1/auth/bind-apple", a.h(a.appBindApple))
	mux.HandleFunc("POST /api/app/v1/auth/bind-email", a.h(a.appBindEmail))
	mux.HandleFunc("POST /api/app/v1/auth/merge-confirm", a.h(a.appMergeConfirm))
	mux.HandleFunc("POST /api/app/v1/auth/cancel", a.h(a.appCancel))
	mux.HandleFunc("GET /api/app/v1/profile", a.h(a.appGetProfile))
	mux.HandleFunc("PUT /api/app/v1/profile", a.h(a.appPutProfile))
	mux.HandleFunc("GET /api/app/v1/legal", a.h(a.appLegal))
	mux.HandleFunc("GET /api/app/v1/onboarding", a.h(a.appGetOnboarding))
	mux.HandleFunc("PUT /api/app/v1/onboarding", a.h(a.appPutOnboarding))
	mux.HandleFunc("POST /api/app/v1/onboarding/abandon", a.h(a.appAbandonOnboarding))
	mux.HandleFunc("POST /api/app/v1/onboarding/sync", a.h(a.appSyncOnboarding))
	mux.HandleFunc("GET /api/app/v1/invites", a.h(a.appGetInvites))
	mux.HandleFunc("POST /api/app/v1/invites/open", a.h(a.appOpenInvite))
	mux.HandleFunc("POST /api/app/v1/invites/accept", a.h(a.appAcceptInvite))
	mux.HandleFunc("GET /api/app/v1/cards", a.h(a.appListCards))
	mux.HandleFunc("GET /api/app/v1/spreads", a.h(a.appSpreads))
	mux.HandleFunc("GET /api/app/v1/explore/topics", a.h(a.appExploreTopics))
	mux.HandleFunc("GET /api/app/v1/preset-questions", a.h(a.appPresetQuestions))
	mux.HandleFunc("GET /api/app/v1/banners", a.h(a.appBanners))
	mux.HandleFunc("GET /api/app/v1/tarotists", a.h(a.appTarotists))
	mux.HandleFunc("GET /api/app/v1/quota", a.h(a.appQuota))
	mux.HandleFunc("POST /api/app/v1/question-suggestions", a.h(a.appQuestionSuggestions))
	mux.HandleFunc("POST /api/app/v1/readings", a.h(a.appCreateReading))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/recommend", a.h(a.appRecommend))
	mux.HandleFunc("GET /api/app/v1/readings/{id}/spread-choices", a.h(a.appSpreadChoices))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/spread", a.h(a.appSelectSpread))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/numerology", a.h(a.appNumerologyMessages))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/redraw", a.h(a.appRedraw))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/interpret", a.h(a.appInterpret))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/interpret/stream", a.h(a.appInterpretStream))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/followups", a.h(a.appFollowup))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/followups/redraw", a.h(a.appFollowupRedraw))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/followups/stream", a.h(a.appFollowupStream))
	mux.HandleFunc("POST /api/app/v1/readings/{id}/workflow", a.h(a.appWorkflow))
	mux.HandleFunc("GET /api/app/v1/readings", a.h(a.appListReadings))
	mux.HandleFunc("GET /api/app/v1/readings/{id}", a.h(a.appGetReading))
	mux.HandleFunc("DELETE /api/app/v1/readings/{id}", a.h(a.appDeleteReading))
	mux.HandleFunc("POST /api/app/v1/daily", a.h(a.appDaily))
	mux.HandleFunc("POST /api/app/v1/daily/stream", a.h(a.appDailyStream))
	mux.HandleFunc("GET /api/app/v1/calendar", a.h(a.appCalendar))
	mux.HandleFunc("GET /api/app/v1/monthly-reports", a.h(a.appMonthlyReports))
	mux.HandleFunc("POST /api/app/v1/monthly-reports/save", a.h(a.appSaveMonthlyReport))
	mux.HandleFunc("GET /api/app/v1/yearly-reports/{id}", a.h(a.appYearlyReport))
	mux.HandleFunc("GET /api/app/v1/yearly-reports/{id}/share", a.h(a.appYearlyShare))
	mux.HandleFunc("POST /api/app/v1/yearly-reports/{id}/unlock", a.h(a.appUnlockYearlyReport))
	mux.HandleFunc("PUT /api/app/v1/yearly-reports/{id}/profile", a.h(a.appYearlyProfile))
	mux.HandleFunc("POST /api/app/v1/yearly-reports/{id}/rhythm", a.h(a.appYearlyRhythm))
	mux.HandleFunc("POST /api/app/v1/yearly-reports/{id}/domains", a.h(a.appYearlyDomains))
	mux.HandleFunc("POST /api/app/v1/yearly-reports/{id}/generate", a.h(a.appYearlyGenerate))
	mux.HandleFunc("POST /api/app/v1/yearly-reports/{id}/generate/stream", a.h(a.appYearlyGenerateStream))
	mux.HandleFunc("GET /api/app/v1/deck-shop", a.h(a.appDeckShop))
	mux.HandleFunc("GET /api/app/v1/deck-preference", a.h(a.appGetDeckPreference))
	mux.HandleFunc("PUT /api/app/v1/deck-preference", a.h(a.appPutDeckPreference))
	mux.HandleFunc("POST /api/app/v1/redeem", a.h(a.appRedeem))
	mux.HandleFunc("POST /api/app/v1/uploads", a.h(a.appUploads))
	mux.HandleFunc("GET /uploads/{name...}", a.serveUpload)
	mux.HandleFunc("POST /api/app/v1/wishlist", a.h(a.appWishlist))
	mux.HandleFunc("POST /api/app/v1/feedback", a.h(a.appFeedback))
	mux.HandleFunc("GET /api/app/v1/notification-preferences", a.h(a.appGetNotificationPrefs))
	mux.HandleFunc("PUT /api/app/v1/notification-preferences", a.h(a.appPutNotificationPrefs))
	// 推送（2026-11-06 重启 Q-B1）：今日/明日抽牌走 App 本地通知；优惠/活动等消息类走个推服务端下发。
	mux.HandleFunc("POST /api/app/v1/push/register", a.h(a.appRegisterPush))
	mux.HandleFunc("GET /api/app/v1/messages", a.h(a.appListInbox))
	mux.HandleFunc("POST /api/app/v1/messages/{id}/read", a.h(a.appReadMessage))
	mux.HandleFunc("GET /api/app/v1/redeem/records", a.h(a.appRedeemRecords))
	mux.HandleFunc("GET /api/app/v1/campaigns", a.h(a.appCampaigns))
	mux.HandleFunc("GET /api/app/v1/campaigns/participations", a.h(a.appCampaignParticipations))
	mux.HandleFunc("GET /api/app/v1/campaigns/rewards", a.h(a.appCampaignRewards))
	mux.HandleFunc("GET /api/app/v1/campaigns/{id}", a.h(a.appCampaign))
	mux.HandleFunc("GET /api/app/v1/help/categories", a.h(a.appHelpCategories))
	mux.HandleFunc("GET /api/app/v1/help/articles", a.h(a.appHelpArticles))
	mux.HandleFunc("GET /api/app/v1/help/articles/{id}", a.h(a.appHelpArticle))

	return withAppLanguage(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, Accept-Language, X-Language, Accept")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	}))
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
			// 传 r.Response.Writer（内嵌的 *response.Writer），它直接包裹原始 http.ResponseWriter：
			// Write/WriteHeader 不走 ghttp 的 BufferWriter 缓冲，Flush 直达底层 http.Flusher。
			// SSE 流式（interpret/stream、followups/stream）依赖这一点才能逐段下发，勿改成 r.Response 本身。
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
			writeErrCodeLang(w, r.Context(), http.StatusUnauthorized, "unauthorized", "未登录或令牌无效")
			return
		}
		var be *bizError
		if errors.As(err, &be) {
			writeErrCodeLang(w, r.Context(), http.StatusOK, be.code, be.msg)
			return
		}
		log.Printf("internal: %v", err)
		writeErrCodeLang(w, r.Context(), http.StatusOK, "internal", "内部错误")
	}
}

type actor struct {
	id   int64
	role string
}

func (a *App) adminActor(r *http.Request, write bool) (actor, error) {
	id, role, err := a.parseAccess(r, "admin")
	if err != nil {
		return actor{}, err
	}
	var disabled bool
	err = a.Store.QueryRow(r.Context(), `SELECT disabled FROM operators WHERE id=$1`, id).Scan(&disabled)
	if err != nil || disabled {
		return actor{}, authFail{}
	}
	if err := a.checkAdminAccess(r, id, role, write); err != nil {
		return actor{}, err
	}
	return actor{id: id, role: role}, nil
}

func (a *App) appActor(r *http.Request) (userID int64, err error) {
	userID, _, err = a.parseAccess(r, "app")
	if err != nil {
		return 0, err
	}
	if userID == 0 {
		return 0, authFail{}
	}
	var merged *int64
	var cancelled *time.Time
	scanErr := a.Store.QueryRow(r.Context(), `SELECT merged_into, cancelled_at FROM app_users WHERE id=$1`, userID).Scan(&merged, &cancelled)
	if scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return 0, authFail{}
		}
		return 0, scanErr
	}
	if merged != nil {
		return 0, authFail{}
	}
	if cancelled != nil {
		return 0, biz("账号已注销")
	}
	return userID, nil
}

func (a *App) lockGen(id int64) (func(), error) {
	return a.lockNamed("gen:" + fmtID(id))
}

func (a *App) lockNamed(key string) (func(), error) {
	if a.Redis == nil {
		return func() {}, nil
	}
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

// divinationActor accepts a logged-in token, or a device id when the caller has not logged in.
// The device id is how a later onboarding check finds this handset.
func (a *App) divinationActor(r *http.Request) (userID int64, deviceID string, err error) {
	if strings.TrimSpace(r.Header.Get("Authorization")) != "" {
		userID, err = a.appActor(r)
		if err != nil {
			return 0, "", err
		}
		if userID > 0 {
			return userID, "", nil
		}
	}
	deviceID = strings.TrimSpace(r.URL.Query().Get("device_id"))
	if deviceID == "" {
		deviceID = peekDeviceID(r)
	}
	if !validDeviceID(deviceID) {
		return 0, "", biz("缺少设备标识")
	}
	return 0, deviceID, nil
}

func (a *App) beginReading(r *http.Request) (userID, id int64, deviceID string, err error) {
	userID, deviceID, err = a.divinationActor(r)
	if err != nil {
		return 0, 0, "", err
	}
	id, err = pathID(r)
	if err != nil {
		return 0, 0, "", err
	}
	if err = a.ownReading(r.Context(), id, userID, deviceID); err != nil {
		return 0, 0, "", err
	}
	return userID, id, deviceID, nil
}

func peekDeviceID(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var probe struct {
		DeviceID string `json:"device_id"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		return ""
	}
	return strings.TrimSpace(probe.DeviceID)
}

func (a *App) ownReading(ctx context.Context, id, userID int64, guest string) error {
	var owner *int64
	var deviceID string
	var deleted *time.Time
	err := a.Store.QueryRow(ctx, `SELECT user_id, COALESCE(device_id,''), deleted_at FROM readings WHERE id=$1`, id).Scan(&owner, &deviceID, &deleted)
	if err != nil {
		return bizCode("reading_not_found", "记录不存在")
	}
	if deleted != nil {
		return bizCode("reading_deleted", "记录已删除")
	}
	if userID > 0 {
		if owner == nil || *owner != userID {
			return bizCode("reading_forbidden", "记录不属于当前用户")
		}
		return nil
	}
	guest = strings.TrimSpace(guest)
	if guest != "" && strings.EqualFold(deviceID, guest) {
		return nil
	}
	return bizCode("reading_forbidden", "记录不属于当前用户")
}
