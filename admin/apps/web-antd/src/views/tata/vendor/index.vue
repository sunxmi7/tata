<script lang="ts" setup>
import type { Binding, Vendor } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import { Button, Card, Form, Input, Modal, Select, Switch, Table, Tag, message } from 'ant-design-vue';

import { createVendor, listBindings, listVendors, probeVendor, putBinding } from '#/api/tata';

const tasks = [
  { label: '问题检测', value: 'detect' },
  { label: '问题改写', value: 'rewrite' },
  { label: '牌阵推荐', value: 'recommend' },
  { label: '解读', value: 'interpret' },
  { label: '塔罗师', value: 'tarotist' },
];
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
const bind = ref({ task: 'interpret', vendor_id: undefined as number | undefined, model: 'demo' });

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
  await load();
}
async function bindTask() {
  if (!bind.value.vendor_id) return;
  await putBinding(bind.value.task, { vendor_id: bind.value.vendor_id, model: bind.value.model });
  message.success('绑定已更新');
  await load();
}
async function probe(id: number) {
  await probeVendor(id);
  message.success('连通');
}
onMounted(load);
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
        :columns="[
          { title: '调用', dataIndex: 'task' },
          { title: '供应商', dataIndex: 'vendor_id' },
          { title: '模型', dataIndex: 'model' },
        ]"
        :data-source="bindings"
        :pagination="false"
        row-key="task"
      />
      <Form v-if="canWrite()" class="mt-4" layout="inline">
        <Form.Item label="调用">
          <Select v-model:value="bind.task" :options="tasks" style="width: 140px" />
        </Form.Item>
        <Form.Item label="供应商">
          <Select
            v-model:value="bind.vendor_id"
            :options="vendors.map((item) => ({ label: item.name, value: item.id }))"
            style="width: 160px"
          />
        </Form.Item>
        <Form.Item label="模型"><Input v-model:value="bind.model" /></Form.Item>
        <Button type="primary" @click="bindTask">绑定</Button>
      </Form>
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
