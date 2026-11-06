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
	// Account merge (确认合并并账).
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
  reading_grant INT,
  period_unit TEXT NOT NULL DEFAULT '',
  period_count INT,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iap_skus_kind_chk CHECK (kind IN ('month_subscription','year_subscription','reading_pack','consumable_pack','yearly_unlock')),
  CONSTRAINT iap_skus_bucket_chk CHECK (ledger_bucket IN ('subscription','consumable')),
  CONSTRAINT iap_skus_bucket_kind_chk CHECK (
    (kind IN ('month_subscription','year_subscription') AND ledger_bucket = 'subscription')
    OR (kind IN ('consumable_pack','yearly_unlock','reading_pack') AND ledger_bucket = 'consumable')
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
  credited_readings INT,
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
	if err != nil {
		return err
	}
	// Shared quota. diamond_cost stays NULL until configured — never seed a price or a per-play amount.
	// Spend order is not stored here; quota.go always debits pack, then subscription.
	_, err = s.Exec(ctx, `
ALTER TABLE readings ADD COLUMN IF NOT EXISTS quota_settled BOOLEAN NOT NULL DEFAULT FALSE;
CREATE TABLE IF NOT EXISTS quota_settings (
  id INT PRIMARY KEY,
  diamond_cost INT,
  followup_limit INT DEFAULT 5,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT quota_settings_singleton CHECK (id = 1),
  CONSTRAINT quota_settings_cost_chk CHECK (diamond_cost IS NULL OR diamond_cost > 0),
  CONSTRAINT quota_settings_followup_chk CHECK (followup_limit IS NULL OR followup_limit > 0)
);
INSERT INTO quota_settings(id) VALUES (1) ON CONFLICT (id) DO NOTHING;
CREATE TABLE IF NOT EXISTS quota_charges (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  reading_id BIGINT NOT NULL,
  charge_key TEXT NOT NULL,
  entitlement TEXT NOT NULL,
  amount INT NOT NULL,
  bucket TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (reading_id, charge_key)
);
CREATE INDEX IF NOT EXISTS quota_charges_user_idx ON quota_charges(user_id, id DESC);
CREATE TABLE IF NOT EXISTS quota_paywall (
  reading_id BIGINT NOT NULL,
  charge_key TEXT NOT NULL,
  user_id BIGINT NOT NULL,
  status TEXT NOT NULL,
  resume_action TEXT NOT NULL,
  resume_payload TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (reading_id, charge_key),
  CONSTRAINT quota_paywall_status_chk CHECK (status IN ('pending','resumed','cancelled','unconfigured'))
);
`)
	if err != nil {
		return err
	}
	// Controlled admin grant. This is a new credit, not an edit of ledger rows or IAP orders.
	// Free counts land on rewards.admin_grant_remaining (still 次数). Diamonds add to
	// pack_diamonds by default (ledger bucket consumable) or subscription_diamonds.
	_, err = s.Exec(ctx, `
ALTER TABLE rewards ADD COLUMN IF NOT EXISTS admin_grant_remaining INT NOT NULL DEFAULT 0;
ALTER TABLE ledger ALTER COLUMN reading_id DROP NOT NULL;
ALTER TABLE ledger ADD COLUMN IF NOT EXISTS grant_id BIGINT;
ALTER TABLE diamond_ledger ADD COLUMN IF NOT EXISTS grant_id BIGINT;
CREATE TABLE IF NOT EXISTS admin_grants (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  operator_id BIGINT NOT NULL,
  kind TEXT NOT NULL,
  amount INT NOT NULL,
  bucket TEXT NOT NULL DEFAULT '',
  ledger_bucket TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  note TEXT NOT NULL,
  diamond_ledger_id BIGINT,
  count_ledger_id BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT admin_grants_kind_chk CHECK (kind IN ('diamonds','free_count')),
  CONSTRAINT admin_grants_bucket_chk CHECK (
    (kind = 'diamonds' AND bucket IN ('pack','subscription'))
    OR (kind = 'free_count' AND bucket = '')
  ),
  CONSTRAINT admin_grants_reason_chk CHECK (reason = 'admin_grant'),
  CONSTRAINT admin_grants_amount_chk CHECK (amount > 0)
);
CREATE INDEX IF NOT EXISTS admin_grants_user_idx ON admin_grants(user_id, id DESC);
`)
	if err != nil {
		return err
	}
	// App UI P0 shell. Diamonds stay the only currency ledger; 塔塔币 is display_label.
	_, err = s.Exec(ctx, `
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS bio TEXT NOT NULL DEFAULT '';
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS avatar_url TEXT NOT NULL DEFAULT '';
ALTER TABLE tarotists ADD COLUMN IF NOT EXISTS avatar_url TEXT NOT NULL DEFAULT '';
ALTER TABLE tarotists ADD COLUMN IF NOT EXISTS tags TEXT NOT NULL DEFAULT '[]';
ALTER TABLE tarotists ADD COLUMN IF NOT EXISTS blurb TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS explore_topics (
  topic_id TEXT PRIMARY KEY,
  group_key TEXT NOT NULL,
  group_title TEXT NOT NULL,
  title TEXT NOT NULL,
  subtitle TEXT NOT NULL DEFAULT '',
  illustration TEXT,
  spread_id BIGINT,
  sort_order INT NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS redeem_codes (
  id BIGSERIAL PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  amount INT NOT NULL,
  benefit_name TEXT NOT NULL,
  max_redemptions INT,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT redeem_codes_kind_chk CHECK (kind IN ('diamonds','free_count')),
  CONSTRAINT redeem_codes_amount_chk CHECK (amount > 0)
);
CREATE TABLE IF NOT EXISTS redeem_redemptions (
  id BIGSERIAL PRIMARY KEY,
  code_id BIGINT NOT NULL REFERENCES redeem_codes(id),
  user_id BIGINT NOT NULL,
  diamond_ledger_id BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (code_id, user_id)
);
CREATE INDEX IF NOT EXISTS redeem_redemptions_user_idx ON redeem_redemptions(user_id, id DESC);
CREATE TABLE IF NOT EXISTS wishlists (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  content TEXT NOT NULL,
  image_url TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS feedback (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  category TEXT NOT NULL,
  content TEXT NOT NULL,
  image_url TEXT NOT NULL DEFAULT '',
  email TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT feedback_category_chk CHECK (category IN ('使用问题','内容建议','其他'))
);
CREATE TABLE IF NOT EXISTS notification_preferences (
  user_id BIGINT PRIMARY KEY,
  daily_draw_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  daily_draw_time TEXT NOT NULL DEFAULT '08:00',
  tomorrow_draw_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  tomorrow_draw_time TEXT NOT NULL DEFAULT '08:00',
  messages_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 个推设备注册。cid 为客户端 SDK 回传的个推 CID，是服务端下发推送的目标。
-- 今日/明日抽牌提醒走 App 本地通知（notification_preferences），不经由此表。
CREATE TABLE IF NOT EXISTS push_devices (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  cid TEXT NOT NULL,
  platform TEXT NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT push_devices_uq UNIQUE (user_id, cid),
  CONSTRAINT push_devices_platform_chk CHECK (platform IN ('', 'ios', 'android'))
);
-- 站内信（优惠/活动等消息通知的服务端副本）。audience='all' 为广播，'user' 时 user_id 指定收件人。
CREATE TABLE IF NOT EXISTS app_messages (
  id BIGSERIAL PRIMARY KEY,
  audience TEXT NOT NULL DEFAULT 'all',
  user_id BIGINT,
  category TEXT NOT NULL DEFAULT 'campaign',
  title TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  target TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT app_messages_audience_chk CHECK (audience IN ('all', 'user'))
);
-- 站内信已读状态。一条广播被多个用户分别标记已读。
CREATE TABLE IF NOT EXISTS app_message_reads (
  message_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (message_id, user_id)
);
`)
	if err != nil {
		return err
	}
	// App UI P1. Campaign detail and FAQ bodies stay pending; row fields for
	// participations and rewards are not created until the placeholder screens confirm them.
	_, err = s.Exec(ctx, `
CREATE TABLE IF NOT EXISTS campaigns (
  campaign_id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  subtitle TEXT NOT NULL DEFAULT '',
  badge TEXT NOT NULL DEFAULT '',
  detail_title TEXT NOT NULL,
  detail_body TEXT NOT NULL,
  detail_status TEXT NOT NULL DEFAULT 'pending',
  status TEXT NOT NULL DEFAULT 'ongoing',
  sort_order INT NOT NULL DEFAULT 0,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  CONSTRAINT campaigns_status_chk CHECK (status IN ('ongoing','ended')),
  CONSTRAINT campaigns_detail_status_chk CHECK (detail_status IN ('pending','ready'))
);
CREATE TABLE IF NOT EXISTS help_categories (
  category_id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  articles_status TEXT NOT NULL DEFAULT 'ready',
  empty_note TEXT NOT NULL DEFAULT '',
  sort_order INT NOT NULL DEFAULT 0,
  CONSTRAINT help_categories_status_chk CHECK (articles_status IN ('ready','pending'))
);
CREATE TABLE IF NOT EXISTS help_articles (
  article_id TEXT PRIMARY KEY,
  category_id TEXT NOT NULL REFERENCES help_categories(category_id),
  title TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '待确认',
  body_status TEXT NOT NULL DEFAULT 'pending',
  sort_order INT NOT NULL DEFAULT 0,
  CONSTRAINT help_articles_body_status_chk CHECK (body_status IN ('pending','ready'))
);
CREATE INDEX IF NOT EXISTS help_articles_category_idx ON help_articles(category_id, sort_order);
`)
	if err != nil {
		return err
	}
	_, err = s.Exec(ctx, `
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS public_id TEXT;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS deck_code TEXT NOT NULL DEFAULT 'rider-waite';
CREATE UNIQUE INDEX IF NOT EXISTS app_users_public_id_key ON app_users(public_id) WHERE public_id IS NOT NULL AND public_id <> '';
CREATE TABLE IF NOT EXISTS monthly_report_saves (
  user_id BIGINT NOT NULL,
  month TEXT NOT NULL,
  saved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, month)
);
CREATE TABLE IF NOT EXISTS yearly_reports (
  user_id BIGINT NOT NULL,
  year INT NOT NULL,
  unlocked BOOLEAN NOT NULL DEFAULT FALSE,
  unlock_source TEXT NOT NULL DEFAULT '',
  unlocked_at TIMESTAMPTZ,
  PRIMARY KEY (user_id, year)
);
ALTER TABLE yearly_reports ADD COLUMN IF NOT EXISTS birth_date TEXT NOT NULL DEFAULT '';
ALTER TABLE yearly_reports ADD COLUMN IF NOT EXISTS gender TEXT NOT NULL DEFAULT '';
ALTER TABLE yearly_reports ADD COLUMN IF NOT EXISTS profile_at TIMESTAMPTZ;
ALTER TABLE yearly_reports ADD COLUMN IF NOT EXISTS rhythm_cards TEXT NOT NULL DEFAULT '';
ALTER TABLE yearly_reports ADD COLUMN IF NOT EXISTS domain_cards TEXT NOT NULL DEFAULT '';
ALTER TABLE yearly_reports ADD COLUMN IF NOT EXISTS report TEXT NOT NULL DEFAULT '';
ALTER TABLE yearly_reports ADD COLUMN IF NOT EXISTS report_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE yearly_reports ADD COLUMN IF NOT EXISTS fail_reason TEXT;
ALTER TABLE yearly_reports ADD COLUMN IF NOT EXISTS generated_at TIMESTAMPTZ;
ALTER TABLE yearly_reports DROP CONSTRAINT IF EXISTS yearly_reports_report_status_chk;
ALTER TABLE yearly_reports ADD CONSTRAINT yearly_reports_report_status_chk
  CHECK (report_status IN ('pending','generating','ready','failed'));
ALTER TABLE iap_skus ADD COLUMN IF NOT EXISTS reading_grant INT;
ALTER TABLE iap_orders ADD COLUMN IF NOT EXISTS credited_readings INT;
ALTER TABLE quota_settings ADD COLUMN IF NOT EXISTS followup_limit INT DEFAULT 5;
ALTER TABLE quota_settings DROP CONSTRAINT IF EXISTS quota_settings_followup_chk;
ALTER TABLE quota_settings ADD CONSTRAINT quota_settings_followup_chk
  CHECK (followup_limit IS NULL OR followup_limit > 0);
ALTER TABLE iap_skus DROP CONSTRAINT IF EXISTS iap_skus_kind_chk;
ALTER TABLE iap_skus DROP CONSTRAINT IF EXISTS iap_skus_bucket_kind_chk;
ALTER TABLE iap_skus ADD CONSTRAINT iap_skus_kind_chk
  CHECK (kind IN ('month_subscription','year_subscription','reading_pack','consumable_pack','yearly_unlock'));
ALTER TABLE iap_skus ADD CONSTRAINT iap_skus_bucket_kind_chk CHECK (
  (kind IN ('month_subscription','year_subscription') AND ledger_bucket = 'subscription')
  OR (kind IN ('consumable_pack','yearly_unlock','reading_pack') AND ledger_bucket = 'consumable')
);
DROP TABLE IF EXISTS daily_slot;
DROP TABLE IF EXISTS paid_pools;
ALTER TABLE iap_skus ADD COLUMN IF NOT EXISTS copy TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS app_config (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
DROP TABLE IF EXISTS copy_slots;
DROP TABLE IF EXISTS yearly_copy;
DROP TABLE IF EXISTS card_copy;
CREATE TABLE IF NOT EXISTS decks (
  code TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  thumbnail TEXT NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT true,
  sort_order INT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE decks ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
ALTER TABLE decks ADD COLUMN IF NOT EXISTS tags TEXT NOT NULL DEFAULT '';
INSERT INTO decks(code, name, enabled, sort_order) VALUES ('rider-waite', '莱德韦特', true, 0)
ON CONFLICT (code) DO NOTHING;
`)
	if err != nil {
		return err
	}
	return s.commentSchema(ctx)
}

