-- 提测前：清空全部占卜记录和 App 用户数据，同一邮箱 / 微信 / 苹果 / 设备可以重新注册、重新走新人体验。
-- 只在测试库执行。先备份。整段一次执行，不要拆开。
--
-- 牌面不删：cards、deck_faces、decks，以及 uploads/cards、uploads/decks。
-- 也不动：后台账号、牌阵、塔罗师、价位、额度参数、兑换码定义、帮助中心、活动、
--         预设问题、运营文案、供应商、广播站内信、操作审计、默认头像 /uploads/avatar-default.png。
--
-- 用户头像可以删。清库前先把本地头像路径收进临时表，脚本最后查出这些路径。
-- 只删查询结果里的文件。远程 http(s) 地址删不了本地文件，默认头像不要删。
--
-- 先看规模（可选）：
--   SELECT
--     (SELECT count(*) FROM app_users) AS users,
--     (SELECT count(*) FROM readings) AS readings,
--     (SELECT count(*) FROM identity_claims) AS identity_claims,
--     (SELECT count(*) FROM device_experiences) AS devices;

BEGIN;

CREATE TEMP TABLE _purge_avatars ON COMMIT DROP AS
SELECT DISTINCT btrim(avatar_url) AS path
FROM app_users
WHERE btrim(avatar_url) <> ''
  AND btrim(avatar_url) <> '/uploads/avatar-default.png'
  AND btrim(avatar_url) NOT LIKE '%/uploads/cards/%'
  AND btrim(avatar_url) NOT LIKE '%/uploads/decks/%'
  AND btrim(avatar_url) NOT LIKE '%/cards/%'
  AND btrim(avatar_url) NOT LIKE '%/decks/%'
  AND NOT EXISTS (
    SELECT 1 FROM tarotists t
    WHERE btrim(t.avatar_url) = btrim(app_users.avatar_url)
      AND btrim(t.avatar_url) <> ''
  );

TRUNCATE
  reading_messages,
  quota_charges,
  quota_paywall,
  quota_use,
  daily_free_uses,
  ledger_lot_draws,
  ledger,
  ai_usages,
  ai_jobs,
  readings,
  device_experiences,
  invite_grants,
  invite_rewards,
  invite_attributions,
  rewards,
  daily_free_balance,
  quota_lots,
  quota_ledger,
  quota_balances,
  subscriptions,
  iap_orders,
  admin_grants,
  redeem_redemptions,
  wishlists,
  feedback,
  notification_preferences,
  push_devices,
  app_message_reads,
  monthly_report_saves,
  yearly_reports,
  user_deck_unlocks,
  user_devices,
  registration_gifts,
  custom_deck_faces,
  custom_decks,
  retained_identities,
  account_merge_requests,
  identity_claims,
  idempotency
RESTART IDENTITY;

-- 定向站内信属于用户数据；audience=all 的运营广播留下。
DELETE FROM app_messages WHERE audience = 'user';

-- 只踢 App 登录态，后台 refresh token（audience=admin）不动。
DELETE FROM refresh_tokens WHERE audience = 'app';

TRUNCATE app_users RESTART IDENTITY;

-- 这些是用户头像。牌面不在里面。本地路径形如 /uploads/misc/<文件名>，到 uploads/misc 下按文件名删。
SELECT path AS delete_user_avatar FROM _purge_avatars ORDER BY 1;

COMMIT;
