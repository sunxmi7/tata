import { requestClient } from '#/api/request';

export interface AppUser {
  id: number;
  email: string | null;
  wechat_openid: string | null;
  apple_sub: string | null;
  phone: string | null;
  timezone: string | null;
  name?: string;
  nickname?: string;
  avatar_url?: string | null;
  gender?: string | null;
  birth_date?: string | null;
  explore_preferences?: string[];
  tarotist_trial_used: boolean;
  tease_off: boolean;
  cancelled_at: string | null;
  created_at: string;
}

export interface UserStatItem {
  label: string;
  count: number;
  percent: number;
}

export interface UserStatGroup {
  total: number;
  items: UserStatItem[];
}

export interface UserStats {
  total_users: number;
  gender: UserStatGroup;
  age: UserStatGroup;
  explore: UserStatGroup;
}

export interface OperatorModule {
  module: string;
  can_write: boolean;
}

export interface Operator {
  id: number;
  username: string;
  role: 'admin' | 'readonly' | string;
  disabled: boolean;
  modules?: OperatorModule[];
}

export interface ReadingRow {
  id: number;
  user_id?: number | null;
  anonymous_id?: string | null;
  type: string;
  question?: string | null;
  status: string;
  vendor_name?: string | null;
  model_name?: string | null;
  fail_reason?: string | null;
}

export interface LedgerRow {
  id: number;
  user_id: number;
  reading_id: number | null;
  entitlement: string;
  amount: number;
  reason: string;
  result: string;
  created_at: string;
  grant_id?: number | null;
}

export interface AuditRow {
  id: number;
  operator_id: number;
  action: string;
  object_type: string;
  object_id: string;
  detail: string;
  actor_kind?: string;
  created_at: string;
}

export interface MessageRow {
  id: number;
  reading_id: number;
  turn_id: number;
  seq: number;
  role: string;
  content: string;
  created_at: string;
}

export interface CalendarRow {
  id: number;
  user_id: number | null;
  type: string;
  question: string;
  status: string;
  progress: string;
  business_date: string;
}

export interface GrantRow {
  id: number;
  user_id: number;
  operator_id: number;
  kind: string;
  amount: number;
  bucket?: string;
  ledger_bucket?: string;
  reason: string;
  note: string;
  diamond_ledger_id?: number | null;
  count_ledger_id?: number | null;
  created_at: string;
}

export interface GrantList {
  grants: GrantRow[];
  can_write?: boolean;
  permission_note?: string;
  user_id?: number;
  quota_remaining?: number;
  balance?: number;
  free_count_remaining?: number;
  invite_remaining?: number;
}

export function listReadings() {
  return requestClient.get<ReadingRow[]>('/readings');
}
export function retryReading(id: number) {
  return requestClient.post(`/readings/${id}/retry`);
}
export function listLedger() {
  return requestClient.get<LedgerRow[]>('/ledger');
}
export function listOperators() {
  return requestClient.get<Operator[]>('/operators');
}
export function createOperator(data: {
  username: string;
  password: string;
  role: 'admin' | 'readonly';
  modules?: OperatorModule[];
}) {
  return requestClient.post<{ id: number }>('/operators', data);
}
export function updateOperator(
  id: number,
  data: { disabled?: boolean; modules?: OperatorModule[] },
) {
  return requestClient.put(`/operators/${id}`, data);
}
export function listUsers() {
  return requestClient.get<AppUser[]>('/users');
}
export function getUserStats() {
  return requestClient.get<UserStats>('/users/stats');
}
export function listAudit() {
  return requestClient.get<AuditRow[]>('/audit');
}
export function listMessages() {
  return requestClient.get<MessageRow[]>('/messages');
}
export function listCalendar() {
  return requestClient.get<CalendarRow[]>('/calendar');
}
export function listGrants(userId?: string) {
  const query = userId?.trim() ? `?user_id=${encodeURIComponent(userId.trim())}` : '';
  return requestClient.get<GrantList>(`/grants${query}`);
}
export function createGrant(data: {
  user_id: number;
  diamonds?: number;
  free_count?: number;
  diamond_bucket?: string;
  reason?: string;
  note?: string;
}) {
  return requestClient.post<GrantList>('/grants', data);
}

export interface RedeemCode {
  id: number;
  code: string;
  kind: 'diamonds' | 'free_count' | string;
  amount: number;
  benefit_name: string;
  max_redemptions: number | null;
  enabled: boolean;
  expires_at: string | null;
  created_at: string;
  redeemed_count: number;
}

export interface RedeemCodeList {
  items: RedeemCode[];
  can_write?: boolean;
  permission_note?: string;
}

export function listRedeemCodes() {
  return requestClient.get<RedeemCodeList>('/redeem-codes');
}

export function createRedeemCodes(data: {
  kind: string;
  amount: number;
  benefit_name: string;
  code?: string;
  count?: number;
  max_redemptions?: number;
  expires_at?: string;
  enabled?: boolean;
}) {
  return requestClient.post<RedeemCodeList>('/redeem-codes', data);
}

export function updateRedeemCode(
  id: number,
  data: {
    enabled?: boolean;
    benefit_name?: string;
    max_redemptions?: number;
    expires_at?: string;
  },
) {
  return requestClient.put<RedeemCodeList>(`/redeem-codes/${id}`, data);
}
