-- 下线塔塔币：删除历史字段与桶。
-- 人工执行，不随服务启动自动跑。执行前先备份。
-- 执行后代码只读写 quota_balances.quota_balance / quota_ledger.bucket='quota'。
--
-- 保留：
--   iap_skus.quota_grant     会员开通赠送额度（未配置为 null，不编造）
--   iap_orders.credited_quota  入账数量（现为额度）
--   redeem_codes.kind='quota'  兑换写入额度。旧值 diamonds 已改名为 quota。
--   iap_skus.kind='consumable_pack' 历史 SKU 行保留并停用，便于对照旧订单

BEGIN;

-- 1) 把尚未并入的塔塔币余额并进统一额度
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'quota_balances' AND column_name = 'pack_diamonds'
  ) THEN
    UPDATE quota_balances
       SET quota_balance = quota_balance + COALESCE(pack_diamonds, 0) + COALESCE(subscription_diamonds, 0),
           updated_at = now()
     WHERE COALESCE(pack_diamonds, 0) > 0 OR COALESCE(subscription_diamonds, 0) > 0;
  END IF;
END $$;

-- 2) 流水桶统一为 quota。同一订单若曾写过两桶，只留最新一行，避免唯一约束冲突
ALTER TABLE quota_ledger DROP CONSTRAINT IF EXISTS quota_ledger_bucket_chk;
DROP INDEX IF EXISTS quota_ledger_order_bucket;

DELETE FROM quota_ledger a
 USING quota_ledger b
 WHERE a.order_id IS NOT NULL
   AND a.order_id = b.order_id
   AND a.id < b.id;

UPDATE quota_ledger
   SET bucket = 'quota'
 WHERE bucket IN ('subscription', 'consumable');

ALTER TABLE quota_ledger
  ADD CONSTRAINT quota_ledger_bucket_chk CHECK (bucket = 'quota');

CREATE UNIQUE INDEX IF NOT EXISTS quota_ledger_order_bucket
  ON quota_ledger(order_id, bucket) WHERE order_id IS NOT NULL;

-- 3) 后台补发桶统一为 quota
UPDATE admin_grants SET bucket = 'quota', ledger_bucket = 'quota'
 WHERE kind = 'quota' AND bucket IN ('pack', 'subscription');

ALTER TABLE admin_grants DROP CONSTRAINT IF EXISTS admin_grants_bucket_chk;
ALTER TABLE admin_grants ADD CONSTRAINT admin_grants_bucket_chk CHECK (
  (kind = 'quota' AND bucket = 'quota')
  OR (kind = 'free_count' AND bucket = '')
);

-- 4) 停用塔塔币 SKU，不删行（旧订单仍引用 sku_code）
UPDATE iap_skus
   SET enabled = FALSE, updated_at = now()
 WHERE kind = 'consumable_pack'
    OR code IN ('pack_60','pack_180','pack_300','pack_680','pack_1280','pack_3280','sub_times','pack_consumable');

-- 5) 删除余额表上的塔塔币字段
ALTER TABLE quota_balances DROP COLUMN IF EXISTS pack_diamonds;
ALTER TABLE quota_balances DROP COLUMN IF EXISTS subscription_diamonds;
ALTER TABLE quota_balances DROP COLUMN IF EXISTS subscription_period_end;
ALTER TABLE quota_balances DROP COLUMN IF EXISTS subscription_period_key;

COMMENT ON TABLE quota_balances IS '会员统一额度。只保留 quota_balance。';
COMMENT ON COLUMN quota_balances.quota_balance IS '会员统一额度。免费用户不扣此列；会员过期后余额保留但不能花。';
COMMENT ON TABLE quota_ledger IS '统一额度流水。bucket 固定 quota。';
COMMENT ON TABLE iap_skus IS '内购价位：会员月/年卡、额度补充包、年运解锁。consumable_pack 为已停用的塔塔币历史行。';

COMMIT;
