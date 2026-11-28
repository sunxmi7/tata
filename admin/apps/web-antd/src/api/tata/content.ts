import { requestClient } from '#/api/request';

export interface Spread {
  id: number;
  name: string;
  name_en?: string;
  description: string;
  card_count: number;
  positions: Array<{ index: number; name: string; meaning: string }>;
  quick_questions?: string[];
  enabled: boolean;
  charged: boolean;
  recommended?: boolean;
  thumbnail?: string | null;
  preview?: string;
  theme_key?: string;
  theme_title?: string;
  theme_subtitle?: string;
  short_sentence?: string;
  depth_group?: string;
  depth_label?: string;
  quota_cost?: number | null;
  quota_priced?: boolean;
  price_ledger?: string | null;
}

export interface DeckProduct {
  code: string;
  kind: string;
  name: string;
  price_amount_minor: number;
  currency: string;
  deck_codes: string[];
  covers_future: boolean;
  deduct_owned: boolean;
  note: string;
  enabled: boolean;
  ledger: string;
}

export interface Tarotist {
  id: number;
  name: string;
  persona: string;
  display_copy: string;
  voice?: string;
  banned?: string;
  ask_style?: string;
  enabled: boolean;
}

export interface Banner {
  id: number;
  copy: string;
  start_at: string;
  end_at: string;
  priority: number;
  target?: string;
  enabled?: boolean;
  image_url?: string | null;
  frequency?: 'every_enter' | 'daily_once' | string;
}

export interface TataCard {
  id: number;
  card_id: number;
  name: string;
  card_name: string;
  name_zh_hant?: string;
  name_en?: string;
  name_ja?: string;
  name_ko?: string;
  arcana?: string;
  suit?: string | null;
  rank?: string | null;
  yi?: string | null;
  upright_message?: string | null;
  upright_yi?: string | null;
  reversed_message?: string | null;
  reversed_yi?: string | null;
  thumbnail?: string | null;
  card_thumbnail?: string;
  enabled: boolean;
  sort_order?: number;
}

export interface TataCardList {
  items: TataCard[];
  thumbnail_template?: string | null;
  thumbnail_default?: string | null;
}

export function listSpreads() {
  return requestClient.get<Spread[]>('/spreads');
}
export function createSpread(data: Partial<Spread>) {
  return requestClient.post('/spreads', data);
}
export function updateSpread(id: number, data: Partial<Spread>) {
  return requestClient.put(`/spreads/${id}`, data);
}

export function listDeckProducts() {
  return requestClient.get<DeckProduct[]>('/deck-products');
}

export function updateDeckProduct(
  code: string,
  data: Partial<Pick<DeckProduct, 'name' | 'price_amount_minor' | 'enabled' | 'note'>>,
) {
  return requestClient.put(`/deck-products/${code}`, data);
}

export function listTarotists() {
  return requestClient.get<Tarotist[]>('/tarotists');
}
export function createTarotist(data: Partial<Tarotist>) {
  return requestClient.post('/tarotists', data);
}
export function updateTarotist(id: number, data: Partial<Tarotist>) {
  return requestClient.put(`/tarotists/${id}`, data);
}

export function listBanners() {
  return requestClient.get<Banner[]>('/banners');
}
export function createBanner(data: Partial<Banner> & { push?: boolean; push_title?: string }) {
  return requestClient.post('/banners', data);
}
export function updateBanner(
  id: number,
  data: Partial<Banner> & { push?: boolean; push_title?: string },
) {
  return requestClient.put(`/banners/${id}`, data);
}

export function listCards() {
  return requestClient.get<TataCardList>('/cards');
}
export function createCard(data: Record<string, unknown>) {
  return requestClient.post('/cards', data);
}
export function updateCard(id: number, data: Record<string, unknown>) {
  return requestClient.put(`/cards/${id}`, data);
}
export function updateCardSettings(data: Record<string, unknown>) {
  return requestClient.put<TataCardList>('/card-settings', data);
}

export function getCopy(kind: string, year?: number) {
  const query = year ? `?year=${year}` : '';
  return requestClient.get<Record<string, unknown>>(`/copy/${kind}${query}`);
}
export function updateCopy(kind: string, data: Record<string, unknown>) {
  return requestClient.put<Record<string, unknown>>(`/copy/${kind}`, data);
}
export function createCopy(kind: string, data: Record<string, unknown>) {
  return requestClient.post<Record<string, unknown>>(`/copy/${kind}`, data);
}
export function deleteCopy(kind: string, id: string) {
  return requestClient.delete<Record<string, unknown>>(`/copy/${kind}/${id}`);
}
