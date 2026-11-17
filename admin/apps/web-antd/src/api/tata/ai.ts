import { requestClient } from '#/api/request';

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
export function updateQuota(data: Record<string, unknown>) {
  return requestClient.put<Record<string, unknown>>('/quota-params', data);
}
