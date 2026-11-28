package tata

import "context"

func isDeckKind(kind string) bool {
	switch kind {
	case "deck_year", "deck_bundle", "deck_single", "deck_addon":
		return true
	default:
		return false
	}
}

func grantsQuota(kind string) bool {
	return kind == iapKindMonth || kind == iapKindYear || kind == iapKindQuotaPack
}

func iapPeriod(kind string) string {
	switch kind {
	case iapKindMonth:
		return "month"
	case iapKindYear:
		return "year"
	default:
		return ""
	}
}

// unifyPaymentCatalog puts every paid product on iap_skus and every purchase on iap_orders.
// Fulfillment stays split: subscriptions, quota_balances, yearly_reports, user_deck_unlocks.
func (s *Store) unifyPaymentCatalog(ctx context.Context) error {
	_, err := s.Exec(ctx, `
ALTER TABLE iap_skus ADD COLUMN IF NOT EXISTS deck_codes TEXT NOT NULL DEFAULT '[]';
ALTER TABLE iap_skus ADD COLUMN IF NOT EXISTS covers_future BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE iap_skus ADD COLUMN IF NOT EXISTS deduct_owned BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE iap_skus ADD COLUMN IF NOT EXISTS note TEXT NOT NULL DEFAULT '';
ALTER TABLE iap_skus DROP CONSTRAINT IF EXISTS iap_skus_kind_chk;
ALTER TABLE iap_skus DROP CONSTRAINT IF EXISTS iap_skus_bucket_chk;
ALTER TABLE iap_skus DROP CONSTRAINT IF EXISTS iap_skus_bucket_kind_chk;
`)
	if err != nil {
		return err
	}
	_, err = s.Exec(ctx, `
ALTER TABLE iap_skus ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 0;
`)
	if err != nil {
		return err
	}
	_, err = s.Exec(ctx, `
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'iap_skus' AND column_name = 'reading_grant'
  ) THEN
    UPDATE iap_skus SET quota_grant = reading_grant WHERE quota_grant IS NULL AND reading_grant IS NOT NULL;
    ALTER TABLE iap_skus DROP COLUMN reading_grant;
  END IF;
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'iap_orders' AND column_name = 'credited_readings'
  ) THEN
    UPDATE iap_orders SET credited_quota = credited_readings
     WHERE credited_quota IS NULL AND credited_readings IS NOT NULL;
    ALTER TABLE iap_orders DROP COLUMN credited_readings;
  END IF;
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'iap_skus' AND column_name = 'ledger_bucket'
  ) THEN
    ALTER TABLE iap_skus DROP COLUMN ledger_bucket;
  END IF;
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'iap_orders' AND column_name = 'ledger_bucket'
  ) THEN
    ALTER TABLE iap_orders DROP COLUMN ledger_bucket;
  END IF;
  DELETE FROM iap_skus WHERE kind = 'consumable_pack';
  UPDATE iap_skus SET kind = 'quota_pack' WHERE kind = 'reading_pack';
  UPDATE iap_orders SET sku_kind = 'quota_pack' WHERE sku_kind = 'reading_pack';
  IF to_regclass('public.deck_products') IS NOT NULL THEN
    INSERT INTO iap_skus(code, kind, display_name, price_amount_minor, price_currency, deck_codes, covers_future, deduct_owned, note, enabled, sort_order)
    SELECT code, kind, name, price_amount_minor, currency, deck_codes, covers_future, deduct_owned, note, enabled, sort_order
    FROM deck_products
    ON CONFLICT (code) DO NOTHING;
    DROP TABLE deck_products;
  END IF;
END $$;
ALTER TABLE iap_skus DROP CONSTRAINT IF EXISTS iap_skus_kind_chk;
ALTER TABLE iap_skus ADD CONSTRAINT iap_skus_kind_chk CHECK (kind IN (
  'month_subscription','year_subscription','quota_pack','yearly_unlock',
  'deck_year','deck_bundle','deck_single','deck_addon'
));
`)
	return err
}
