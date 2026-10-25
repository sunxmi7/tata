<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import { Button, Form, Input, InputNumber, Modal, Table, message } from 'ant-design-vue';

import { createBanner, listBanners } from '#/api/tata';

const access = useAccessStore();
const rows = ref<Record<string, unknown>[]>([]);
const open = ref(false);
const form = ref({
  copy: '',
  start_at: '2026-01-01T00:00:00+08:00',
  end_at: '2027-01-01T00:00:00+08:00',
  priority: 10,
  target: '',
});
async function load() {
  rows.value = await listBanners();
}
async function save() {
  await createBanner(form.value);
  message.success('已保存');
  open.value = false;
  await load();
}
onMounted(load);
</script>

<template>
  <Page description="按北京时间和优先级下发。" title="首页提示窗">
    <template #extra>
      <Button v-if="access.accessCodes.includes('write')" type="primary" @click="open = true">新建</Button>
    </template>
    <Table
      :columns="[
        { title: '文案', dataIndex: 'copy' },
        { title: '优先级', dataIndex: 'priority' },
        { title: '开始', dataIndex: 'start_at' },
        { title: '结束', dataIndex: 'end_at' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="false"
    />
    <Modal v-model:open="open" title="提示窗" @ok="save">
      <Form layout="vertical">
        <Form.Item label="文案"><Input v-model:value="form.copy" /></Form.Item>
        <Form.Item label="开始"><Input v-model:value="form.start_at" /></Form.Item>
        <Form.Item label="结束"><Input v-model:value="form.end_at" /></Form.Item>
        <Form.Item label="优先级"><InputNumber v-model:value="form.priority" /></Form.Item>
        <Form.Item label="跳转"><Input v-model:value="form.target" /></Form.Item>
      </Form>
    </Modal>
  </Page>
</template>
