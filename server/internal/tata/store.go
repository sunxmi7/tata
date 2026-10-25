package tata

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/crypto/bcrypt"
)

type Store struct {
	DB  *sql.DB
	KEK []byte
}

func Open(ctx context.Context, dsn string, kek []byte) (*Store, error) {
	if dsn == "" {
		dsn = g.Cfg().MustGet(ctx, "database.default.link").String()
	}
	if err := gdb.SetConfig(gdb.Config{
		"default": gdb.ConfigGroup{{
			Link: toGFLink(dsn),
		}},
	}); err != nil {
		return nil, err
	}
	if err := g.DB().PingMaster(); err != nil {
		return nil, err
	}
	sqlDB, err := g.DB().Master()
	if err != nil {
		return nil, err
	}
	s := &Store{DB: sqlDB, KEK: kek}
	if err := s.migrate(ctx); err != nil {
		return nil, err
	}
	if err := s.seed(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.Exec(ctx, `
CREATE TABLE IF NOT EXISTS operators (
  id BIGSERIAL PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL,
  disabled BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS app_users (
  id BIGSERIAL PRIMARY KEY,
  email TEXT UNIQUE,
  password_hash TEXT,
  birth_date TEXT,
  gender TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS refresh_tokens (
  id TEXT PRIMARY KEY,
  audience TEXT NOT NULL,
  subject_id BIGINT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE IF NOT EXISTS spreads (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  card_count INT NOT NULL,
  positions TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  charged BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS tarotists (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  persona TEXT NOT NULL DEFAULT '',
  display_copy TEXT NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS vendors (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  api_key_enc BYTEA NOT NULL,
  models TEXT NOT NULL,
  timeout_sec INT,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS bindings (
  task TEXT PRIMARY KEY,
  vendor_id BIGINT NOT NULL,
  model TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS banners (
  id BIGSERIAL PRIMARY KEY,
  copy TEXT NOT NULL,
  start_at TIMESTAMPTZ NOT NULL,
  end_at TIMESTAMPTZ NOT NULL,
  priority INT NOT NULL DEFAULT 0,
  target TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS readings (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT,
  anonymous_id TEXT,
  type TEXT NOT NULL,
  question TEXT NOT NULL DEFAULT '',
  spread_id BIGINT,
  tarotist_id BIGINT,
  status TEXT NOT NULL,
  cards TEXT,
  interpretation TEXT,
  vendor_name TEXT,
  model_name TEXT,
  fail_reason TEXT,
  quota_consumed BOOLEAN NOT NULL DEFAULT FALSE,
  first_experience BOOLEAN NOT NULL DEFAULT FALSE,
  quota_cycle TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS ledger (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  reading_id BIGINT NOT NULL,
  entitlement TEXT NOT NULL,
  amount INT NOT NULL,
  reason TEXT NOT NULL,
  result TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS quota_use (
  user_id BIGINT NOT NULL,
  cycle TEXT NOT NULL,
  reading_id BIGINT NOT NULL,
  PRIMARY KEY (user_id, cycle)
);
CREATE TABLE IF NOT EXISTS daily_slot (
  user_id BIGINT NOT NULL,
  cycle TEXT NOT NULL,
  reading_id BIGINT NOT NULL,
  PRIMARY KEY (user_id, cycle)
);
CREATE TABLE IF NOT EXISTS rewards (
  user_id BIGINT PRIMARY KEY,
  granted_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS audit_log (
  id BIGSERIAL PRIMARY KEY,
  operator_id BIGINT NOT NULL,
  action TEXT NOT NULL,
  object_type TEXT NOT NULL,
  object_id TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS idempotency (
  scope TEXT NOT NULL,
  key TEXT NOT NULL,
  status TEXT NOT NULL,
  response TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (scope, key)
);
`)
	if err != nil {
		return err
	}
	_, err = s.Exec(ctx, `
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS wechat_openid TEXT;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS apple_sub TEXT;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS phone TEXT;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai';
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS tarotist_trial_used BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS tease_off BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE readings ADD COLUMN IF NOT EXISTS spread_name TEXT NOT NULL DEFAULT '';
ALTER TABLE readings ADD COLUMN IF NOT EXISTS business_date TEXT NOT NULL DEFAULT '';
ALTER TABLE readings ADD COLUMN IF NOT EXISTS skip_detect BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE readings ADD COLUMN IF NOT EXISTS reward_consumed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE tarotists ADD COLUMN IF NOT EXISTS voice TEXT NOT NULL DEFAULT '';
ALTER TABLE tarotists ADD COLUMN IF NOT EXISTS banned TEXT NOT NULL DEFAULT '';
ALTER TABLE tarotists ADD COLUMN IF NOT EXISTS ask_style TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS app_users_wechat_openid_key ON app_users(wechat_openid) WHERE wechat_openid IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS app_users_apple_sub_key ON app_users(apple_sub) WHERE apple_sub IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS app_users_phone_key ON app_users(phone) WHERE phone IS NOT NULL;
CREATE TABLE IF NOT EXISTS identity_claims (
  kind TEXT NOT NULL,
  value TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (kind, value)
);
CREATE TABLE IF NOT EXISTS reading_messages (
  id BIGSERIAL PRIMARY KEY,
  reading_id BIGINT NOT NULL,
  turn_id BIGINT NOT NULL,
  seq INT NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`)
	if err != nil {
		return err
	}
	// Account merge (确认合并并账). Paid pools are balance placeholders only — no prices.
	_, err = s.Exec(ctx, `
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS merged_into BIGINT;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS merged_at TIMESTAMPTZ;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS subscription_prompt_required BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS identity_prompt_required BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE rewards ADD COLUMN IF NOT EXISTS remaining INT;
UPDATE rewards SET remaining = CASE WHEN consumed_at IS NULL AND expires_at > now() THEN 1 ELSE 0 END WHERE remaining IS NULL;
ALTER TABLE rewards ALTER COLUMN remaining SET DEFAULT 1;
ALTER TABLE rewards ALTER COLUMN remaining SET NOT NULL;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS actor_kind TEXT NOT NULL DEFAULT 'operator';
CREATE TABLE IF NOT EXISTS paid_pools (
  user_id BIGINT PRIMARY KEY,
  subscription_remaining INT NOT NULL DEFAULT 0,
  pack_remaining INT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS subscriptions (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  sku_kind TEXT NOT NULL,
  status TEXT NOT NULL,
  starts_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  merge_state TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS subscriptions_user_idx ON subscriptions(user_id);
CREATE TABLE IF NOT EXISTS daily_free_balance (
  user_id BIGINT NOT NULL,
  cycle TEXT NOT NULL,
  used_flags INT NOT NULL,
  remaining INT NOT NULL,
  daily_slot_used INT NOT NULL DEFAULT 0,
  daily_slot_remaining INT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, cycle)
);
CREATE TABLE IF NOT EXISTS daily_free_uses (
  user_id BIGINT NOT NULL,
  cycle TEXT NOT NULL,
  reading_id BIGINT NOT NULL,
  source_user_id BIGINT NOT NULL,
  kind TEXT NOT NULL,
  PRIMARY KEY (kind, cycle, reading_id)
);
CREATE TABLE IF NOT EXISTS account_merge_requests (
  id BIGSERIAL PRIMARY KEY,
  primary_user_id BIGINT NOT NULL,
  secondary_user_id BIGINT NOT NULL,
  identity_kind TEXT NOT NULL,
  identity_value TEXT NOT NULL,
  email_password_hash TEXT,
  status TEXT NOT NULL,
  subscription_resolution TEXT NOT NULL DEFAULT '',
  confirm_at TIMESTAMPTZ,
  result TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS account_merge_pending_pair
  ON account_merge_requests(primary_user_id, secondary_user_id, identity_kind, identity_value)
  WHERE status = 'pending_confirm';
CREATE TABLE IF NOT EXISTS retained_identities (
  id BIGSERIAL PRIMARY KEY,
  merge_request_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  source_user_id BIGINT NOT NULL,
  kind TEXT NOT NULL,
  value TEXT NOT NULL,
  reason TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`)
	if err != nil {
		return err
	}
	// IAP diamond ledger. Price and diamond_grant stay NULL until configured.
	// Subscription diamonds and consumable-pack diamonds are separate buckets.
	// paid_pools remains the merge placeholder and is not an IAP credit target.
	_, err = s.Exec(ctx, `
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS sku_id BIGINT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS platform TEXT NOT NULL DEFAULT '';
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS store_original_transaction_id TEXT NOT NULL DEFAULT '';
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS auto_renew BOOLEAN;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS last_order_id BIGINT;
CREATE INDEX IF NOT EXISTS subscriptions_store_txn_idx ON subscriptions(user_id, platform, store_original_transaction_id);
CREATE TABLE IF NOT EXISTS iap_skus (
  id BIGSERIAL PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  ledger_bucket TEXT NOT NULL,
  display_name TEXT NOT NULL,
  app_store_product_id TEXT,
  play_product_id TEXT,
  price_amount_minor BIGINT,
  price_currency TEXT,
  diamond_grant INT,
  period_unit TEXT NOT NULL DEFAULT '',
  period_count INT,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iap_skus_kind_chk CHECK (kind IN ('month_subscription','year_subscription','consumable_pack')),
  CONSTRAINT iap_skus_bucket_chk CHECK (ledger_bucket IN ('subscription','consumable')),
  CONSTRAINT iap_skus_bucket_kind_chk CHECK (
    (kind IN ('month_subscription','year_subscription') AND ledger_bucket = 'subscription')
    OR (kind = 'consumable_pack' AND ledger_bucket = 'consumable')
  )
);
CREATE UNIQUE INDEX IF NOT EXISTS iap_skus_app_store_product_id
  ON iap_skus(app_store_product_id) WHERE app_store_product_id IS NOT NULL AND app_store_product_id <> '';
CREATE UNIQUE INDEX IF NOT EXISTS iap_skus_play_product_id
  ON iap_skus(play_product_id) WHERE play_product_id IS NOT NULL AND play_product_id <> '';
CREATE TABLE IF NOT EXISTS iap_settings (
  id INT PRIMARY KEY,
  restore_hook_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  restore_required_first_pack BOOLEAN NOT NULL DEFAULT FALSE,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iap_settings_singleton CHECK (id = 1)
);
CREATE TABLE IF NOT EXISTS iap_orders (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  sku_id BIGINT,
  sku_code TEXT NOT NULL DEFAULT '',
  sku_kind TEXT NOT NULL DEFAULT '',
  ledger_bucket TEXT NOT NULL DEFAULT '',
  platform TEXT NOT NULL,
  product_id TEXT NOT NULL DEFAULT '',
  store_transaction_id TEXT NOT NULL,
  original_transaction_id TEXT NOT NULL DEFAULT '',
  verify_status TEXT NOT NULL,
  credit_status TEXT NOT NULL,
  credited_diamonds INT,
  period_start TIMESTAMPTZ,
  period_end TIMESTAMPTZ,
  subscription_id BIGINT,
  source TEXT NOT NULL,
  verify_message TEXT NOT NULL DEFAULT '',
  receipt_sha256 TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  verified_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iap_orders_platform_chk CHECK (platform IN ('app_store','play')),
  CONSTRAINT iap_orders_verify_chk CHECK (verify_status IN ('pending','verified','failed')),
  CONSTRAINT iap_orders_credit_chk CHECK (credit_status IN ('none','credited','amount_unconfigured','skipped_expired')),
  CONSTRAINT iap_orders_source_chk CHECK (source IN ('verify','restore'))
);
CREATE UNIQUE INDEX IF NOT EXISTS iap_orders_platform_txn ON iap_orders(platform, store_transaction_id);
CREATE INDEX IF NOT EXISTS iap_orders_user_idx ON iap_orders(user_id, id DESC);
CREATE TABLE IF NOT EXISTS diamond_balances (
  user_id BIGINT PRIMARY KEY,
  subscription_diamonds INT NOT NULL DEFAULT 0,
  subscription_period_end TIMESTAMPTZ,
  subscription_period_key TEXT NOT NULL DEFAULT '',
  pack_diamonds INT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS diamond_ledger (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  bucket TEXT NOT NULL,
  delta INT NOT NULL,
  balance_after INT NOT NULL,
  order_id BIGINT,
  store_transaction_id TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT diamond_ledger_bucket_chk CHECK (bucket IN ('subscription','consumable'))
);
CREATE UNIQUE INDEX IF NOT EXISTS diamond_ledger_order_bucket
  ON diamond_ledger(order_id, bucket) WHERE order_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS diamond_ledger_user_idx ON diamond_ledger(user_id, id DESC);
`)
	return err
}

func (s *Store) seed(ctx context.Context) error {
	if err := s.ensureOperator(ctx, "admin", "admin123", "admin"); err != nil {
		return err
	}
	if err := s.ensureOperator(ctx, "readonly", "readonly123", "readonly"); err != nil {
		return err
	}
	var n int
	if err := s.QueryRow(ctx, `SELECT count(*) FROM spreads WHERE name='时间流'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		pos, _ := json.Marshal([]position{
			{Index: 1, Name: "过去", Meaning: "与当前问题相关的既往经历、背景或已经形成的影响"},
			{Index: 2, Name: "现在", Meaning: "当前处境、核心矛盾或主要影响因素"},
			{Index: 3, Name: "未来", Meaning: "当前发展趋势及潜在走向，不代表必然发生"},
		})
		_, err := s.Exec(ctx, `INSERT INTO spreads(name, description, card_count, positions, enabled, charged) VALUES ($1,$2,$3,$4,true,false)`,
			"时间流", "过去 / 现在 / 未来", 3, string(pos))
		if err != nil {
			return err
		}
	}
	personas := []struct{ name, persona, copy, voice, banned, ask string }{
		{"知晚", "先接住情绪，再讲牌。可以指出问题，但不羞辱。", "温和、慢一点", "语速偏慢，保留可能和倾向", "不说你活该", "先问感受，再问事实"},
		{"明序", "先给结构，再解释牌。也要接住情绪，但不拖。", "清楚、有步骤", "吐字清楚，限定词不省略", "不说绝对会发生", "先确认问题，再追问一个变量"},
		{"隐川", "话少，先回答原问题，再补牌面。", "短、留白", "声音低，不加重语气", "不编造用户没说的事实", "只追问一个必要缺口"},
		{"照野", "轻度犀利。一次回复最多一两句调侃，围绕用户明确说过的行为。用户说不舒服或别这样说，立即停止调侃。悲伤或危机时改成认真、平和。", "直，但不伤人", "调侃时也不丢掉限定词", "不贴标签，不连续讽刺", "把调侃拉回牌面"},
	}
	for _, p := range personas {
		var exists int
		if err := s.QueryRow(ctx, `SELECT count(*) FROM tarotists WHERE name=$1`, p.name).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		if _, err := s.Exec(ctx, `INSERT INTO tarotists(name, persona, display_copy, voice, banned, ask_style, enabled) VALUES ($1,$2,$3,$4,$5,$6,true)`,
			p.name, p.persona, p.copy, p.voice, p.banned, p.ask); err != nil {
			return err
		}
	}
	return s.seedIAP(ctx)
}

// seedIAP reserves month / year / consumable slots. Product ids, prices, and
// diamond grants stay empty; callers configure them later without invented numbers.
func (s *Store) seedIAP(ctx context.Context) error {
	slots := []struct {
		code, kind, bucket, name, period string
	}{
		{"sub_month", "month_subscription", "subscription", "月订阅", "month"},
		{"sub_year", "year_subscription", "subscription", "年订阅", "year"},
		{"pack_consumable", "consumable_pack", "consumable", "次卡 / 点数包", ""},
	}
	for _, slot := range slots {
		if _, err := s.Exec(ctx, `INSERT INTO iap_skus(code, kind, ledger_bucket, display_name, period_unit)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (code) DO NOTHING`, slot.code, slot.kind, slot.bucket, slot.name, slot.period); err != nil {
			return err
		}
	}
	_, err := s.Exec(ctx, `INSERT INTO iap_settings(id) VALUES (1) ON CONFLICT (id) DO NOTHING`)
	return err
}

func (s *Store) ensureOperator(ctx context.Context, user, pass, role string) error {
	var n int
	if err := s.QueryRow(ctx, `SELECT count(*) FROM operators WHERE username=$1`, user).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.Exec(ctx, `INSERT INTO operators(username, password_hash, role) VALUES ($1,$2,$3)`, user, string(hash), role)
	return err
}

func (s *Store) audit(ctx context.Context, operatorID int64, action, objectType, objectID, detail string) {
	_, _ = s.Exec(ctx, `INSERT INTO audit_log(operator_id, action, object_type, object_id, detail) VALUES ($1,$2,$3,$4,$5)`,
		operatorID, action, objectType, objectID, detail)
}

func (s *Store) encrypt(plain string) ([]byte, error) {
	block, err := aes.NewCipher(s.KEK)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plain), nil), nil
}

func (s *Store) decrypt(ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(s.KEK)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, body := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func cycleOf(now time.Time) string {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	t := now.In(loc)
	if t.Hour() < 8 {
		t = t.AddDate(0, 0, -1)
	}
	return t.Format("2006-01-02")
}

func cycleFor(now time.Time, tz string) string {
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return cycleOf(now)
	}
	t := now.In(loc)
	if t.Hour() < 8 {
		t = t.AddDate(0, 0, -1)
	}
	return t.Format("2006-01-02")
}

func nextBeijingEight(now time.Time) time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	t := now.In(loc)
	next := time.Date(t.Year(), t.Month(), t.Day()+1, 8, 0, 0, 0, loc)
	return next
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func fmtID(id int64) string { return fmt.Sprintf("%d", id) }

func toGFLink(dsn string) string {
	if strings.HasPrefix(dsn, "pgsql:") {
		return dsn
	}
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		return dsn
	}
	pass, _ := u.User.Password()
	extra := ""
	if u.RawQuery != "" {
		extra = "?" + u.RawQuery
	}
	return fmt.Sprintf("pgsql:%s:%s@tcp(%s)%s%s", u.User.Username(), pass, u.Host, u.Path, extra)
}
