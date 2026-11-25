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
	if err := s.renameDiamondSchema(ctx); err != nil {
		return err
	}
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
  reading_id BIGINT NOT NULL
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
UPDATE app_users SET timezone='Asia/Shanghai' WHERE timezone IS NULL OR timezone='';
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS birth_time TEXT NOT NULL DEFAULT '';
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS birth_place TEXT NOT NULL DEFAULT '';
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS tarotist_trial_used BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS reading_trial_used BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE app_users u SET reading_trial_used=TRUE
WHERE reading_trial_used=FALSE AND (
  EXISTS (SELECT 1 FROM device_experiences d WHERE d.user_id=u.id AND d.consumed)
  OR EXISTS (SELECT 1 FROM quota_charges c WHERE c.user_id=u.id AND c.entitlement='onboard_trial')
);
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS tease_off BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE readings ADD COLUMN IF NOT EXISTS spread_name TEXT NOT NULL DEFAULT '';
ALTER TABLE readings ADD COLUMN IF NOT EXISTS business_date TEXT NOT NULL DEFAULT '';
ALTER TABLE readings ADD COLUMN IF NOT EXISTS skip_detect BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE readings ADD COLUMN IF NOT EXISTS reward_consumed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE readings ADD COLUMN IF NOT EXISTS numerology_map TEXT NOT NULL DEFAULT '';
ALTER TABLE readings ADD COLUMN IF NOT EXISTS draw_mode TEXT NOT NULL DEFAULT '';
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
	_, err = s.Exec(ctx, `
ALTER TABLE reading_messages ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT '';
ALTER TABLE reading_messages ADD COLUMN IF NOT EXISTS style TEXT NOT NULL DEFAULT '';
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
	// Paid catalog. Membership, quota packs, yearly unlock, and decks share iap_skus.
	// Fulfillment stays split across subscriptions, quota_balances, yearly_reports, user_deck_unlocks.
	_, err = s.Exec(ctx, `
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS sku_id BIGINT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS platform TEXT NOT NULL DEFAULT '';
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS store_original_transaction_id TEXT NOT NULL DEFAULT '';
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS auto_renew BOOLEAN;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS last_order_id BIGINT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS pending_expires_at TIMESTAMPTZ;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS pending_quota INT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS pending_order_id BIGINT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS pending_sku_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE iap_orders DROP CONSTRAINT IF EXISTS iap_orders_credit_chk;
ALTER TABLE iap_orders ADD CONSTRAINT iap_orders_credit_chk
  CHECK (credit_status IN ('none','credited','amount_unconfigured','skipped_expired','deferred'));
CREATE INDEX IF NOT EXISTS subscriptions_store_txn_idx ON subscriptions(user_id, platform, store_original_transaction_id);
CREATE TABLE IF NOT EXISTS iap_skus (
  id BIGSERIAL PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  display_name TEXT NOT NULL,
  app_store_product_id TEXT,
  play_product_id TEXT,
  price_amount_minor BIGINT,
  price_currency TEXT,
  quota_grant INT,
  period_unit TEXT NOT NULL DEFAULT '',
  period_count INT,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  deck_codes TEXT NOT NULL DEFAULT '[]',
  covers_future BOOLEAN NOT NULL DEFAULT FALSE,
  deduct_owned BOOLEAN NOT NULL DEFAULT FALSE,
  note TEXT NOT NULL DEFAULT '',
  sort_order INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iap_skus_kind_chk CHECK (kind IN (
    'month_subscription','year_subscription','quota_pack','yearly_unlock',
    'deck_year','deck_bundle','deck_single','deck_addon'
  ))
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
  platform TEXT NOT NULL,
  product_id TEXT NOT NULL DEFAULT '',
  store_transaction_id TEXT NOT NULL,
  original_transaction_id TEXT NOT NULL DEFAULT '',
  verify_status TEXT NOT NULL,
  credit_status TEXT NOT NULL,
  credited_quota INT,
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
  CONSTRAINT iap_orders_credit_chk CHECK (credit_status IN ('none','credited','amount_unconfigured','skipped_expired','deferred')),
  CONSTRAINT iap_orders_source_chk CHECK (source IN ('verify','restore'))
);
CREATE UNIQUE INDEX IF NOT EXISTS iap_orders_platform_txn ON iap_orders(platform, store_transaction_id);
CREATE INDEX IF NOT EXISTS iap_orders_user_idx ON iap_orders(user_id, id DESC);
CREATE TABLE IF NOT EXISTS quota_balances (
  user_id BIGINT PRIMARY KEY,
  quota_balance INT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS quota_lots (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  source TEXT NOT NULL,
  total INT NOT NULL,
  remaining INT NOT NULL,
  granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ,
  order_id BIGINT,
  reason TEXT NOT NULL DEFAULT '',
  CONSTRAINT quota_lots_source_chk CHECK (source IN ('subscription','pack','admin')),
  CONSTRAINT quota_lots_amount_chk CHECK (total > 0 AND remaining >= 0 AND remaining <= total)
);
CREATE INDEX IF NOT EXISTS quota_lots_user_idx ON quota_lots(user_id, expires_at, id);
CREATE TABLE IF NOT EXISTS quota_ledger (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  bucket TEXT NOT NULL,
  delta INT NOT NULL,
  balance_after INT NOT NULL,
  order_id BIGINT,
  store_transaction_id TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT quota_ledger_bucket_chk CHECK (bucket = 'quota')
);
CREATE UNIQUE INDEX IF NOT EXISTS quota_ledger_order_bucket
  ON quota_ledger(order_id, bucket) WHERE order_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS quota_ledger_user_idx ON quota_ledger(user_id, id DESC);
`)
	if err != nil {
		return err
	}
	// Shared quota. quota_cost stays NULL until configured — never seed a price or a per-play amount.
	// Spend order is not stored here; quota.go always debits pack, then subscription.
	_, err = s.Exec(ctx, `
ALTER TABLE readings ADD COLUMN IF NOT EXISTS quota_settled BOOLEAN NOT NULL DEFAULT FALSE;
CREATE TABLE IF NOT EXISTS quota_settings (
  id INT PRIMARY KEY,
  quota_cost INT,
  followup_limit INT DEFAULT 5,
  daily_free_count INT NOT NULL DEFAULT 1,
  daily_slot_count INT NOT NULL DEFAULT 1,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT quota_settings_singleton CHECK (id = 1),
  CONSTRAINT quota_settings_cost_chk CHECK (quota_cost IS NULL OR quota_cost > 0),
  CONSTRAINT quota_settings_followup_chk CHECK (followup_limit IS NULL OR followup_limit > 0),
  CONSTRAINT quota_settings_daily_free_chk CHECK (daily_free_count > 0),
  CONSTRAINT quota_settings_daily_slot_chk CHECK (daily_slot_count > 0)
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
	// Free counts land on rewards.admin_grant_remaining. Quota credit quota_balance.
	_, err = s.Exec(ctx, `
ALTER TABLE rewards ADD COLUMN IF NOT EXISTS admin_grant_remaining INT NOT NULL DEFAULT 0;
ALTER TABLE ledger ALTER COLUMN reading_id DROP NOT NULL;
ALTER TABLE ledger ADD COLUMN IF NOT EXISTS grant_id BIGINT;
CREATE TABLE IF NOT EXISTS ledger_lot_draws (
  id BIGSERIAL PRIMARY KEY,
  ledger_id BIGINT NOT NULL,
  lot_id BIGINT,
  source TEXT NOT NULL,
  amount INT NOT NULL,
  remaining_after INT NOT NULL,
  expires_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS ledger_lot_draws_ledger_idx ON ledger_lot_draws(ledger_id, id);
ALTER TABLE quota_ledger ADD COLUMN IF NOT EXISTS grant_id BIGINT;
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
  quota_ledger_id BIGINT,
  count_ledger_id BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT admin_grants_kind_chk CHECK (kind IN ('quota','free_count')),
  CONSTRAINT admin_grants_bucket_chk CHECK (
    (kind = 'quota' AND bucket IN ('pack','subscription'))
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
	// App UI P0 shell. Display label is 额度.
	_, err = s.Exec(ctx, `
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS bio TEXT NOT NULL DEFAULT '';
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS avatar_url TEXT NOT NULL DEFAULT '';
ALTER TABLE tarotists ADD COLUMN IF NOT EXISTS avatar_url TEXT NOT NULL DEFAULT '';
ALTER TABLE tarotists ADD COLUMN IF NOT EXISTS tags TEXT NOT NULL DEFAULT '[]';
ALTER TABLE tarotists ADD COLUMN IF NOT EXISTS blurb TEXT NOT NULL DEFAULT '';
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
  CONSTRAINT redeem_codes_kind_chk CHECK (kind IN ('quota','free_count')),
  CONSTRAINT redeem_codes_amount_chk CHECK (amount > 0)
);
CREATE TABLE IF NOT EXISTS redeem_redemptions (
  id BIGSERIAL PRIMARY KEY,
  code_id BIGINT NOT NULL REFERENCES redeem_codes(id),
  user_id BIGINT NOT NULL,
  quota_ledger_id BIGINT,
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
ALTER TABLE yearly_reports ADD COLUMN IF NOT EXISTS birth_time TEXT NOT NULL DEFAULT '';
ALTER TABLE yearly_reports ADD COLUMN IF NOT EXISTS birth_place TEXT NOT NULL DEFAULT '';
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
ALTER TABLE quota_settings ADD COLUMN IF NOT EXISTS followup_limit INT DEFAULT 5;
ALTER TABLE quota_settings DROP CONSTRAINT IF EXISTS quota_settings_followup_chk;
ALTER TABLE quota_settings ADD CONSTRAINT quota_settings_followup_chk
  CHECK (followup_limit IS NULL OR followup_limit > 0);
ALTER TABLE quota_settings ADD COLUMN IF NOT EXISTS daily_free_count INT NOT NULL DEFAULT 1;
ALTER TABLE quota_settings ADD COLUMN IF NOT EXISTS daily_slot_count INT NOT NULL DEFAULT 1;
ALTER TABLE quota_settings DROP CONSTRAINT IF EXISTS quota_settings_daily_free_chk;
ALTER TABLE quota_settings ADD CONSTRAINT quota_settings_daily_free_chk CHECK (daily_free_count > 0);
ALTER TABLE quota_settings DROP CONSTRAINT IF EXISTS quota_settings_daily_slot_chk;
ALTER TABLE quota_settings ADD CONSTRAINT quota_settings_daily_slot_chk CHECK (daily_slot_count > 0);
ALTER TABLE quota_settings ADD COLUMN IF NOT EXISTS registration_grant_delay_minutes INT;
ALTER TABLE quota_settings ADD COLUMN IF NOT EXISTS registration_valid_days INT NOT NULL DEFAULT 3;
ALTER TABLE quota_settings ADD COLUMN IF NOT EXISTS registration_grant_count INT NOT NULL DEFAULT 1;
ALTER TABLE quota_settings ADD COLUMN IF NOT EXISTS registration_spread_ids TEXT NOT NULL DEFAULT '[]';
ALTER TABLE quota_settings DROP CONSTRAINT IF EXISTS quota_settings_reg_delay_chk;
ALTER TABLE quota_settings ADD CONSTRAINT quota_settings_reg_delay_chk
  CHECK (registration_grant_delay_minutes IS NULL OR registration_grant_delay_minutes >= 0);
ALTER TABLE quota_settings DROP CONSTRAINT IF EXISTS quota_settings_reg_days_chk;
ALTER TABLE quota_settings ADD CONSTRAINT quota_settings_reg_days_chk CHECK (registration_valid_days > 0);
ALTER TABLE quota_settings DROP CONSTRAINT IF EXISTS quota_settings_reg_count_chk;
ALTER TABLE quota_settings ADD CONSTRAINT quota_settings_reg_count_chk CHECK (registration_grant_count > 0);
ALTER TABLE quota_settings ADD COLUMN IF NOT EXISTS tokens_per_quota INT;
ALTER TABLE quota_settings DROP CONSTRAINT IF EXISTS quota_settings_tokens_chk;
ALTER TABLE quota_settings ADD CONSTRAINT quota_settings_tokens_chk
  CHECK (tokens_per_quota IS NULL OR tokens_per_quota > 0);
CREATE TABLE IF NOT EXISTS user_devices (
  user_id BIGINT NOT NULL,
  device_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, device_id)
);
CREATE INDEX IF NOT EXISTS user_devices_device_idx ON user_devices(device_id);
CREATE TABLE IF NOT EXISTS registration_gifts (
  user_id BIGINT PRIMARY KEY,
  registered_at TIMESTAMPTZ NOT NULL,
  granted_at TIMESTAMPTZ,
  usable_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  total INT NOT NULL DEFAULT 0,
  remaining INT NOT NULL DEFAULT 0,
  used_at TIMESTAMPTZ,
  reward_blocked BOOLEAN NOT NULL DEFAULT FALSE
);
ALTER TABLE registration_gifts ADD COLUMN IF NOT EXISTS reward_blocked BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE quota_use DROP CONSTRAINT IF EXISTS quota_use_pkey;
CREATE UNIQUE INDEX IF NOT EXISTS quota_use_reading_uidx ON quota_use(reading_id);
CREATE INDEX IF NOT EXISTS quota_use_user_cycle_idx ON quota_use(user_id, cycle);
ALTER TABLE iap_skus DROP CONSTRAINT IF EXISTS iap_skus_kind_chk;
ALTER TABLE iap_skus DROP CONSTRAINT IF EXISTS iap_skus_bucket_chk;
ALTER TABLE iap_skus DROP CONSTRAINT IF EXISTS iap_skus_bucket_kind_chk;
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
ALTER TABLE decks ADD COLUMN IF NOT EXISTS unlock_required BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE decks ADD COLUMN IF NOT EXISTS back_url TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS deck_faces (
  deck_code TEXT NOT NULL REFERENCES decks(code) ON DELETE CASCADE,
  card_id INT NOT NULL,
  image_url TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (deck_code, card_id),
  CONSTRAINT deck_faces_card_chk CHECK (card_id >= 0 AND card_id < 78)
);
CREATE TABLE IF NOT EXISTS deck_products (
  code TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  name TEXT NOT NULL,
  price_amount_minor INT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'CNY',
  deck_codes TEXT NOT NULL DEFAULT '[]',
  covers_future BOOLEAN NOT NULL DEFAULT FALSE,
  deduct_owned BOOLEAN NOT NULL DEFAULT FALSE,
  note TEXT NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  sort_order INT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT deck_products_kind_chk CHECK (kind IN ('deck_year','deck_bundle','deck_single','deck_addon'))
);
CREATE TABLE IF NOT EXISTS user_deck_unlocks (
  user_id BIGINT NOT NULL,
  deck_code TEXT NOT NULL,
  source TEXT NOT NULL,
  product_code TEXT NOT NULL DEFAULT '',
  ends_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, deck_code)
);
ALTER TABLE spreads ADD COLUMN IF NOT EXISTS recommended BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE spreads ADD COLUMN IF NOT EXISTS thumbnail TEXT NOT NULL DEFAULT '';
INSERT INTO decks(code, name, enabled, sort_order) VALUES ('rider-waite', '莱德韦特', true, 0)
ON CONFLICT (code) DO NOTHING;
CREATE TABLE IF NOT EXISTS cards (
  id INT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  name_zh_hant TEXT NOT NULL DEFAULT '',
  name_en TEXT NOT NULL DEFAULT '',
  name_ja TEXT NOT NULL DEFAULT '',
  name_ko TEXT NOT NULL DEFAULT '',
  arcana TEXT NOT NULL DEFAULT '',
  suit TEXT NOT NULL DEFAULT '',
  rank TEXT NOT NULL DEFAULT '',
  yi TEXT NOT NULL DEFAULT '',
  thumbnail TEXT NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  sort_order INT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE cards ADD COLUMN IF NOT EXISTS upright_message TEXT NOT NULL DEFAULT '';
ALTER TABLE cards ADD COLUMN IF NOT EXISTS upright_yi TEXT NOT NULL DEFAULT '';
ALTER TABLE cards ADD COLUMN IF NOT EXISTS reversed_message TEXT NOT NULL DEFAULT '';
ALTER TABLE cards ADD COLUMN IF NOT EXISTS reversed_yi TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS ai_jobs (
  id BIGSERIAL PRIMARY KEY,
  kind TEXT NOT NULL,
  reading_id BIGINT,
  yearly_year INT,
  user_id BIGINT,
  followup_turn INT,
  status TEXT NOT NULL,
  partial_text TEXT NOT NULL DEFAULT '',
  result_text TEXT,
  error_text TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS ai_jobs_reading_kind_idx ON ai_jobs (reading_id, kind, id DESC);
CREATE INDEX IF NOT EXISTS ai_jobs_yearly_idx ON ai_jobs (user_id, yearly_year, kind, id DESC);
CREATE TABLE IF NOT EXISTS preset_questions (
  question_id TEXT PRIMARY KEY,
  pool TEXT NOT NULL,
  category TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  enabled BOOLEAN NOT NULL DEFAULT TRUE
);
ALTER TABLE preset_questions ADD COLUMN IF NOT EXISTS scene TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS preset_questions_scene_idx ON preset_questions (scene, pool, sort_order);
CREATE TABLE IF NOT EXISTS operator_module_perms (
  operator_id BIGINT NOT NULL REFERENCES operators(id) ON DELETE CASCADE,
  module TEXT NOT NULL,
  can_write BOOLEAN NOT NULL DEFAULT FALSE,
  PRIMARY KEY (operator_id, module)
);
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS explore_preferences TEXT NOT NULL DEFAULT '[]';
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS custom_deck_id BIGINT;
ALTER TABLE spreads ADD COLUMN IF NOT EXISTS name_en TEXT NOT NULL DEFAULT '';
ALTER TABLE spreads ADD COLUMN IF NOT EXISTS theme_key TEXT NOT NULL DEFAULT '';
ALTER TABLE spreads ADD COLUMN IF NOT EXISTS theme_title TEXT NOT NULL DEFAULT '';
ALTER TABLE spreads ADD COLUMN IF NOT EXISTS theme_subtitle TEXT NOT NULL DEFAULT '';
ALTER TABLE spreads ADD COLUMN IF NOT EXISTS short_sentence TEXT NOT NULL DEFAULT '';
ALTER TABLE spreads ADD COLUMN IF NOT EXISTS depth_group TEXT NOT NULL DEFAULT '';
ALTER TABLE spreads ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 1000;
ALTER TABLE spreads ADD COLUMN IF NOT EXISTS quota_cost INT;
ALTER TABLE spreads ADD COLUMN IF NOT EXISTS quick_questions TEXT NOT NULL DEFAULT '[]';
ALTER TABLE readings ADD COLUMN IF NOT EXISTS recommended_spread_id BIGINT;
ALTER TABLE readings ADD COLUMN IF NOT EXISTS spread_selection TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS invite_attributions (
  device_id TEXT PRIMARY KEY,
  invite_code TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS invite_rewards (
  id BIGSERIAL PRIMARY KEY,
  inviter_id BIGINT NOT NULL,
  invitee_id BIGINT NOT NULL UNIQUE,
  invite_code TEXT NOT NULL,
  invitee_name TEXT NOT NULL DEFAULT '',
  invitee_avatar TEXT NOT NULL DEFAULT '',
  amount INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE invite_rewards ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'code';
CREATE TABLE IF NOT EXISTS invite_grants (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  relationship_id BIGINT NOT NULL,
  role TEXT NOT NULL,
  total INT NOT NULL,
  remaining INT NOT NULL,
  granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  UNIQUE (relationship_id, role)
);
CREATE INDEX IF NOT EXISTS invite_grants_user_idx ON invite_grants(user_id, expires_at, id);
CREATE INDEX IF NOT EXISTS invite_rewards_inviter_idx ON invite_rewards(inviter_id, id DESC);
CREATE TABLE IF NOT EXISTS custom_decks (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  name TEXT NOT NULL DEFAULT '我的自定义牌组',
  status TEXT NOT NULL DEFAULT 'draft',
  face_source TEXT NOT NULL DEFAULT '',
  back_source TEXT NOT NULL DEFAULT '',
  back_url TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT custom_decks_status_chk CHECK (status IN ('draft','saved'))
);
CREATE INDEX IF NOT EXISTS custom_decks_user_idx ON custom_decks(user_id, id DESC);
CREATE TABLE IF NOT EXISTS custom_deck_faces (
  deck_id BIGINT NOT NULL REFERENCES custom_decks(id) ON DELETE CASCADE,
  card_id INT NOT NULL,
  image_url TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (deck_id, card_id),
  CONSTRAINT custom_deck_faces_card_chk CHECK (card_id >= 0 AND card_id < 78)
);
ALTER TABLE quota_balances ADD COLUMN IF NOT EXISTS quota_balance INT NOT NULL DEFAULT 0;
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'quota_balances' AND column_name = 'pack_diamonds'
  ) THEN
    UPDATE quota_balances
       SET quota_balance = pack_diamonds + subscription_diamonds
     WHERE quota_balance = 0 AND (pack_diamonds > 0 OR subscription_diamonds > 0);
  END IF;
END $$;
DO $$
DECLARE def text;
BEGIN
  SELECT pg_get_constraintdef(oid) INTO def FROM pg_constraint WHERE conname = 'quota_ledger_bucket_chk';
  IF def IS NULL THEN
    ALTER TABLE quota_ledger ADD CONSTRAINT quota_ledger_bucket_chk
      CHECK (bucket IN ('subscription','consumable','quota'));
  ELSIF position('quota' in def) = 0 THEN
    ALTER TABLE quota_ledger DROP CONSTRAINT quota_ledger_bucket_chk;
    ALTER TABLE quota_ledger ADD CONSTRAINT quota_ledger_bucket_chk
      CHECK (bucket IN ('subscription','consumable','quota'));
  END IF;
END $$;
ALTER TABLE rewards ADD COLUMN IF NOT EXISTS invite_remaining INT NOT NULL DEFAULT 0;
ALTER TABLE readings ADD COLUMN IF NOT EXISTS onboard_experience BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE readings ADD COLUMN IF NOT EXISTS device_id TEXT NOT NULL DEFAULT '';
ALTER TABLE readings ADD COLUMN IF NOT EXISTS theme_key TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS device_experiences (
  device_id TEXT PRIMARY KEY,
  user_id BIGINT,
  reading_id BIGINT,
  consumed BOOLEAN NOT NULL DEFAULT FALSE,
  consumed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE device_experiences ADD COLUMN IF NOT EXISTS guide_completed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE device_experiences ADD COLUMN IF NOT EXISTS gender TEXT NOT NULL DEFAULT '';
ALTER TABLE device_experiences ADD COLUMN IF NOT EXISTS birth_date TEXT NOT NULL DEFAULT '';
ALTER TABLE device_experiences ADD COLUMN IF NOT EXISTS explore_preferences TEXT NOT NULL DEFAULT '[]';
ALTER TABLE device_experiences ADD COLUMN IF NOT EXISTS trial_forfeited BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE device_experiences SET guide_completed=TRUE WHERE consumed=TRUE AND guide_completed=FALSE;
DROP INDEX IF EXISTS device_experiences_anon_idx;
ALTER TABLE readings DROP COLUMN IF EXISTS anonymous_id;
ALTER TABLE device_experiences DROP COLUMN IF EXISTS anonymous_id;
DROP TABLE IF EXISTS anon_devices;
DELETE FROM iap_skus WHERE kind='consumable_pack';
UPDATE iap_skus SET enabled=FALSE, updated_at=now() WHERE code='sub_times';
DO $$
DECLARE def text;
BEGIN
  SELECT pg_get_constraintdef(oid) INTO def FROM pg_constraint WHERE conname = 'admin_grants_bucket_chk';
  IF def IS NULL THEN
    ALTER TABLE admin_grants ADD CONSTRAINT admin_grants_bucket_chk CHECK (
      (kind = 'quota' AND bucket IN ('pack','subscription','quota'))
      OR (kind = 'free_count' AND bucket = '')
    );
  ELSIF position('quota' in def) = 0 THEN
    ALTER TABLE admin_grants DROP CONSTRAINT admin_grants_bucket_chk;
    ALTER TABLE admin_grants ADD CONSTRAINT admin_grants_bucket_chk CHECK (
      (kind = 'quota' AND bucket IN ('pack','subscription','quota'))
      OR (kind = 'free_count' AND bucket = '')
    );
  END IF;
END $$;
`)
	if err != nil {
		return err
	}
	if err := s.unifyPaymentCatalog(ctx); err != nil {
		return err
	}
	if _, err := s.Exec(ctx, `DROP TABLE IF EXISTS explore_topics`); err != nil {
		return err
	}
	return s.commentSchema(ctx)
}

func (s *Store) commentSchema(ctx context.Context) error {
	_, err := s.Exec(ctx, `
COMMENT ON TABLE operators IS '管理端账号。对应 /api/admin/v1/auth/*；role=admin 拥有全部模块读写并可分配权限，readonly 按 operator_module_perms 授权。';
COMMENT ON COLUMN operators.role IS 'admin 或 readonly。admin 不读权限表，隐含全部模块。';
COMMENT ON TABLE operator_module_perms IS '运营账号的模块权限。module=content/ai/records/accounts/pay；can_write=false 为只读。admin 角色不写此表。';
COMMENT ON COLUMN operator_module_perms.module IS 'content 内容、ai 模型、records 记录、accounts 账号、pay 付费。与后台菜单分组一致。';

COMMENT ON TABLE app_users IS 'App 用户。对应 /api/app/v1/auth/*、/profile。merged_into 非空表示已并入另一账号。';
COMMENT ON COLUMN app_users.public_id IS '对外短 ID，6 位。对应「我的」短码，首次读 profile 生成。';
COMMENT ON COLUMN app_users.phone IS '历史列，App 不再绑定或展示手机号。注销/合并时仍会清空。';
COMMENT ON COLUMN app_users.deck_code IS '当前官方牌面风格 code，须对应 decks 中已启用项。使用自定义牌组时仍作名称解析兜底。';
COMMENT ON COLUMN app_users.gender IS '档案性别。入库值：女性 / 男性 / 其他 / 不愿透露。PUT /profile 另接受女、男。';
COMMENT ON COLUMN app_users.explore_preferences IS '探索偏好 JSON 字符串数组。对应个人档案多选项，PUT /profile。';
COMMENT ON COLUMN app_users.custom_deck_id IS '当前使用的自定义牌组。空则用 deck_code 官方风格。';
COMMENT ON COLUMN app_users.merged_into IS '历史并账标记。新请求不再合并；非空的旧账号仍不能登录。';

COMMENT ON TABLE refresh_tokens IS '刷新令牌。App 登录不签发；管理端会话用。';
COMMENT ON TABLE identity_claims IS '已占用的登录身份（email/phone/wechat/apple），删号后仍占坑防抢注。';

COMMENT ON TABLE spreads IS '牌阵。普通占卜与探索共用。theme_key 为空是普通占卜和推荐角标；非空是探索主题下的牌阵。对应 GET /spreads 与 GET /explore/topics。';
COMMENT ON COLUMN spreads.positions IS 'JSON 牌位：index/name/meaning。';
COMMENT ON COLUMN spreads.recommended IS '已上架普通牌阵里标成推荐的牌阵，可以有多张。映射表未冻结前，推荐接口返回这些牌阵，用户可以改选。一张都没有时不兜底。';
COMMENT ON COLUMN spreads.name_en IS '牌阵英文名。更多牌阵文字稿里的英文名称。空则未填。';
COMMENT ON COLUMN spreads.thumbnail IS '选阵屏牌阵示意图。空则 App 选阵仍用内置牌背占位；管理端列表不把牌背当成示意图。';
COMMENT ON COLUMN spreads.theme_key IS '探索主题键，如 yes_or_no。空则不是探索牌阵。同一主题下可有多套。';
COMMENT ON COLUMN spreads.theme_title IS '探索牌阵主题标题。';
COMMENT ON COLUMN spreads.theme_subtitle IS '探索牌阵主题副标题。';
COMMENT ON COLUMN spreads.short_sentence IS '牌阵短句，对应文字总结版。';
COMMENT ON COLUMN spreads.depth_group IS '普通占卜分组：quick 快速指引、advanced 进阶分析、panorama 深度全景。空则不是更多牌阵这 19 套。不是免费白名单。';
COMMENT ON COLUMN spreads.quota_cost IS '这套牌阵的会员扣次。结算优先读这里；NULL 时用全局会员单次额度。两者都空则不猜测。';
COMMENT ON COLUMN spreads.sort_order IS '列表顺序。普通占卜 1–19。探索从 101 起，按主题再按主题内顺序。默认 1000。';
COMMENT ON TABLE invite_rewards IS '邀请关系。邀请码是邀请人账号 ID。一个被邀请账号只能绑定一次。双方各得一笔奖励，最多成功邀请 5 人。';
COMMENT ON TABLE invite_grants IS '邀请奖励按笔。role=inviter/invitee，各 3 次，从发放时刻起 1 年。未过期且最早到期的先扣。';
COMMENT ON TABLE custom_decks IS '用户自定义牌组。face_source/back_source 为 upload 或官方 decks.code。';
COMMENT ON TABLE custom_deck_faces IS '自定义牌组已上传的牌面。card_id 对应韦特 0–77。';
COMMENT ON TABLE tarotists IS '塔罗师。对应 GET /tarotists；会员权益 benefits.tarotists.count 统计 enabled=true。';
COMMENT ON TABLE vendors IS '大模型供应商。api_key_enc 用 KEK 加密。';
COMMENT ON TABLE bindings IS '任务到供应商+模型的绑定。task=detect/rewrite/recommend/interpret/tarotist。';

COMMENT ON TABLE banners IS '运营 Banner。对应 GET /banners 与管理端 /admin/v1/banners。可触发个推。';
COMMENT ON TABLE campaigns IS '探索「参与活动」。对应 GET /campaigns；detail_status=pending 时正文未定稿。';
COMMENT ON TABLE readings IS '一次解读。对应 /readings*。type=normal/tarotist/explore/daily。';
COMMENT ON COLUMN readings.type IS 'normal 普通、tarotist 占卜师、explore 探索、daily 今日一牌。';
COMMENT ON COLUMN readings.theme_key IS '探索主题键。创建时可写；选阵后若为空则从 spreads.theme_key 回填。yes_or_no/either_or 为此刻抉择，其余为主题占卜。';
COMMENT ON COLUMN readings.status IS '流程状态：QUESTION_CREATED → … → INTERPRETING → COMPLETED。INTERPRETING 时 interpretation 可能是未完成片段。';
COMMENT ON COLUMN readings.interpretation IS '解读正文。任务进行中为已生成片段，完成后为全文。App 退出再进用 GET /readings/{id} 取。';
COMMENT ON TABLE ai_jobs IS 'AI 生成任务。HTTP/SSE 断开后仍在跑；结果写入 readings 或 yearly_reports。';
COMMENT ON TABLE preset_questions IS '问题输入页预设题库。pool=featured 首屏，pool=backup 更多问题。scene 为文档六类场景。对应 GET /preset-questions?scene=。来源 docs/塔塔_预设问题库_V1.docx。';
COMMENT ON COLUMN preset_questions.pool IS 'featured 首屏混合 15 条；backup 备用题库。';
COMMENT ON COLUMN preset_questions.scene IS '场景类型：relationship/career/study/growth/social/life。App 用 ?scene= 查询。';
COMMENT ON COLUMN preset_questions.category IS '场景中文名，与文档一致：感情关系/事业与工作/学业与考试/自我成长/人际关系/生活与选择。';
COMMENT ON COLUMN ai_jobs.kind IS 'interpret / detect / followup / yearly。';
COMMENT ON COLUMN ai_jobs.status IS 'queued / running / succeeded / failed。';
COMMENT ON COLUMN ai_jobs.partial_text IS '流式过程中的已产出文本，供重连回放。';
COMMENT ON COLUMN readings.cards IS 'JSON 牌面。灵数抽牌 numbers 映射 Rider-Waite 后写入。';
COMMENT ON TABLE reading_messages IS '解读追问对话。对应 GET /readings/{id} 的 messages、followups。';

COMMENT ON TABLE monthly_report_saves IS '月度报告「已保存」标记。对应 POST /monthly-reports/save。统计仍现查 readings。';
COMMENT ON TABLE yearly_reports IS '年度运势。解锁后按 Pixso 1034:13493 四步：资料 → 十二宫 12 张 → 四季 4 张 → 生成报告。';
COMMENT ON COLUMN yearly_reports.unlock_source IS 'year_subscription 年卡免费，purchase 单独买。';
COMMENT ON COLUMN yearly_reports.birth_time IS '出生时间 HH:MM。与 birth_place 一并写入 app_users。';
COMMENT ON COLUMN yearly_reports.birth_place IS '出生地点，最多 80 字。';
COMMENT ON COLUMN yearly_reports.rhythm_cards IS '十二宫 12 张 JSON（自我到潜意识）。空表示未抽。接口字段 rhythm_cards / house_cards。';
COMMENT ON COLUMN yearly_reports.domain_cards IS '四季 4 张 JSON（春夏秋冬）。空表示未抽。不与十二宫牌重复。接口字段 domain_cards / season_cards。';
COMMENT ON COLUMN yearly_reports.report IS '模型生成的年度报告正文。未生成前为空，不编造。';
COMMENT ON COLUMN yearly_reports.report_status IS 'pending / generating / ready / failed。';

COMMENT ON TABLE iap_skus IS '付费商品目录。会员、额度补充包、年运单买、牌组共用这一张表，用 kind 区分。到账不在这张表。';
COMMENT ON COLUMN iap_skus.kind IS 'month_subscription 月卡；year_subscription 年卡；quota_pack 额度补充包；yearly_unlock 年运单买；deck_year/deck_bundle/deck_single/deck_addon 牌组。';
COMMENT ON COLUMN iap_skus.price_amount_minor IS '价格，单位分。测试数据取设计图：月卡 3900、年卡 19800、补充包 2000/6000/10000、年运 2880、牌组 9800/1200/660/300。';
COMMENT ON COLUMN iap_skus.quota_grant IS '入账额度，进 quota_balance。补充包测试数据 50/160/300。会员开通赠送图上没有总数，为 null。年运和牌组必须为 null。';
COMMENT ON COLUMN iap_skus.deck_codes IS '牌组商品包含的牌组编码 JSON。年度畅享验单写成 *。';
COMMENT ON COLUMN iap_skus.period_unit IS '月卡 month、年卡 year；其余空。';
COMMENT ON COLUMN iap_skus.code IS '稳定编码。种子：sub_month/sub_year，pack_quota_20/60/100，unlock_yearly。';
COMMENT ON COLUMN iap_skus.app_store_product_id IS 'App Store 商品 ID。验单按票据 product_id 映射本列。种子为占位编码，上架前换成 App Store Connect 正式 ID。';

COMMENT ON TABLE iap_settings IS 'IAP 开关单行表 id=1。restore_hook_enabled 控制恢复购买。';
COMMENT ON TABLE iap_orders IS '本地验单订单。不请求苹果/谷歌，只按 product_id 映射 iap_skus。对应 POST /iap/verify、/iap/restore。';
COMMENT ON COLUMN iap_orders.sku_kind IS '下单时拷贝的 iap_skus.kind。';
COMMENT ON COLUMN iap_orders.credited_quota IS '本次入账额度。年运、牌组、未配置额度为 null。';
COMMENT ON COLUMN iap_orders.credit_status IS 'none/credited/amount_unconfigured/skipped_expired/deferred。deferred 表示下一期尚未开始。';

COMMENT ON TABLE subscriptions IS '商店订阅有效期。sku_kind 这里是 month/year（与 iap_skus.kind 不同）。年卡有效则年运免费解锁。';
COMMENT ON COLUMN subscriptions.sku_kind IS 'month 或 year，由 month_subscription/year_subscription 推导。';

COMMENT ON TABLE quota_balances IS '会员统一额度。只保留 quota_balance。塔塔币分账列由 docs/sql/drop_tata_coin_2026-11-17.sql 删除。';
COMMENT ON TABLE quota_ledger IS '统一额度流水。新写入 bucket=quota。历史 subscription/consumable 由 drop SQL 回写。';
COMMENT ON TABLE quota_lots IS '会员额度批次。source=subscription 先扣，再按 expires_at 扣 pack/admin。到期剩余作废，不冻结、不恢复。';
COMMENT ON COLUMN quota_balances.quota_balance IS '会员额度池合计，等于未到期 quota_lots.remaining 之和。没有批次的旧余额仍按原账户扣。';

COMMENT ON TABLE rewards IS '免费次数账本。invite_remaining=邀请奖励；admin_grant_remaining=后台补发/兑换码。remaining 注册奖励已下线。';
COMMENT ON COLUMN rewards.remaining IS '历史注册奖励，扣费不再读取。';
COMMENT ON COLUMN rewards.admin_grant_remaining IS '管理端补发、兑换码 free_count。免费用户在邀请次数之后可扣。';
COMMENT ON COLUMN rewards.invite_remaining IS '邀请奖励次数。不过日清，最多 15。与每日免费分开记账。';

COMMENT ON TABLE quota_settings IS '计费参数。id=1。quota_cost 现为会员单次额度消耗，未配置为 null 不猜测。daily_free_count 默认 1。';
COMMENT ON COLUMN quota_settings.followup_limit IS '每条解读追问上限。管理端 PUT /quota-params 可改；null 不限制。种子默认 5（设计稿）。';
COMMENT ON COLUMN quota_settings.daily_free_count IS '每个业务日的共用免费次数。管理端可改，默认 1。';
COMMENT ON COLUMN quota_settings.daily_slot_count IS '每个业务日可抽今日一牌（含明日运势）的次数。管理端可改，默认 1。';
COMMENT ON COLUMN quota_settings.registration_grant_delay_minutes IS '注册完成后多少分钟发放赠送。NULL 不自动发；0 表示注册完成立即发。';
COMMENT ON COLUMN quota_settings.registration_valid_days IS '注册赠送有效天数，从实际发放时刻起算。默认 3。';
COMMENT ON COLUMN quota_settings.registration_grant_count IS '每次注册实际发放的次数。默认 1。';
COMMENT ON COLUMN quota_settings.registration_spread_ids IS '注册赠送和邀请可用的普通占卜牌阵 id JSON 数组。空数组表示名单未配。只允许 3 张及以内。';
COMMENT ON COLUMN quota_settings.tokens_per_quota IS '多少 token 折成 1 额度。NULL 未启用。启用后解读、改写、年运生成、会员追问按输入加输出 token 向上取整。结算尚未读取。';
COMMENT ON TABLE registration_gifts IS '注册赠送的独立记录。与新人引导、塔罗师试用、邀请奖励分开。间隔未配置时只记注册时间，不发放。reward_blocked 为真表示邮箱、微信、苹果、手机号或设备曾注册过，不再发放。';
COMMENT ON TABLE user_devices IS '账号登录时记下的设备。注销不删这行。同一设备再注册新账号时不再发注册赠送和邀请奖励。';
COMMENT ON COLUMN app_users.cancelled_at IS '软删除时刻。行和占卜记录保留。邮箱、微信、苹果、手机号、设备写入 identity_claims 后不能再注册。';
COMMENT ON TABLE quota_charges IS '单次解读扣费。entitlement=daily_free/invite_reward/admin_grant_free/member_quota/ai_trial/onboard_trial。';
COMMENT ON TABLE device_experiences IS '设备只决定引导出不出现。解读资格在账号 reading_trial_used，每账号 1 次。';
COMMENT ON COLUMN readings.onboard_experience IS '新人引导那一次解读。AI 成功后才扣账号资格，失败不扣。成功后可回看，不能再免费生成、追问或补抽。';
COMMENT ON COLUMN readings.device_id IS '登录前的进度按设备保存。登录时把同一设备、尚未归属账号的记录挂到该账号。';
COMMENT ON TABLE quota_paywall IS '付费墙暂停点。IAP 入账后可 resume。';
COMMENT ON TABLE quota_use IS '每日免费占用（同一 reading 唯一）。条数受 quota_settings.daily_free_count 限制。并账后写入 daily_free_*。';
COMMENT ON TABLE daily_free_balance IS '并账后的每日免费余额快照。';
COMMENT ON TABLE daily_free_uses IS '并账后每日免费消耗明细。';
COMMENT ON TABLE ledger IS '次数扣减流水。管理端 GET /admin/v1/ledger；IAP 额度记在 quota_ledger。';

COMMENT ON TABLE admin_grants IS '管理端人工补发。新记录只写 kind=quota，bucket 为 subscription 或 pack。历史 free_count 只保留查询。';
COMMENT ON TABLE redeem_codes IS '兑换码。kind=quota 入额度，free_count 入免费次数。管理端 POST /redeem-codes 生成，App POST /redeem 核销。';
COMMENT ON TABLE redeem_redemptions IS '兑换码核销记录。对应 GET /redeem/records。';

COMMENT ON TABLE notification_preferences IS '通知三开关。今日/明日抽牌走本地通知；messages_enabled 控制优惠/活动推送。对应 /notification-preferences。';
COMMENT ON TABLE push_devices IS '个推 CID。优惠/活动服务端推送目标。今日/明日抽牌不经此表。对应 POST /push/register。';
COMMENT ON TABLE app_messages IS '站内信（优惠/活动）。audience=all 广播，user 定向。对应 GET /messages。';
COMMENT ON TABLE app_message_reads IS '站内信已读。对应 POST /messages/{id}/read。';

COMMENT ON TABLE help_categories IS '帮助中心分类。对应 GET /help/categories。';
COMMENT ON TABLE help_articles IS '帮助文章。使用与占卜、账号与设置按 Pixso 1034:621 写入。管理端运营文案可改分类名、增删问题和排序。';
COMMENT ON COLUMN iap_skus.copy IS '价位附带运营文案。年卡用于专属活动权益；空则 benefits.campaign_perk.copy_status=pending。';
COMMENT ON TABLE app_config IS '无独立表的运营配置。key 如 yearly.2026.theme、card.thumbnail_template（{id}/{name}）、card.thumbnail_default。单牌名/宜/图在 cards 表。缩略图都未配时 App 返回内置 /uploads/card-default.png。对应 GET/PUT /copy/{kind}。';
COMMENT ON TABLE cards IS '单牌目录。种子 78 张 Rider-Waite（id 0-77，愚者=0）。管理端 GET/POST /cards、PUT /cards/{id}；App 读缩略图/宜/多语言名。抽牌仍按 deck() 0-77。';
COMMENT ON COLUMN cards.name IS '简体中文牌名，抽牌 JSON 与 i18n msgid 的稳定键。创建后不可改。';
COMMENT ON COLUMN cards.yi IS '今日一牌正位「宜」的运营覆盖。空则用 upright_yi。';
COMMENT ON COLUMN cards.upright_message IS '今日一牌正位寄语。来源 Pixso 1034:624。';
COMMENT ON COLUMN cards.upright_yi IS '今日一牌正位今日宜，四条用 / 连接。';
COMMENT ON COLUMN cards.reversed_message IS '今日一牌逆位寄语。来源 Pixso 1034:624。';
COMMENT ON COLUMN cards.reversed_yi IS '今日一牌逆位今日宜，四条用 / 连接。';
COMMENT ON COLUMN cards.thumbnail IS '遗留单牌图覆盖。官方套图在 deck_faces；空则走牌组 faces / 模板 / 默认图 / 内置牌背。';
COMMENT ON TABLE decks IS '牌组风格。莱德韦特默认可切换。星辰之境等四套 unlock_required。售价在 iap_skus 的 deck_* 行。';
COMMENT ON TABLE user_deck_unlocks IS '牌组解锁。deck_code=* 表示年度畅享，覆盖当前和以后需解锁的牌组。不写额度流水。';
COMMENT ON COLUMN decks.unlock_required IS '为 true 时，未解锁用户不能把它设为当前牌组。';
COMMENT ON COLUMN decks.description IS '套装一句介绍。对应定稿 693:39。空为未填。';
COMMENT ON COLUMN decks.tags IS '套装标签 JSON 数组。空为未填。';
COMMENT ON COLUMN decks.thumbnail IS '套装封面/列表图。App 仍用 thumbnail 字段名。';
COMMENT ON COLUMN decks.back_url IS '该套牌卡背图。空则客户端可用默认牌背。';
COMMENT ON TABLE deck_faces IS '官方牌组每张牌面。card_id 对应韦特 0–77。未配则回落到 cards.thumbnail / 模板。';
COMMENT ON TABLE wishlists IS '功能心愿单。对应 POST /wishlist、GET /admin/v1/wishlists。';
COMMENT ON TABLE feedback IS '反馈意见。category=使用问题/内容建议/其他。对应 POST /feedback、GET /admin/v1/feedback。';

COMMENT ON TABLE account_merge_requests IS '历史合并请求。新的绑定冲突不再写入，确认接口已下线。';
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
	if err := s.seedReadonlyModulePerms(ctx); err != nil {
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
		_, err := s.Exec(ctx, `INSERT INTO spreads(name, description, card_count, positions, enabled, charged, recommended) VALUES ($1,$2,$3,$4,true,false,true)`,
			"时间流", "过去 / 现在 / 未来", 3, string(pos))
		if err != nil {
			return err
		}
	}
	var rec int
	if err := s.QueryRow(ctx, `SELECT count(*) FROM spreads WHERE recommended`).Scan(&rec); err != nil {
		return err
	}
	if rec == 0 {
		if _, err := s.Exec(ctx, `UPDATE spreads SET recommended=true WHERE name='时间流'`); err != nil {
			return err
		}
	}
	if err := s.seedOrdinarySpreads(ctx); err != nil {
		return err
	}
	if err := s.seedDeckShop(ctx); err != nil {
		return err
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
	if err := s.seedExploreSpreads(ctx); err != nil {
		return err
	}
	if err := s.seedCampaigns(ctx); err != nil {
		return err
	}
	if err := s.seedHelp(ctx); err != nil {
		return err
	}
	if err := s.seedContentV1(ctx); err != nil {
		return err
	}
	if err := s.seedCards(ctx); err != nil {
		return err
	}
	return s.seedTarotistPublic(ctx)
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
		{"account", "账号与设置", "ready", "", 2},
	}
	for _, c := range categories {
		if _, err := s.Exec(ctx, `INSERT INTO help_categories(category_id, title, articles_status, empty_note, sort_order)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (category_id) DO NOTHING`,
			c.id, c.title, c.status, c.note, c.order); err != nil {
			return err
		}
	}
	for _, a := range helpArticleCopy {
		if _, err := s.Exec(ctx, `INSERT INTO help_articles(article_id, category_id, title, body, body_status, sort_order)
			VALUES ($1,$2,$3,$4,'ready',$5)
			ON CONFLICT (article_id) DO NOTHING`,
			a.ID, a.Category, a.Title, a.Body, a.Order); err != nil {
			return err
		}
		if _, err := s.Exec(ctx, `UPDATE help_articles SET body=$1, body_status='ready'
			WHERE article_id=$2 AND body_status='pending'`, a.Body, a.ID); err != nil {
			return err
		}
	}
	if _, err := s.Exec(ctx, `DELETE FROM help_articles WHERE category_id='membership_pay'`); err != nil {
		return err
	}
	if _, err := s.Exec(ctx, `DELETE FROM help_categories WHERE category_id='membership_pay'`); err != nil {
		return err
	}
	// 旧种子把账号与设置排在第 3 位，中间那一类已经删掉。没有其它分类时收成 2。
	if _, err := s.Exec(ctx, `UPDATE help_categories SET sort_order=2
		WHERE category_id='account' AND sort_order=3
		  AND NOT EXISTS (SELECT 1 FROM help_categories WHERE category_id NOT IN ('reading','account'))`); err != nil {
		return err
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
		code, kind, name, period, apple string
		price                           int64
		grant, periodCount              any
	}
	slots := []sku{
		{"sub_month", "month_subscription", "月卡", "month", "tata.membership.month", 3900, nil, 1},
		{"sub_year", "year_subscription", "年卡", "year", "tata.membership.year", 19800, nil, 1},
		{"pack_quota_20", "quota_pack", "额度补充包 20元", "", "tata.quota.20", 2000, 50, nil},
		{"pack_quota_60", "quota_pack", "额度补充包 60元", "", "tata.quota.60", 6000, 160, nil},
		{"pack_quota_100", "quota_pack", "额度补充包 100元", "", "tata.quota.100", 10000, 300, nil},
		{"unlock_yearly", "yearly_unlock", "年运解锁", "", "tata.unlock.yearly", 2880, nil, nil},
	}
	const upsert = `INSERT INTO iap_skus(
		code, kind, display_name, period_unit, period_count,
		price_amount_minor, price_currency, quota_grant, app_store_product_id)
		VALUES ($1,$2,$3,$4,$5,$6,'CNY',$7,$8)
		ON CONFLICT (code) DO UPDATE SET
			kind=EXCLUDED.kind,
			display_name=EXCLUDED.display_name,
			period_unit=EXCLUDED.period_unit,
			period_count=COALESCE(iap_skus.period_count, EXCLUDED.period_count),
			price_amount_minor=COALESCE(iap_skus.price_amount_minor, EXCLUDED.price_amount_minor),
			price_currency=COALESCE(iap_skus.price_currency, EXCLUDED.price_currency),
			quota_grant=COALESCE(iap_skus.quota_grant, EXCLUDED.quota_grant),
			app_store_product_id=COALESCE(NULLIF(iap_skus.app_store_product_id,''), EXCLUDED.app_store_product_id),
			updated_at=now()`
	for _, slot := range slots {
		if _, err := s.Exec(ctx, upsert, slot.code, slot.kind, slot.name, slot.period,
			slot.periodCount, slot.price, slot.grant, slot.apple); err != nil {
			return err
		}
	}
	_, err := s.Exec(ctx, `INSERT INTO iap_settings(id) VALUES (1) ON CONFLICT (id) DO NOTHING`)
	return err
}

func (s *Store) seedReadonlyModulePerms(ctx context.Context) error {
	_, err := s.Exec(ctx, `
INSERT INTO operator_module_perms (operator_id, module, can_write)
SELECT o.id, m.module, FALSE
FROM operators o
CROSS JOIN (VALUES ('content'),('ai'),('records'),('accounts'),('pay')) AS m(module)
WHERE o.username = 'readonly'
ON CONFLICT (operator_id, module) DO NOTHING`)
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
