import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  modelSelectOptions,
  modelSelectPlaceholder,
  nextModelAfterVendorChange,
  resolveBindForm,
  vendorModels,
  vendorSelectOptions,
} from './bind-form.ts';

const vendors = [
  { id: 1, name: '旧供应商', enabled: true, models: ['demo'] },
  { id: 24, name: '本机演示模型', enabled: true, models: ['demo'] },
  { id: 51, name: 'eino-demo', enabled: true, models: ['demo'] },
  { id: 9, name: '停用供应商', enabled: false, models: [] },
];

const bindings = [
  { task: 'detect', vendor_id: 51, model: 'demo' },
  { task: 'rewrite', vendor_id: 51, model: 'demo' },
  { task: 'recommend', vendor_id: 51, model: 'demo' },
  { task: 'interpret', vendor_id: 51, model: 'demo' },
  { task: 'tarotist', vendor_id: 24, model: 'demo' },
];

describe('resolveBindForm', () => {
  it('fills vendor and model from the selected task binding', () => {
    assert.deepEqual(resolveBindForm('tarotist', bindings), {
      task: 'tarotist',
      vendor_id: 24,
      model: 'demo',
    });
    assert.deepEqual(resolveBindForm('interpret', bindings), {
      task: 'interpret',
      vendor_id: 51,
      model: 'demo',
    });
  });

  it('does not fall back to the first vendor when a task has no binding', () => {
    assert.deepEqual(resolveBindForm('interpret', [{ task: 'interpret', vendor_id: null, model: '' }]), {
      task: 'interpret',
      vendor_id: undefined,
      model: '',
    });
  });
});

describe('vendorSelectOptions', () => {
  it('keeps enabled vendors first and does not invent a default selection', () => {
    const options = vendorSelectOptions(vendors);
    assert.equal(options[0]?.value, 1);
    assert.deepEqual(
      options.map((item) => item.value),
      [1, 24, 51, 9],
    );
    assert.equal(
      options.find((item) => item.value === 9)?.label,
      '停用供应商（停用）',
    );
  });
});

describe('model select', () => {
  it('lists the chosen vendor models and keeps a leftover current value', () => {
    assert.deepEqual(vendorModels(vendors[2]), ['demo']);
    assert.deepEqual(modelSelectOptions(vendors[2], ''), [{ label: 'demo', value: 'demo' }]);
    assert.deepEqual(modelSelectOptions(vendors[2], 'legacy'), [
      { label: 'legacy', value: 'legacy' },
      { label: 'demo', value: 'demo' },
    ]);
  });

  it('shows a clear empty state and does not keep a model the new vendor lacks', () => {
    assert.equal(modelSelectPlaceholder(undefined), '请先选择供应商');
    assert.equal(modelSelectPlaceholder(9, vendors[3]), '该供应商暂无模型');
    assert.equal(modelSelectPlaceholder(51, vendors[2]), '选择模型');
    assert.equal(nextModelAfterVendorChange(vendors[2], 'demo'), 'demo');
    assert.equal(nextModelAfterVendorChange(vendors[3], 'demo'), '');
  });
});
