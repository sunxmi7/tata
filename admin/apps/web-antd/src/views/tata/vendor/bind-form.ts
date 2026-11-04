export type BindFormState = {
  task: string;
  vendor_id: number | undefined;
  model: string;
};

export type BindingLike = {
  task: string;
  vendor_id?: null | number;
  model?: null | string;
};

export type VendorLike = {
  id: number;
  name: string;
  enabled: boolean;
  models?: null | string[];
};

export function vendorModels(vendor?: VendorLike) {
  const models = vendor?.models;
  return Array.isArray(models) ? models.filter(Boolean) : [];
}

export function resolveBindForm(task: string, bindings: BindingLike[]): BindFormState {
  const current = bindings.find((item) => item.task === task);
  if (current?.vendor_id) {
    return { task, vendor_id: current.vendor_id, model: current.model ?? '' };
  }
  return { task, vendor_id: undefined, model: '' };
}

export function vendorSelectOptions(vendors: VendorLike[]) {
  return [...vendors]
    .sort((a, b) => Number(b.enabled) - Number(a.enabled) || a.id - b.id)
    .map((item) => ({
      label: item.enabled ? item.name : `${item.name}（停用）`,
      value: item.id,
    }));
}

export function modelSelectOptions(vendor: VendorLike | undefined, currentModel: string) {
  const models = vendorModels(vendor);
  const options = models.map((item) => ({ label: item, value: item }));
  const current = (currentModel ?? '').trim();
  if (current && !models.includes(current)) {
    options.unshift({ label: current, value: current });
  }
  return options;
}

export function modelSelectPlaceholder(vendorId?: null | number, vendor?: VendorLike) {
  if (vendorId === undefined || vendorId === null) return '请先选择供应商';
  if (vendorModels(vendor).length === 0) return '该供应商暂无模型';
  return '选择模型';
}

export function nextModelAfterVendorChange(vendor: VendorLike | undefined, currentModel: string) {
  const models = vendorModels(vendor);
  return currentModel && models.includes(currentModel) ? currentModel : '';
}
