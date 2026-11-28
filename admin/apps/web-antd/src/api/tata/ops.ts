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
  merged_into?: number | null;
  register_ip?: string;
  region_province?: string;
  region_city?: string;
  region_status?: string;
}

export interface AdminPage<T> {
  items: T[];
  total: number;
  limit: number;
  offset: number;
}

export interface UserListQuery {
  q?: string;
  id?: number | string;
  cancelled?: boolean | string;
  merged?: boolean | string;
  limit?: number;
  offset?: number;
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

export interface UserDailyNew {
  date: string;
  count: number;
}

export interface UserStats {
  total_users: number;
  gender: UserStatGroup;
  age: UserStatGroup;
  explore: UserStatGroup;
  region?: UserStatGroup;
  daily_new?: UserDailyNew[];
  notes?: string[];
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
  user_display?: string | null;
  type: string;
  conversation_type?: string | null;
  conversation_label?: string | null;
  question?: string | null;
  status: string;
  stage_label?: string;
  vendor_name?: string | null;
  model_name?: string | null;
  fail_reason?: string | null;
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
  price_minor?: number | null;
  price_currency?: string | null;
  created_at?: string;
}

export interface LedgerDraw {
  lot_id?: number | null;
  source: string;
  amount: number;
  remaining_after: number;
  expires_at?: string | null;
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
  reading_status?: string | null;
  draws?: LedgerDraw[];
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

export interface WishlistRow {
  id: number;
  user_id: number;
  content: string;
  image_url: string;
  created_at: string;
}

export interface FeedbackRow {
  id: number;
  user_id: number;
  category: string;
  content: string;
  image_url: string;
  email: string;
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
  quota_ledger_id?: number | null;
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

export interface ReadingUsageStep {
  id: number;
  kind: string;
  kind_label: string;
  task: string;
  vendor_name: string;
  model_name: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  cached_tokens: number;
  reasoning_tokens: number;
  ok: boolean;
  created_at: string;
  price_minor: number | null;
  price_currency: string | null;
}

export interface ReadingUsageDetail {
  reading_id: number;
  question: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  price_minor: number | null;
  price_currency: string | null;
  steps: ReadingUsageStep[];
}

export function listReadings() {
  return requestClient.get<ReadingRow[]>('/readings');
}
export function getReadingUsages(id: number) {
  return requestClient.get<ReadingUsageDetail>(`/readings/${id}/usages`);
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

export function listUsers(params?: UserListQuery) {
  return requestClient.get<AdminPage<AppUser>>(`/users${toQuery(params)}`);
}
export function getUser(id: number) {
  return requestClient.get<{
    user: AppUser;
    recent_readings: Array<Record<string, unknown>>;
    recent_orders: Array<Record<string, unknown>>;
    notes?: string[];
  }>(`/users/${id}`);
}
export function getUserStats(days?: number) {
  return requestClient.get<UserStats>(`/users/stats${toQuery({ days })}`);
}
export function listAudit(params?: {
  operator_id?: number | string;
  action?: string;
  object_type?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}) {
  return requestClient.get<AdminPage<AuditRow>>(`/audit${toQuery(params)}`);
}
export function listMessages() {
  return requestClient.get<MessageRow[]>('/messages');
}
export function listWishlists() {
  return requestClient.get<WishlistRow[]>('/wishlists');
}
export function listFeedback() {
  return requestClient.get<FeedbackRow[]>('/feedback');
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
  quota: number;
  quota_bucket: 'pack' | 'subscription';
  reason?: string;
  note?: string;
}) {
  return requestClient.post<GrantList>('/grants', data);
}

export interface RedeemCode {
  id: number;
  code: string;
  kind: 'quota' | 'free_count' | string;
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

export interface InviteRelation {
  id: number;
  inviter_id: number;
  invitee_id: number;
  invite_code: string;
  source?: string;
  source_label?: string;
  created_at: string;
  inviter_total?: number | null;
  inviter_remaining?: number | null;
  inviter_granted_at?: string | null;
  inviter_expires_at?: string | null;
  invitee_total?: number | null;
  invitee_remaining?: number | null;
  invitee_granted_at?: string | null;
  invitee_expires_at?: string | null;
}

function userQuery(userId?: number) {
  return userId ? `?user_id=${userId}` : '';
}

export function listInvites(userId?: number) {
  return requestClient.get<{ items: InviteRelation[] }>(`/invites${userQuery(userId)}`);
}

export interface RegistrationGift {
  user_id: number;
  registered_at: string;
  granted_at?: string | null;
  usable_at?: string | null;
  expires_at?: string | null;
  total: number;
  remaining: number;
  used_at?: string | null;
}

export function listRegistrationGifts(userId?: number) {
  return requestClient.get<{ items: RegistrationGift[] }>(
    `/registration-gifts${userQuery(userId)}`,
  );
}

export interface QuotaLot {
  id: number;
  user_id: number;
  source: 'subscription' | 'pack' | 'admin';
  total: number;
  remaining: number;
  granted_at: string;
  expires_at?: string | null;
  order_id?: number | null;
  reason: string;
  expired: boolean;
}

export function listQuotaLots(userId?: number) {
  return requestClient.get<{ items: QuotaLot[] }>(`/quota-lots${userQuery(userId)}`);
}

export interface YearlyQualification {
  user_id: number;
  year: number;
  unlocked: boolean;
  unlock_source: string;
  unlocked_at?: string | null;
  report_status: string;
  generated_at?: string | null;
}

export interface DeviceGuide {
  device_id: string;
  guide_completed: boolean;
  device_trial_consumed: boolean;
  user_id?: number | null;
  reading_trial_used: boolean;
  created_at: string;
}

export function listOnboarding() {
  return requestClient.get<{ items: DeviceGuide[] }>('/onboarding');
}

export interface Membership {
  id: number;
  user_id: number;
  sku_kind: string;
  status: string;
  platform: string;
  starts_at: string;
  expires_at: string;
  active: boolean;
}

export interface MembershipList {
  items: Membership[];
  can_write?: boolean;
  writes_iap_orders?: boolean;
  sku_kind_default?: string;
  month_days?: number;
  year_days?: number;
  permission_note?: string;
  user_id?: number;
  extended?: boolean;
  duration_days?: number;
  membership?: Membership;
}

export function listMemberships(userId?: number) {
  return requestClient.get<MembershipList>(`/memberships${userQuery(userId)}`);
}

export function grantMembership(data: { user_id: number; sku_kind?: string; note: string }) {
  return requestClient.post<MembershipList>('/memberships', data);
}

export function listYearlyQualifications(userId?: number) {
  return requestClient.get<{ items: YearlyQualification[] }>(
    `/yearly-qualifications${userQuery(userId)}`,
  );
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
