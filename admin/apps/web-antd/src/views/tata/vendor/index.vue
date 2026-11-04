<script lang="ts" setup>
import type { Binding, Vendor } from '#/api/tata';

import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import { Button, Card, Form, Input, Modal, Select, Switch, Table, Tag, message } from 'ant-design-vue';

import { createVendor, listBindings, listVendors, probeVendor, putBinding } from '#/api/tata';

import {
  modelSelectOptions,
  modelSelectPlaceholder,
  nextModelAfterVendorChange,
  resolveBindForm,
  vendorSelectOptions,
} from './bind-form';

const tasks = [
  { label: '问题检测', value: 'detect' },
  { label: '问题改写', value: 'rewrite' },
  { label: '牌阵推荐', value: 'recommend' },
  { label: '解读', value: 'interpret' },
  { label: '塔罗师', value: 'tarotist' },
];
const taskLabelByValue = Object.fromEntries(tasks.map((item) => [item.value, item.label]));

const access = useAccessStore();
const vendors = ref<Vendor[]>([]);
const bindings = ref<Binding[]>([]);
const open = ref(false);
const form = ref({
  name: '演示模型',
  base_url: 'http://127.0.0.1:8080/demo/openai/v1',
  api_key: 'demo',
  models: 'demo',
  enabled: true,
});
const bind = ref({ task: 'interpret', vendor_id: undefined as number | undefined, model: '' });

const vendorNameById = computed(() => {
  const map = new Map<number, string>();
  for (const vendor of vendors.value) {
    map.set(vendor.id, vendor.name);
  }
  return map;
});

const selectedVendor = computed(() =>
  vendors.value.find((item) => item.id === bind.value.vendor_id),
);

const vendorOptions = computed(() => vendorSelectOptions(vendors.value));

const modelOptions = computed(() =>
  modelSelectOptions(selectedVendor.value, bind.value.model ?? ''),
);

const modelPlaceholder = computed(() =>
  modelSelectPlaceholder(bind.value.vendor_id, selectedVendor.value),
);

function vendorLabel(id?: number) {
  if (id === undefined || id === null) return '—';
  const name = vendorNameById.value.get(id);
  return name ? `${name} (#${id})` : `#${id}`;
}

function taskLabel(task: string) {
  return taskLabelByValue[task] ?? task;
}

function applyBinding(task: string) {
  bind.value = resolveBindForm(task, bindings.value);
}

function onTaskChange(task: string) {
  applyBinding(task);
}

function onVendorChange(value?: number | null) {
  bind.value.vendor_id = value ?? undefined;
  bind.value.model = nextModelAfterVendorChange(selectedVendor.value, bind.value.model ?? '');
}

function bindingRowProps(record: Binding) {
  return {
    onClick: () => {
      if (!canWrite()) return;
      applyBinding(record.task);
    },
    style: { cursor: canWrite() ? 'pointer' : undefined },
  };
}

function bindingRowClassName(record: Binding) {
  return record.task === bind.value.task ? 'ant-table-row-selected' : '';
}

async function load() {
  vendors.value = await listVendors();
  bindings.value = await listBindings();
}

function canWrite() {
  return access.accessCodes.includes('write');
}
async function save() {
  const created = await createVendor({
    name: form.value.name,
    base_url: form.value.base_url,
    api_key: form.value.api_key,
    models: form.value.models.split(',').map((item) => item.trim()).filter(Boolean),
    enabled: form.value.enabled,
  });
  message.success('已保存，密钥不会回显');
  open.value = false;
  bind.value.vendor_id = created.id;
  bind.value.model = '';
  await load();
}
async function bindTask() {
  if (!bind.value.vendor_id) {
    message.warning('请选择供应商');
    return;
  }
  const model = (bind.value.model ?? '').trim();
  if (!model) {
    message.warning('请选择模型');
    return;
  }
  await putBinding(bind.value.task, { vendor_id: bind.value.vendor_id, model });
  message.success('绑定已更新');
  await load();
  applyBinding(bind.value.task);
}
async function probe(id: number) {
  await probeVendor(id);
  message.success('连通');
}
onMounted(async () => {
  await load();
  applyBinding(bind.value.task);
});
</script>