func (s *Store) commentSchema(ctx context.Context) error {
	_, err := s.Exec(ctx, `
COMMENT ON TABLE operators IS '管理端账号。对应 /api/admin/v1/auth/*；role=admin 可写，readonly 只读。';
COMMENT ON COLUMN operators.role IS 'admin 或 readonly。';

COMMENT ON TABLE app_users IS 'App 用户。对应 /api/app/v1/auth/*、/profile。merged_into 非空表示已并入另一账号。';
COMMENT ON COLUMN app_users.public_id IS '对外短 ID，6 位。对应「我的」短码，首次读 profile 生成。';
COMMENT ON COLUMN app_users.phone IS '手机号。接口 account_display 优先脱敏手机，没有再退回邮箱。';
COMMENT ON COLUMN app_users.deck_code IS '当前牌面风格 code，须对应 decks 中已启用项。对应 GET/PUT /deck-preference。';
COMMENT ON COLUMN app_users.merged_into IS '并账后指向主账号 id。被合并账号不可再登录。';

COMMENT ON TABLE refresh_tokens IS '刷新令牌。App 登录不签发；管理端会话用。';
COMMENT ON TABLE identity_claims IS '已占用的登录身份（email/phone/wechat/apple），删号后仍占坑防抢注。';

COMMENT ON TABLE spreads IS '牌阵。对应 GET /spreads；抽牌 spread_id。种子含时间流等。';
COMMENT ON COLUMN spreads.positions IS 'JSON 牌位：index/name/meaning。';
COMMENT ON TABLE tarotists IS '塔罗师。对应 GET /tarotists；会员权益 benefits.tarotists.count 统计 enabled=true。';
COMMENT ON TABLE vendors IS '大模型供应商。api_key_enc 用 KEK 加密。';
COMMENT ON TABLE bindings IS '任务到供应商+模型的绑定。task=detect/rewrite/recommend/interpret/tarotist。';

COMMENT ON TABLE banners IS '运营 Banner。对应 GET /banners 与管理端 /admin/v1/banners。可触发个推。';
COMMENT ON TABLE campaigns IS '探索「参与活动」。对应 GET /campaigns；detail_status=pending 时正文未定稿。';
COMMENT ON TABLE explore_topics IS '探索主题占卜条目。对应 GET /explore/topics。';

COMMENT ON TABLE readings IS '一次解读。对应 /readings*。type=normal/tarotist/explore/daily。';
COMMENT ON COLUMN readings.type IS 'normal 普通、tarotist 占卜师、explore 主题、daily 今日一牌。';
COMMENT ON COLUMN readings.status IS '流程状态：QUESTION_CREATED → … → INTERPRETED / 付费墙等。';
COMMENT ON COLUMN readings.cards IS 'JSON 牌面。灵数抽牌 numbers 映射 Rider-Waite 后写入。';
COMMENT ON TABLE reading_messages IS '解读追问对话。对应 /readings/{id}/messages、followups。';

COMMENT ON TABLE monthly_report_saves IS '月度报告「已保存」标记。对应 POST /monthly-reports/save。统计仍现查 readings。';
COMMENT ON TABLE yearly_reports IS '年度运势。解锁后按定稿四步：资料 → 12 张全年节奏 → 6 张六大领域 → 生成报告。';
COMMENT ON COLUMN yearly_reports.unlock_source IS 'year_subscription 年卡免费，purchase 单独买。';
COMMENT ON COLUMN yearly_reports.rhythm_cards IS '12 张全年节奏牌 JSON。空表示未抽。';
COMMENT ON COLUMN yearly_reports.domain_cards IS '6 张六大领域牌 JSON（感情/事业/财运/健康/人际/成长）。空表示未抽。不与节奏牌重复。';
COMMENT ON COLUMN yearly_reports.report IS '模型生成的年度报告正文。未生成前为空，不编造。';
COMMENT ON COLUMN yearly_reports.report_status IS 'pending / generating / ready / failed。';

COMMENT ON TABLE iap_skus IS '内购价位，三类对应三块屏：会员（月/年/次卡 593:1905）、塔塔币 6 档（565:1904）、年运单独解锁（574:2423）。种子已填设计价与 App Store 占位商品号；管理员改过的价与商品号 COALESCE 保留。对应 GET /iap/skus 与管理端价位页。';
COMMENT ON COLUMN iap_skus.kind IS 'month_subscription 会员月卡；year_subscription 会员年卡；reading_pack 会员次卡；consumable_pack 塔塔币；yearly_unlock 年运解锁。';
COMMENT ON COLUMN iap_skus.ledger_bucket IS 'subscription=订阅钻石桶（月/年卡）；consumable=消耗桶（次卡/塔塔币/年运）。';
COMMENT ON COLUMN iap_skus.price_amount_minor IS '价格，单位分。种子已填；管理员可改。未配置为 null。';
COMMENT ON COLUMN iap_skus.diamond_grant IS '塔塔币/订阅发放钻石数。次卡与年运解锁必须为 null。未配置验单 credit_status=amount_unconfigured。';
COMMENT ON COLUMN iap_skus.reading_grant IS '仅次卡。解读次数，入账到 rewards.admin_grant_remaining。未配置为 null。';
COMMENT ON COLUMN iap_skus.period_unit IS '月卡 month、年卡 year；其余空。';
COMMENT ON COLUMN iap_skus.code IS '稳定编码。种子：sub_month/sub_year/sub_times（会员），pack_*（塔塔币档），unlock_yearly（年运）。';
COMMENT ON COLUMN iap_skus.app_store_product_id IS 'App Store 商品 ID。验单按票据 product_id 映射本列。种子为占位编码，上架前换成 App Store Connect 正式 ID。';

COMMENT ON TABLE iap_settings IS 'IAP 开关单行表 id=1。restore_hook_enabled 控制恢复购买。';
COMMENT ON TABLE iap_orders IS '本地验单订单。不请求苹果/谷歌，只按 product_id 映射 iap_skus。对应 POST /iap/verify、/iap/restore。';
COMMENT ON COLUMN iap_orders.sku_kind IS '下单时拷贝的 iap_skus.kind。';
COMMENT ON COLUMN iap_orders.credited_diamonds IS '本次入账钻石。未配置或非钻石商品为 null。';
COMMENT ON COLUMN iap_orders.credited_readings IS '次卡本次入账解读次数。未配置或非次卡为 null。';
COMMENT ON COLUMN iap_orders.credit_status IS 'none/credited/amount_unconfigured/skipped_expired。';

COMMENT ON TABLE subscriptions IS '商店订阅有效期。sku_kind 这里是 month/year（与 iap_skus.kind 不同）。年卡有效则年运免费解锁。';
COMMENT ON COLUMN subscriptions.sku_kind IS 'month 或 year，由 month_subscription/year_subscription 推导。';

COMMENT ON TABLE diamond_balances IS '塔塔币余额。subscription_diamonds 月/年卡周期桶，pack_diamonds 消耗包。扣费先 pack 后 subscription。';
COMMENT ON TABLE diamond_ledger IS '塔塔币流水。bucket=subscription|consumable。对应 GET /wallet/ledger。';

COMMENT ON TABLE rewards IS '免费次数账本。remaining=注册奖励；admin_grant_remaining=后台补发+次卡入账。';
COMMENT ON COLUMN rewards.remaining IS '注册/过期型免费次数。每日免费不在此列。';
COMMENT ON COLUMN rewards.admin_grant_remaining IS '可扣的解读次数：管理端补发、兑换码 free_count、次卡 reading_pack。';

COMMENT ON TABLE quota_settings IS '共用额度。id=1。diamond_cost 为单次解读钻石成本，未配置为 null。followup_limit 为每条解读可追问次数，null 表示不限制。';
COMMENT ON COLUMN quota_settings.followup_limit IS '每条解读追问上限。管理端 PUT /quota-params 可改；null 不限制。种子默认 5（设计稿）。';
COMMENT ON TABLE quota_charges IS '单次解读实际扣费记录。entitlement=daily_free/reward/admin_grant/diamond。';
COMMENT ON TABLE quota_paywall IS '付费墙暂停点。IAP 入账后可 resume。';
COMMENT ON TABLE quota_use IS '每日免费占用（user_id+cycle 唯一）。并账后写入 daily_free_*。';
COMMENT ON TABLE daily_free_balance IS '并账后的每日免费余额快照。';
COMMENT ON TABLE daily_free_uses IS '并账后每日免费消耗明细。';
COMMENT ON TABLE ledger IS '次数扣减流水。管理端 GET /admin/v1/ledger；IAP 钻石不写入此表。';

COMMENT ON TABLE admin_grants IS '管理端人工补发。kind=diamonds 入 diamond_*；free_count 入 rewards.admin_grant_remaining。';
COMMENT ON TABLE redeem_codes IS '兑换码。kind=diamonds|free_count。对应 POST /redeem。';
COMMENT ON TABLE redeem_redemptions IS '兑换码核销记录。对应 GET /redeem/records。';

COMMENT ON TABLE notification_preferences IS '通知三开关。今日/明日抽牌走本地通知；messages_enabled 控制优惠/活动推送。对应 /notification-preferences。';
COMMENT ON TABLE push_devices IS '个推 CID。优惠/活动服务端推送目标。今日/明日抽牌不经此表。对应 POST /push/register。';
COMMENT ON TABLE app_messages IS '站内信（优惠/活动）。audience=all 广播，user 定向。对应 GET /messages。';
COMMENT ON TABLE app_message_reads IS '站内信已读。对应 POST /messages/{id}/read。';

COMMENT ON TABLE help_categories IS '帮助中心分类。对应 GET /help/categories。';
COMMENT ON TABLE help_articles IS '帮助文章。body_status=pending 时正文为「待确认」。管理端 PUT /copy/help 可改。';
COMMENT ON COLUMN iap_skus.copy IS '价位附带运营文案。年卡用于专属活动权益；空则 benefits.campaign_perk.copy_status=pending。';
COMMENT ON TABLE app_config IS '无独立表的运营配置。key 如 yearly.2026.theme、card.愚者.yi、card.愚者.thumbnail、card.thumbnail_template（{id}/{name}）、card.thumbnail_default。缩略图都未配时 App 返回内置 /uploads/card-default.png。对应 GET/PUT /copy/{kind}。';
COMMENT ON TABLE decks IS '牌面风格。种子仅 rider-waite / 莱德韦特。预览图空则用内置默认牌背。介绍/标签空则 App 返回空串与空数组，不编造。管理端 PUT /copy/decks，App GET/PUT /deck-preference。';
COMMENT ON COLUMN decks.description IS '套装一句介绍。对应定稿 693:39。空为未填。';
COMMENT ON COLUMN decks.tags IS '套装标签 JSON 数组。空为未填。';
COMMENT ON TABLE wishlists IS '功能心愿单。对应 POST /wishlist。';
COMMENT ON TABLE feedback IS '反馈意见。category=使用问题/内容建议/其他。对应 POST /feedback。';

COMMENT ON TABLE account_merge_requests IS '确认合并并账请求。';
COMMENT ON TABLE retained_identities IS '并账后保留的登录身份。';
COMMENT ON TABLE audit_log IS '审计。operator 操作或 App iap_verify/restore。';
COMMENT ON TABLE idempotency IS '写接口幂等键。';
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
	if err := s.seedIAP(ctx); err != nil {
		return err
	}
	if err := s.seedExplore(ctx); err != nil {
		return err
	}
	if err := s.seedCampaigns(ctx); err != nil {
		return err
	}
	if err := s.seedHelp(ctx); err != nil {
		return err
	}
	return s.seedTarotistPublic(ctx)
}

func (s *Store) seedExplore(ctx context.Context) error {
	var spreadID sql.NullInt64
	_ = s.QueryRow(ctx, `SELECT id FROM spreads WHERE name='时间流' LIMIT 1`).Scan(&spreadID)
	topics := []struct {
		id, groupKey, groupTitle, title, subtitle string
		order                                     int
	}{
		{"yes_no", "now_choice", "此刻抉择", "是否", "一件事该不该做", 1},
		{"either_or", "now_choice", "此刻抉择", "二选一", "两个选项怎么选", 2},
		{"love", "theme", "主题占卜", "感情", "关系里正在发生的事", 10},
		{"career", "theme", "主题占卜", "事业", "工作与方向", 11},
		{"study", "theme", "主题占卜", "学业", "学习与考试", 12},
		{"wealth", "theme", "主题占卜", "财富", "钱与资源流向", 13},
		{"relation", "theme", "主题占卜", "人际", "沟通与边界", 14},
		{"self", "theme", "主题占卜", "自我", "当下的自己", 15},
		{"family", "theme", "主题占卜", "家庭", "家人与归属", 16},
		{"health", "theme", "主题占卜", "身心", "节奏与恢复", 17},
		{"general", "theme", "主题占卜", "综合", "最近一段时期", 18},
	}
	for _, t := range topics {
		if _, err := s.Exec(ctx, `INSERT INTO explore_topics(topic_id, group_key, group_title, title, subtitle, illustration, spread_id, sort_order)
			VALUES ($1,$2,$3,$4,$5,NULL,$6,$7)
			ON CONFLICT (topic_id) DO NOTHING`,
			t.id, t.groupKey, t.groupTitle, t.title, t.subtitle, spreadID, t.order); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seedCampaigns(ctx context.Context) error {
	campaigns := []struct {
		id, title, subtitle, badge, detailTitle, detailBody string
		order                                               int
	}{
		{
			"friend_invite",
			"邀请好友，一起探索",
			"分享塔塔，查看邀请进度与奖励",
			"好友邀请",
			"好友邀请",
			"详情页不在本次交付范围。奖励、条件与有效期以活动配置为准。",
			1,
		},
		{
			"membership_benefit",
			"参与活动，领取会员体验",
			"查看活动要求与对应权益",
			"",
			"会员福利",
			"详情页不在本次交付范围。不预写未确认的会员时长或领取门槛。",
			2,
		},
	}
	for _, c := range campaigns {
		if _, err := s.Exec(ctx, `INSERT INTO campaigns(campaign_id, title, subtitle, badge, detail_title, detail_body, detail_status, status, sort_order, enabled)
			VALUES ($1,$2,$3,$4,$5,$6,'pending','ongoing',$7,true)
			ON CONFLICT (campaign_id) DO NOTHING`,
			c.id, c.title, c.subtitle, c.badge, c.detailTitle, c.detailBody, c.order); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seedHelp(ctx context.Context) error {
	categories := []struct {
		id, title, status, note string
		order                   int
	}{
		{"reading", "使用与占卜", "ready", "", 1},
		{"membership_pay", "会员与支付", "pending", "这一类的问题还在整理，条目待确认。", 2},
		{"account", "账号与设置", "ready", "", 3},
	}
	for _, c := range categories {
		if _, err := s.Exec(ctx, `INSERT INTO help_categories(category_id, title, articles_status, empty_note, sort_order)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (category_id) DO NOTHING`,
			c.id, c.title, c.status, c.note, c.order); err != nil {
			return err
		}
	}
	articles := []struct {
		id, category, title string
		order               int
	}{
		{"how_to_start", "reading", "如何开始一次塔罗占卜？", 1},
		{"where_records", "reading", "在哪里查看占卜记录？", 2},
		{"change_deck", "reading", "如何更换使用的牌组？", 3},
		{"draw_failed", "reading", "抽牌或解读失败怎么办？", 4},
		{"annual_entry", "reading", "年度运势在哪里开启？", 5},
		{"language_theme", "account", "如何切换语言和主题？", 1},
	}
	for _, a := range articles {
		if _, err := s.Exec(ctx, `INSERT INTO help_articles(article_id, category_id, title, body, body_status, sort_order)
			VALUES ($1,$2,$3,'待确认','pending',$4)
			ON CONFLICT (article_id) DO NOTHING`,
			a.id, a.category, a.title, a.order); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seedTarotistPublic(ctx context.Context) error {
	rows, err := s.Query(ctx, `SELECT id, display_copy, tags, blurb FROM tarotists WHERE enabled=true`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		id                int64
		copy, tags, blurb string
	}
	list := []row{}
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.id, &item.copy, &item.tags, &item.blurb); err != nil {
			return err
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range list {
		tags := item.tags
		if tags == "" || tags == "[]" {
			parts := []string{}
			for _, p := range strings.Split(item.copy, "、") {
				p = strings.TrimSpace(p)
				if p != "" {
					parts = append(parts, p)
				}
			}
			raw, _ := json.Marshal(parts)
			tags = string(raw)
		}
		blurb := item.blurb
		if blurb == "" {
			blurb = item.copy
		}
		if _, err := s.Exec(ctx, `UPDATE tarotists SET tags=$1, blurb=$2 WHERE id=$3`, tags, blurb, item.id); err != nil {
			return err
		}
	}
	return nil
}

// seedIAP fills the three price families from the UI. Existing admin-set prices
// and App Store IDs are kept (COALESCE). Apple IDs are placeholders until Connect.
func (s *Store) seedIAP(ctx context.Context) error {
	type sku struct {
		code, kind, bucket, name, period, apple string
		price                                   int64
		grant, readings, periodCount            any
	}
	slots := []sku{
		{"sub_month", "month_subscription", "subscription", "月卡", "month", "tata.membership.month", 3900, nil, nil, 1},
		{"sub_year", "year_subscription", "subscription", "年卡", "year", "tata.membership.year", 19800, nil, nil, 1},
		{"sub_times", "reading_pack", "consumable", "次卡", "", "tata.membership.times", 1990, nil, 5, nil},
		{"pack_60", "consumable_pack", "consumable", "60 塔塔币", "", "tata.coin.60", 600, 60, nil, nil},
		{"pack_180", "consumable_pack", "consumable", "180 塔塔币", "", "tata.coin.180", 1800, 180, nil, nil},
		{"pack_300", "consumable_pack", "consumable", "300 塔塔币", "", "tata.coin.300", 3000, 300, nil, nil},
		{"pack_680", "consumable_pack", "consumable", "680 塔塔币", "", "tata.coin.680", 6800, 680, nil, nil},
		{"pack_1280", "consumable_pack", "consumable", "1280 塔塔币", "", "tata.coin.1280", 12800, 1280, nil, nil},
		{"pack_3280", "consumable_pack", "consumable", "3280 塔塔币", "", "tata.coin.3280", 32800, 3280, nil, nil},
		{"unlock_yearly", "yearly_unlock", "consumable", "年运解锁", "", "tata.unlock.yearly", 2880, nil, nil, nil},
	}
	const upsert = `INSERT INTO iap_skus(
		code, kind, ledger_bucket, display_name, period_unit, period_count,
		price_amount_minor, price_currency, diamond_grant, reading_grant, app_store_product_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'CNY',$8,$9,$10)
		ON CONFLICT (code) DO UPDATE SET
			display_name=EXCLUDED.display_name,
			period_unit=EXCLUDED.period_unit,
			period_count=COALESCE(iap_skus.period_count, EXCLUDED.period_count),
			price_amount_minor=COALESCE(iap_skus.price_amount_minor, EXCLUDED.price_amount_minor),
			price_currency=COALESCE(iap_skus.price_currency, EXCLUDED.price_currency),
			diamond_grant=COALESCE(iap_skus.diamond_grant, EXCLUDED.diamond_grant),
			reading_grant=COALESCE(iap_skus.reading_grant, EXCLUDED.reading_grant),
			app_store_product_id=COALESCE(NULLIF(iap_skus.app_store_product_id,''), EXCLUDED.app_store_product_id),
			updated_at=now()`
	for _, slot := range slots {
		if _, err := s.Exec(ctx, upsert, slot.code, slot.kind, slot.bucket, slot.name, slot.period,
			slot.periodCount, slot.price, slot.grant, slot.readings, slot.apple); err != nil {
			return err
		}
	}
	if _, err := s.Exec(ctx, `INSERT INTO iap_skus(code, kind, ledger_bucket, display_name, period_unit)
		VALUES ('pack_consumable','consumable_pack','consumable','点数包','')
		ON CONFLICT (code) DO NOTHING`); err != nil {
		return err
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
