<script lang="ts" setup>
import type { Banner } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Button,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Switch,
  Table,
  Tag,
  message,
} from 'ant-design-vue';

import { createBanner, listBanners, updateBanner } from '#/api/tata';

import { hasTataWrite } from '../access';

const rows = ref<Banner[]>([]);
const open = ref(false);
const editingId = ref<number | null>(null);
const form = ref({
  copy: '',
  start_at: '2026-01-01T00:00:00+08:00',
  end_at: '2027-01-01T00:00:00+08:00',
  priority: 10,
  target: '',
  enabled: true,
  image_url: '',
  frequency: 'every_enter',
});

async function load() {
  rows.value = await listBanners();
}

function openCreate() {
  editingId.value = null;
  form.value = {
    copy: '',
    start_at: '2026-01-01T00:00:00+08:00',
    end_at: '2027-01-01T00:00:00+08:00',
    priority: 10,
    target: '',
    enabled: true,
    image_url: '',
    frequency: 'every_enter',
  };
  open.value = true;
}

function openEdit(row: Banner) {
  editingId.value = row.id;
  form.value = {
    copy: row.copy,
    start_at: row.start_at,
    end_at: row.end_at,
    priority: row.priority,
    target: row.target || '',
    enabled: row.enabled !== false,
    image_url: row.image_url || '',
    frequency: row.frequency || 'every_enter',
  };
  open.value = true;
}

async function save() {
  const body = { ...form.value };
  if (editingId.value) {
    await updateBanner(editingId.value, body);
  } else {
    await createBanner(body);
  }
  message.success('已保存');
  open.value = false;
  await load();
}

async function toggleEnabled(row: Banner, enabled: boolean) {
  await updateBanner(row.id, {
    copy: row.copy,
    start_at: row.start_at,
    end_at: row.end_at,
    priority: row.priority,
    target: row.target || '',
    image_url: row.image_url || '',
    frequency: row.frequency || 'every_enter',
    enabled,
  });
  message.success(enabled ? '已启用' : '已下架');
  await load();
}

onMounted(load);
</script>

<template>
  <Page
    description="配置文案、时间窗、启停与展示频率。曝光/点击见友盟，本页不编造数字。"
    title="首页提示窗"
  >
    <template #extra>
      <Button v-if="hasTataWrite('content')" type="primary" @click="openCreate">新建</Button>
    </template>
    <Table
      :columns="[
        { title: '文案', dataIndex: 'copy' },
        { title: '频率', dataIndex: 'frequency', width: 120 },
        { title: '优先级', dataIndex: 'priority', width: 90 },
        { title: '开始', dataIndex: 'start_at', width: 200 },
        { title: '结束', dataIndex: 'end_at', width: 200 },
        { title: '启停', dataIndex: 'enabled', width: 90 },
        { title: '操作', dataIndex: 'action', width: 90 },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="false"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'frequency'">
          {{ record.frequency === 'daily_once' ? '每天一次' : '每次进入' }}
        </template>
        <template v-else-if="column.dataIndex === 'enabled'">
          <Switch
            v-if="hasTataWrite('content')"
            :checked="record.enabled !== false"
            @change="(v: any) => toggleEnabled(record as Banner, !!v)"
          />
          <Tag v-else :color="record.enabled !== false ? 'green' : 'default'">
            {{ record.enabled !== false ? '启用' : '下架' }}
          </Tag>
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button v-if="hasTataWrite('content')" type="link" @click="openEdit(record as Banner)">
            编辑
          </Button>
        </template>
      </template>
    </Table>
    <p class="mt-3 text-sm text-gray-500">曝光与点击请到友盟查看自定义事件，后台只负责配置下发。</p>
    <Modal
      v-model:open="open"
      :title="editingId ? '编辑提示窗' : '新建提示窗'"
      @ok="save"
    >
      <Form layout="vertical">
        <Form.Item label="文案"><Input v-model:value="form.copy" /></Form.Item>
        <Form.Item label="开始"><Input v-model:value="form.start_at" /></Form.Item>
        <Form.Item label="结束"><Input v-model:value="form.end_at" /></Form.Item>
        <Form.Item label="优先级"><InputNumber v-model:value="form.priority" /></Form.Item>
        <Form.Item label="跳转"><Input v-model:value="form.target" /></Form.Item>
        <Form.Item label="素材 URL"><Input v-model:value="form.image_url" /></Form.Item>
        <Form.Item label="展示频率">
          <Select
            v-model:value="form.frequency"
            :options="[
              { value: 'every_enter', label: '每次进入' },
              { value: 'daily_once', label: '每天一次' },
            ]"
          />
        </Form.Item>
        <Form.Item label="启用"><Switch v-model:checked="form.enabled" /></Form.Item>
      </Form>
    </Modal>
  </Page>
</template>
