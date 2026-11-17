import { requestClient } from '#/api/request';

export interface IAPSku {
  id: number;
  code: string;
  kind: string;
  display_name?: string;
  price_amount_minor?: number | null;
  price_currency?: string | null;
  diamond_grant?: number | null;
  reading_grant?: number | null;
  period_count?: number | null;
  app_store_product_id?: string | null;
  play_product_id?: string | null;
  copy?: string | null;
  enabled?: boolean;
}

export interface IAPOrder {
  id: number;
  user_id: number;
  sku_code: string;
  sku_kind: string;
  ledger_bucket?: string;
  platform?: string;
  product_id?: string;
  verify_status?: string;
  credit_status?: string;
  credited_diamonds?: number | null;
  credited_readings?: number | null;
  created_at: string;
  verify_message?: string;
  subscription_status?: string | null;
  subscription_expires_at?: string | null;
}

export interface IAPDiamonds {
  user_id?: number;
  quota_remaining?: number;
  balance?: number;
  is_member?: boolean;
  ledger?: Array<Record<string, unknown>>;
}

export function listIAPOrders(userId?: string) {
  const query = userId?.trim() ? `?user_id=${encodeURIComponent(userId.trim())}` : '';
  return requestClient.get<IAPOrder[]>(`/iap/orders${query}`);
}
export function listIAPSkus(kind?: string) {
  const query = kind?.trim() ? `?kind=${encodeURIComponent(kind.trim())}` : '';
  return requestClient.get<IAPSku[]>(`/iap/skus${query}`);
}
export function createIAPSku(data: Record<string, unknown>) {
  return requestClient.post<IAPSku>('/iap/skus', data);
}
export function updateIAPSku(id: number, data: Record<string, unknown>) {
  return requestClient.put<IAPSku>(`/iap/skus/${id}`, data);
}
export function getIAPDiamonds(userId: string) {
  return requestClient.get<IAPDiamonds>(
    `/iap/diamonds?user_id=${encodeURIComponent(userId)}`,
  );
}
