import { requestClient } from '#/api/request';

export interface IAPSku {
  id: number;
  code: string;
  kind: string;
  display_name?: string;
  price_amount_minor?: number | null;
  price_currency?: string | null;
  quota_grant?: number | null;
  period_count?: number | null;
  app_store_product_id?: string | null;
  play_product_id?: string | null;
  copy?: string | null;
  note?: string;
  deck_codes?: string[];
  covers_future?: boolean;
  enabled?: boolean;
}

export interface IAPOrder {
  id: number;
  user_id: number;
  sku_code: string;
  sku_kind: string;
  platform?: string;
  product_id?: string;
  verify_status?: string;
  credit_status?: string;
  credited_quota?: number | null;
  created_at: string;
  verify_message?: string;
  subscription_status?: string | null;
  subscription_expires_at?: string | null;
  price_amount_minor?: number | null;
  price_currency?: string | null;
  price_estimated?: boolean;
}

export interface IAPOrderQuery {
  user_id?: number | string;
  verify_status?: string;
  platform?: string;
  sku_code?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}

export interface AdminPage<T> {
  items: T[];
  total: number;
  limit: number;
  offset: number;
}

export interface IAPRevenue {
  from: string;
  to: string;
  grain: string;
  revenue_minor: number;
  paying_users: number;
  new_users: number;
  pay_rate: number;
  by_sku: Array<{
    sku_code: string;
    sku_kind: string;
    orders: number;
    paying_users: number;
    revenue_minor: number;
  }>;
  series: Array<{
    bucket: string;
    orders: number;
    paying_users: number;
    revenue_minor: number;
  }>;
  amount_basis?: string;
  currency?: string;
  notes?: string[];
}

export interface IAPQuota {
  user_id?: number;
  quota_remaining?: number;
  balance?: number;
  is_member?: boolean;
  ledger?: Array<Record<string, unknown>>;
}

function toQuery(params?: Record<string, unknown>) {
  if (!params) return '';
  const sp = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return;
    sp.set(key, String(value));
  });
  const q = sp.toString();
  return q ? `?${q}` : '';
}

export function listIAPOrders(params?: IAPOrderQuery | string) {
  if (typeof params === 'string') {
    return requestClient.get<AdminPage<IAPOrder>>(
      `/iap/orders${toQuery({ user_id: params.trim() || undefined })}`,
    );
  }
  return requestClient.get<AdminPage<IAPOrder>>(`/iap/orders${toQuery(params)}`);
}

export function getIAPRevenue(params?: {
  from?: string;
  to?: string;
  grain?: 'day' | 'week' | 'month';
}) {
  return requestClient.get<IAPRevenue>(`/iap/revenue${toQuery(params)}`);
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
export function getIAPQuota(userId: string) {
  return requestClient.get<IAPQuota>(
    `/iap/quota?user_id=${encodeURIComponent(userId)}`,
  );
}
