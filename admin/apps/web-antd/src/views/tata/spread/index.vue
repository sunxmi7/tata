<script lang="ts" setup>
import type { Spread } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import {
  Button,
  Form,
  Input,
  InputNumber,
  Modal,
  Space,
  Switch,
  Table,
  Tag,
  message,
} from 'ant-design-vue';

import { createSpread, listSpreads, updateSpread } from '#/api/tata';

const access = useAccessStore();
const rows = ref<Spread[]>([]);
const open = ref(false);
const editing = ref<null | Spread>(null);
const form = ref({
  name: '',
  description: '',
  card_count: 3,
  positionsText:
    '[{"index":1,"name":"过去","meaning":""},{"index":2,"name":"现在","meaning":""},{"index":3,"name":"未来","meaning":""}]',
  enabled: true,
  charged: false,
});

const columns = [
  { title: '名称', dataIndex: 'name' },
  { title: '牌数', dataIndex: 'card_count' },
  { title: '启用', dataIndex: 'enabled' },
  { title: '收费', dataIndex: 'charged' },
  { title: '操作', dataIndex: 'action' },
];

async function load() {
  rows.value = await listSpreads();
}

function canWrite() {
  return access.accessCodes.includes('write');
}

function showCreate() {
  editing.value = null;
  form.value = {
    name: '',
    description: '',
    card_count: 3,
    positionsText:
      '[{"index":1,"name":"过去","meaning":""},{"index":2,"name":"现在","meaning":""},{"index":3,"name":"未来","meaning":""}]',
    enabled: true,
    charged: false,
  };
  open.value = true;
}

function showEdit(row: Spread) {
  editing.value = row;
  form.value = {
    name: row.name,
    description: row.description,
    card_count: row.card_count,
    positionsText: JSON.stringify(row.positions, null, 2),
    enabled: row.enabled,
    charged: row.charged,
  };
  open.value = true;
}

async function save() {
  const positions = JSON.parse(form.value.positionsText);
  const body = {
    name: form.value.name,
    description: form.value.description,
    card_count: form.value.card_count,
    positions,
    enabled: form.value.enabled,
    charged: form.value.charged,
  };
  if (editing.value) {
    await updateSpread(editing.value.id, body);
  } else {
    await createSpread(body);
  }
  message.success('已保存');
  open.value = false;
  await load();
}

onMounted(load);
</script>

<template>
  <Page description="牌阵配置。没有价格字段。" title="牌阵">
    <template #extra>
      <Button v-if="canWrite()" type="primary" @click="showCreate">新建</Button>
    </template>
    <Table :columns="columns" :data-source="rows" row-key="id" :pagination="false">
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'enabled'">
          <Tag :color="record.enabled ? 'green' : 'default'">
            {{ record.enabled ? '启用' : '停用' }}
          </Tag>
        </template>
        <template v-else-if="column.dataIndex === 'charged'">
          {{ record.charged ? '是' : '否' }}
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button v-if="canWrite()" type="link" @click="showEdit(record)">编辑</Button>
        </template>
      </template>
    </Table>
    <Modal v-model:open="open" title="牌阵" @ok="save">
      <Form layout="vertical">
        <Form.Item label="名称"><Input v-model:value="form.name" /></Form.Item>
        <Form.Item label="描述"><Input v-model:value="form.description" /></Form.Item>
        <Form.Item label="牌数"><InputNumber v-model:value="form.card_count" :min="1" /></Form.Item>
        <Form.Item label="牌位 JSON"><Input.TextArea v-model:value="form.positionsText" :rows="6" /></Form.Item>
        <Space>
          <span>启用</span><Switch v-model:checked="form.enabled" />
          <span>是否收费</span><Switch v-model:checked="form.charged" />
        </Space>
      </Form>
    </Modal>
  </Page>
</template>
