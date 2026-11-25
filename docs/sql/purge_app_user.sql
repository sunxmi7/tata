-- 测试环境：物理清除一个 App 账号及其业务数据，同一邮箱/微信/苹果可重新注册。
-- 只改 _purge_params 里用得到的字段，其余保持 NULL。
--
-- 微信登录：
--   1) 还没注销：后台「用户账号」的微信列就是 wechat_openid，填到下面即可。
--   2) 已经注销：微信列已被清空。用后台这条记录的数字 id，
--      再把 identity_claims 里 kind='wechat' 的 value 填进 wechat_openid，否则同一微信会提示「该微信已注销，不能重复注册」。
-- 先查：
--   SELECT id, name, wechat_openid, cancelled_at FROM app_users WHERE wechat_openid IS NOT NULL OR cancelled_at IS NOT NULL ORDER BY id DESC;
--   SELECT * FROM identity_claims WHERE kind='wechat' ORDER BY created_at DESC;

BEGIN;

CREATE TEMP TABLE _purge_params ON COMMIT DROP AS
SELECT
  NULL::bigint AS user_id,            -- 已注销时必填，例如 42
  NULL::text AS email,
  NULL::text AS public_id,
  'oXXXX'::text AS wechat_openid,     -- 改成后台「微信」列或 identity_claims.value
  NULL::text AS apple_sub;

CREATE TEMP TABLE _purge_users ON COMMIT DROP AS
WITH RECURSIVE picked AS (
  SELECT u.id
  FROM app_users u
  CROSS JOIN _purge_params p
  WHERE (p.user_id IS NOT NULL AND u.id = p.user_id)
     OR (p.email IS NOT NULL AND p.email <> '' AND lower(u.email) = lower(p.email))
     OR (p.public_id IS NOT NULL AND p.public_id <> '' AND u.public_id = p.public_id)
     OR (p.wechat_openid IS NOT NULL AND p.wechat_openid <> '' AND lower(u.wechat_openid) = lower(p.wechat_openid))
     OR (p.apple_sub IS NOT NULL AND p.apple_sub <> '' AND u.apple_sub = p.apple_sub)
  UNION
  SELECT u.id
  FROM app_users u
  JOIN picked t ON u.merged_into = t.id
)
SELECT DISTINCT id FROM picked;

-- 确认会删哪些人：取消下面注释执行一次，看对了再跑全文。
-- SELECT u.id, u.email, u.wechat_openid, u.apple_sub, u.public_id, u.cancelled_at, u.merged_into
-- FROM app_users u JOIN _purge_users t ON t.id = u.id;

DO $$
DECLARE n int;
BEGIN
  SELECT count(*) INTO n FROM _purge_users;
  IF n = 0 AND NOT EXISTS (
    SELECT 1 FROM _purge_params
    WHERE coalesce(email, '') <> ''
       OR coalesce(wechat_openid, '') <> ''
       OR coalesce(apple_sub, '') <> ''
  ) THEN
    RAISE EXCEPTION '没有匹配到用户，请检查 user_id / 邮箱 / wechat_openid';
  END IF;
END $$;

CREATE TEMP TABLE _purge_idents ON COMMIT DROP AS
SELECT 'email'::text AS kind, lower(email) AS value
FROM app_users
WHERE id IN (SELECT id FROM _purge_users) AND email IS NOT NULL AND email <> ''
UNION
SELECT 'wechat', lower(wechat_openid)
FROM app_users
WHERE id IN (SELECT id FROM _purge_users) AND wechat_openid IS NOT NULL AND wechat_openid <> ''
UNION
SELECT 'apple', lower(apple_sub)
FROM app_users
WHERE id IN (SELECT id FROM _purge_users) AND apple_sub IS NOT NULL AND apple_sub <> ''
UNION
SELECT 'phone', lower(phone)
FROM app_users
WHERE id IN (SELECT id FROM _purge_users) AND phone IS NOT NULL AND phone <> ''
UNION
SELECT lower(kind), lower(value)
FROM retained_identities
WHERE user_id IN (SELECT id FROM _purge_users)
   OR source_user_id IN (SELECT id FROM _purge_users)
