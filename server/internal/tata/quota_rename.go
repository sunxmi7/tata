package tata

import "context"

// renameDiamondSchema renames the retired diamond tables and columns to quota.
// It runs before the rest of migrate so later statements only see the quota names.
func (s *Store) renameDiamondSchema(ctx context.Context) error {
	_, err := s.Exec(ctx, `
DO $$
BEGIN
  IF to_regclass('public.diamond_balances') IS NOT NULL AND to_regclass('public.quota_balances') IS NULL THEN
    ALTER TABLE diamond_balances RENAME TO quota_balances;
  END IF;
  IF to_regclass('public.diamond_ledger') IS NOT NULL AND to_regclass('public.quota_ledger') IS NULL THEN
    ALTER TABLE diamond_ledger RENAME TO quota_ledger;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'diamond_ledger_bucket_chk') THEN
    ALTER TABLE quota_ledger RENAME CONSTRAINT diamond_ledger_bucket_chk TO quota_ledger_bucket_chk;
  END IF;
  IF to_regclass('public.diamond_ledger_order_bucket') IS NOT NULL THEN
    ALTER INDEX diamond_ledger_order_bucket RENAME TO quota_ledger_order_bucket;
  END IF;
  IF to_regclass('public.diamond_ledger_user_idx') IS NOT NULL THEN
    ALTER INDEX diamond_ledger_user_idx RENAME TO quota_ledger_user_idx;
  END IF;
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'iap_skus' AND column_name = 'diamond_grant'
  ) THEN
    ALTER TABLE iap_skus RENAME COLUMN diamond_grant TO quota_grant;
  END IF;
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'iap_orders' AND column_name = 'credited_diamonds'
  ) THEN
    ALTER TABLE iap_orders RENAME COLUMN credited_diamonds TO credited_quota;
  END IF;
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'quota_settings' AND column_name = 'diamond_cost'
  ) THEN
    ALTER TABLE quota_settings RENAME COLUMN diamond_cost TO quota_cost;
  END IF;
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'admin_grants' AND column_name = 'diamond_ledger_id'
  ) THEN
    ALTER TABLE admin_grants RENAME COLUMN diamond_ledger_id TO quota_ledger_id;
  END IF;
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'redeem_redemptions' AND column_name = 'diamond_ledger_id'
  ) THEN
    ALTER TABLE redeem_redemptions RENAME COLUMN diamond_ledger_id TO quota_ledger_id;
  END IF;
END $$;
`)
	if err != nil {
		return err
	}
	_, err = s.Exec(ctx, `
DO $$
BEGIN
  IF to_regclass('public.admin_grants') IS NOT NULL THEN
    ALTER TABLE admin_grants DROP CONSTRAINT IF EXISTS admin_grants_kind_chk;
    ALTER TABLE admin_grants DROP CONSTRAINT IF EXISTS admin_grants_bucket_chk;
    UPDATE admin_grants SET kind = 'quota' WHERE kind = 'diamonds';
    ALTER TABLE admin_grants ADD CONSTRAINT admin_grants_kind_chk CHECK (kind IN ('quota','free_count'));
    ALTER TABLE admin_grants ADD CONSTRAINT admin_grants_bucket_chk CHECK (
      (kind = 'quota' AND bucket IN ('pack','subscription','quota'))
      OR (kind = 'free_count' AND bucket = '')
    );
  END IF;
  IF to_regclass('public.redeem_codes') IS NOT NULL THEN
    UPDATE redeem_codes SET kind = 'quota' WHERE kind = 'diamonds';
    -- kind 约束由后续 migrate 统一落到 SKU/裸额度集合；此处不再写回 free_count。
  END IF;
  IF to_regclass('public.quota_charges') IS NOT NULL THEN
    UPDATE quota_charges SET entitlement = 'quota_pack' WHERE entitlement = 'diamond_pack';
    UPDATE quota_charges SET entitlement = 'quota_subscription' WHERE entitlement = 'diamond_subscription';
  END IF;
END $$;
`)
	return err
}
