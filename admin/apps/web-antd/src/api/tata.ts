import { requestClient } from '#/api/request';

export interface Spread {
  id: number;
  name: string;
  description: string;
  card_count: number;
  positions: Array<{ index: number; name: string; meaning: string }>;
  enabled: boolean;
  charged: boolean;
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

export interface Vendor {
  id: number;
  name: string;
  base_url: string;
  api_key_set: boolean;
  models: string[];
  timeout_sec?: number;
  enabled: boolean;
}

export interface Binding {
  task: string;
  vendor_id?: number;
  model: string;
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

export function listTarotists() {
  return requestClient.get<Tarotist[]>('/tarotists');
}
export function createTarotist(data: Partial<Tarotist>) {
  return requestClient.post('/tarotists', data);
}
export function updateTarotist(id: number, data: Partial<Tarotist>) {
  return requestClient.put(`/tarotists/${id}`, data);
}

export function listVendors() {
  return requestClient.get<Vendor[]>('/vendors');
}
export function createVendor(data: Record<string, unknown>) {
  return requestClient.post<{ id: number }>('/vendors', data);
}
export function updateVendor(id: number, data: Record<string, unknown>) {
  return requestClient.put(`/vendors/${id}`, data);
}
export function probeVendor(id: number) {
  return requestClient.post(`/vendors/${id}/probe`);
}
export function listBindings() {
  return requestClient.get<Binding[]>('/bindings');
}
export function putBinding(task: string, data: { vendor_id: number; model: string }) {
  return requestClient.put(`/bindings/${task}`, data);
}

export function getQuota() {
  return requestClient.get<Record<string, unknown>>('/quota-params');
}
export function listBanners() {
  return requestClient.get<Record<string, unknown>[]>('/banners');
}
export function createBanner(data: Record<string, unknown>) {
  return requestClient.post('/banners', data);
}
export function listReadings() {
  return requestClient.get<Record<string, unknown>[]>('/readings');
}
export function retryReading(id: number) {
  return requestClient.post(`/readings/${id}/retry`);
}
export function listLedger() {
  return requestClient.get<Record<string, unknown>[]>('/ledger');
}
export function listOperators() {
  return requestClient.get<Record<string, unknown>[]>('/operators');
}
export function listUsers() {
  return requestClient.get<Record<string, unknown>[]>('/users');
}
export function listAudit() {
  return requestClient.get<Record<string, unknown>[]>('/audit');
}
export function listMessages() {
  return requestClient.get<Record<string, unknown>[]>('/messages');
}
export function listCalendar() {
  return requestClient.get<Record<string, unknown>[]>('/calendar');
}