UNION
SELECT 'email', lower(email) FROM _purge_params WHERE email IS NOT NULL AND email <> ''
UNION
SELECT 'wechat', lower(wechat_openid) FROM _purge_params WHERE wechat_openid IS NOT NULL AND wechat_openid <> ''
UNION
SELECT 'apple', lower(apple_sub) FROM _purge_params WHERE apple_sub IS NOT NULL AND apple_sub <> '';

CREATE TEMP TABLE _purge_readings ON COMMIT DROP AS
SELECT id FROM readings WHERE user_id IN (SELECT id FROM _purge_users);

UPDATE app_users
SET custom_deck_id = NULL
WHERE id IN (SELECT id FROM _purge_users);

DELETE FROM custom_deck_faces
WHERE deck_id IN (SELECT id FROM custom_decks WHERE user_id IN (SELECT id FROM _purge_users));
DELETE FROM custom_decks WHERE user_id IN (SELECT id FROM _purge_users);

DELETE FROM reading_messages WHERE reading_id IN (SELECT id FROM _purge_readings);
DELETE FROM quota_charges WHERE user_id IN (SELECT id FROM _purge_users)
   OR reading_id IN (SELECT id FROM _purge_readings);
DELETE FROM quota_paywall WHERE user_id IN (SELECT id FROM _purge_users)
   OR reading_id IN (SELECT id FROM _purge_readings);
DELETE FROM quota_use WHERE user_id IN (SELECT id FROM _purge_users)
   OR reading_id IN (SELECT id FROM _purge_readings);
DELETE FROM daily_free_uses WHERE user_id IN (SELECT id FROM _purge_users)
   OR source_user_id IN (SELECT id FROM _purge_users)
   OR reading_id IN (SELECT id FROM _purge_readings);
DELETE FROM ledger WHERE user_id IN (SELECT id FROM _purge_users)
   OR reading_id IN (SELECT id FROM _purge_readings);
DELETE FROM ai_jobs WHERE user_id IN (SELECT id FROM _purge_users)
   OR reading_id IN (SELECT id FROM _purge_readings);
DELETE FROM readings WHERE id IN (SELECT id FROM _purge_readings)
   OR user_id IN (SELECT id FROM _purge_users);

DELETE FROM device_experiences
WHERE user_id IN (SELECT id FROM _purge_users)
   OR reading_id IN (SELECT id FROM _purge_readings);

DELETE FROM invite_rewards
WHERE inviter_id IN (SELECT id FROM _purge_users)
   OR invitee_id IN (SELECT id FROM _purge_users);
DELETE FROM rewards WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM daily_free_balance WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM quota_ledger WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM quota_balances WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM subscriptions WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM iap_orders WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM admin_grants WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM redeem_redemptions WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM wishlists WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM feedback WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM notification_preferences WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM push_devices WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM app_message_reads WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM app_messages WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM monthly_report_saves WHERE user_id IN (SELECT id FROM _purge_users);
DELETE FROM yearly_reports WHERE user_id IN (SELECT id FROM _purge_users);

DELETE FROM retained_identities
WHERE user_id IN (SELECT id FROM _purge_users)
   OR source_user_id IN (SELECT id FROM _purge_users);
DELETE FROM account_merge_requests
WHERE primary_user_id IN (SELECT id FROM _purge_users)
   OR secondary_user_id IN (SELECT id FROM _purge_users);

DELETE FROM refresh_tokens
WHERE audience = 'app' AND subject_id IN (SELECT id FROM _purge_users);

DELETE FROM identity_claims c
USING _purge_idents i
WHERE c.kind = i.kind AND c.value = i.value;

DELETE FROM app_users WHERE id IN (SELECT id FROM _purge_users);

COMMIT;