<template>
  <Page description="OpenAI 兼容接口。密钥保存后不明文回显。" title="模型供应商">
    <template #extra>
      <Button v-if="canWrite()" type="primary" @click="open = true">新建</Button>
    </template>
    <Table
      :columns="[
        { title: '名称', dataIndex: 'name' },
        { title: '地址', dataIndex: 'base_url' },
        { title: '密钥', dataIndex: 'api_key_set' },
        { title: '启用', dataIndex: 'enabled' },
        { title: '操作', dataIndex: 'action' },
      ]"
      :data-source="vendors"
      row-key="id"
      :pagination="false"
      :locale="{ emptyText: '暂无供应商' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'api_key_set'">{{ record.api_key_set ? '已设置' : '未设置' }}</template>
        <Tag v-else-if="column.dataIndex === 'enabled'" :color="record.enabled ? 'green' : 'default'">
          {{ record.enabled ? '启用' : '停用' }}
        </Tag>
        <Button v-else-if="column.dataIndex === 'action' && canWrite()" type="link" @click="probe(record.id)">探测</Button>
      </template>
    </Table>
    <Card class="mt-4" title="调用绑定">
      <Table
        class="bind-table"
        :columns="[
          { title: '调用', dataIndex: 'task' },
          { title: '供应商', dataIndex: 'vendor_id' },
          { title: '模型', dataIndex: 'model' },
        ]"
        :custom-row="bindingRowProps"
        :data-source="bindings"
        :pagination="false"
        :row-class-name="bindingRowClassName"
        row-key="task"
        :locale="{ emptyText: '暂无绑定' }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'task'">{{ taskLabel(record.task) }}</template>
          <template v-else-if="column.dataIndex === 'vendor_id'">{{ vendorLabel(record.vendor_id) }}</template>
          <template v-else-if="column.dataIndex === 'model'">{{ record.model || '—' }}</template>
        </template>
      </Table>
      <!--
        Spacing must live outside <Form>. Ant Design's unlayered
        `.ant-form { margin: 0 }` zeros Tailwind mt-* on the form root
        (computed margin stays 0 even when class="mt-4" is present).
      -->
      <div v-if="canWrite()" class="bind-form-bar">
        <Form class="bind-form" layout="inline">
          <Form.Item label="调用">
            <Select
              v-model:value="bind.task"
              :options="tasks"
              placeholder="选择调用"
              style="width: 160px"
              @change="onTaskChange"
            />
          </Form.Item>
          <Form.Item label="供应商">
            <Select
              v-model:value="bind.vendor_id"
              :options="vendorOptions"
              allow-clear
              placeholder="选择供应商"
              style="min-width: 200px; width: 220px"
              @change="onVendorChange"
            />
          </Form.Item>
          <Form.Item label="模型">
            <Select
              v-model:value="bind.model"
              :disabled="bind.vendor_id == null || modelOptions.length === 0"
              :options="modelOptions"
              allow-clear
              :placeholder="modelPlaceholder"
              style="min-width: 200px; width: 220px"
            />
          </Form.Item>
          <Form.Item>
            <Button type="primary" @click="bindTask">绑定</Button>
          </Form.Item>
        </Form>
      </div>
    </Card>
    <Modal v-model:open="open" title="供应商" @ok="save">
      <Form layout="vertical">
        <Form.Item label="名称"><Input v-model:value="form.name" /></Form.Item>
        <Form.Item label="Base URL"><Input v-model:value="form.base_url" /></Form.Item>
        <Form.Item label="密钥"><Input.Password v-model:value="form.api_key" /></Form.Item>
        <Form.Item label="模型，逗号分隔"><Input v-model:value="form.models" /></Form.Item>
        <Form.Item label="启用"><Switch v-model:checked="form.enabled" /></Form.Item>
      </Form>
    </Modal>
  </Page>
</template>

<style scoped>
.bind-table :deep(.ant-table-thead > tr > th),
.bind-table :deep(.ant-table-tbody > tr > td) {
  padding-block: 12px;
}

.bind-form-bar {
  padding-top: 16px;
}

.bind-form {
  align-items: center;
  row-gap: 8px;
}

.bind-form :deep(.ant-form-item) {
  align-items: center;
  margin-bottom: 0;
}

.bind-form :deep(.ant-form-item-label),
.bind-form :deep(.ant-form-item-control-input) {
  display: flex;
  align-items: center;
}

.bind-form :deep(.ant-select-selector) {
  display: flex;
  align-items: center;
  min-height: var(--ant-control-height, 32px);
  padding-block: 4px;
}

.bind-form :deep(.ant-select-selection-item),
.bind-form :deep(.ant-select-selection-placeholder) {
  line-height: 1.375;
}
</style>
