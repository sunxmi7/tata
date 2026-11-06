<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import {
  Button,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Switch,
  Table,
  message,
} from 'ant-design-vue';

import { createIAPSku, listIAPSkus, updateIAPSku } from '#/api/tata';

const kindOptions = [
  {
    label: '会员',
    options: [
      { label: '月卡', value: 'month_subscription' },
      { label: '年卡', value: 'year_subscription' },
      { label: '次卡', value: 'reading_pack' },
    ],
  },
  {
    label: '塔塔币',
    options: [{ label: '塔塔币', value: 'consumable_pack' }],
  },
  {
    label: '年运',
    options: [{ label: '年运解锁', value: 'yearly_unlock' }],
  },
];

const kindLabel: Record<string, string> = {
  month_subscription: '会员 · 月卡',
  year_subscription: '会员 · 年卡',
  reading_pack: '会员 · 次卡',
  consumable_pack: '塔塔币',
  yearly_unlock: '年运解锁',
};

const access = useAccessStore();
const rows = ref<Record<string, unknown>[]>([]);
const filterKind = ref<string | undefined>();
const open = ref(false);
const editing = ref<Record<string, unknown> | null>(null);
const form = ref(emptyForm());

const canWrite = computed(() => access.accessCodes.includes('write'));

function emptyForm() {
  return {
    code: '',
    kind: 'consumable_pack',
    display_name: '',
    price_amount_minor: null as number | null,
    price_currency: 'CNY',
    diamond_grant: null as number | null,
    reading_grant: null as number | null,
    period_count: null as number | null,
    app_store_product_id: '',
    play_product_id: '',
    copy: '',
    enabled: true,
  };
}

async function load() {
  rows.value = await listIAPSkus(filterKind.value);
}

function showCreate() {
  editing.value = null;
  form.value = emptyForm();
  if (filterKind.value) {
    form.value.kind = filterKind.value;
  }
  open.value = true;
}

function showEdit(row: Record<string, unknown>) {
  editing.value = row;
  form.value = {
    code: String(row.code ?? ''),
    kind: String(row.kind ?? 'consumable_pack'),
    display_name: String(row.display_name ?? ''),
    price_amount_minor: (row.price_amount_minor as number | null) ?? null,
    price_currency: String(row.price_currency ?? 'CNY'),
    diamond_grant: (row.diamond_grant as number | null) ?? null,
    reading_grant: (row.reading_grant as number | null) ?? null,
    period_count: (row.period_count as number | null) ?? null,
    app_store_product_id: String(row.app_store_product_id ?? ''),
    play_product_id: String(row.play_product_id ?? ''),
    copy: String(row.copy ?? ''),
    enabled: row.enabled !== false,
  };
  open.value = true;
}

function payload() {
  const body: Record<string, unknown> = {
    display_name: form.value.display_name,
    app_store_product_id: form.value.app_store_product_id,
    play_product_id: form.value.play_product_id,
    price_currency: form.value.price_currency,
    copy: form.value.copy,
    enabled: form.value.enabled,
  };
  if (!editing.value) {
    body.code = form.value.code;
    body.kind = form.value.kind;
  }
  if (form.value.price_amount_minor != null) {
    body.price_amount_minor = form.value.price_amount_minor;
  }
  if (
    form.value.kind !== 'yearly_unlock' &&
    form.value.kind !== 'reading_pack' &&
    form.value.diamond_grant != null
  ) {
    body.diamond_grant = form.value.diamond_grant;
  }
  if (form.value.kind === 'reading_pack' && form.value.reading_grant != null) {
    body.reading_grant = form.value.reading_grant;
  }
  if (form.value.period_count != null) {
    body.period_count = form.value.period_count;
  }
  return body;
}

async function save() {
  if (editing.value) {
    await updateIAPSku(Number(editing.value.id), payload());
  } else {
    await createIAPSku(payload());
  }
  message.success('已保存');
  open.value = false;
  await load();
}

onMounted(load);
</script>

<template>
  <Page
    description="三类价位：会员（月卡 / 年卡 / 次卡）、塔塔币、年运解锁。价格单位为分。年卡「展示文案」即专属活动权益。"
    title="价位"
  >
    <template #extra>
      <div class="flex gap-2">
        <Select
          v-model:value="filterKind"
          allow-clear
          placeholder="全部类型"
          style="width: 160px"
          :options="kindOptions"
          @change="load"
        />
        <Button v-if="canWrite" type="primary" @click="showCreate">新建</Button>
      </div>
    </template>
    <Table
      :columns="[
        { title: '编码', dataIndex: 'code' },
        { title: '类型', dataIndex: 'kind' },
        { title: '名称', dataIndex: 'display_name' },
        { title: '价格（分）', dataIndex: 'price_amount_minor' },
        { title: '货币', dataIndex: 'price_currency' },
        { title: 'App Store 商品号', dataIndex: 'app_store_product_id' },
        { title: '文案', dataIndex: 'copy', ellipsis: true },
        { title: '钻石', dataIndex: 'diamond_grant' },
        { title: '解读次数', dataIndex: 'reading_grant' },
        { title: '启用', dataIndex: 'enabled' },
        { title: '操作', dataIndex: 'action' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="false"
      :locale="{ emptyText: '暂无价位' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'kind'">
          {{ kindLabel[String(record.kind)] || record.kind }}
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button v-if="canWrite" type="link" @click="showEdit(record)">编辑</Button>
        </template>
      </template>
    </Table>
    <Modal v-model:open="open" :title="editing ? '编辑价位' : '新建价位'" @ok="save">
      <Form layout="vertical">
        <Form.Item label="编码">
          <Input v-model:value="form.code" :disabled="!!editing" placeholder="例如 pack_60" />
        </Form.Item>
        <Form.Item label="类型">
          <Select v-model:value="form.kind" :disabled="!!editing" :options="kindOptions" />
        </Form.Item>
        <Form.Item label="名称">
          <Input v-model:value="form.display_name" />
        </Form.Item>
        <Form.Item label="价格（分）">
          <InputNumber v-model:value="form.price_amount_minor" :min="1" style="width: 100%" />
        </Form.Item>
        <Form.Item label="货币">
          <Select
            v-model:value="form.price_currency"
            :options="[
              { label: 'CNY', value: 'CNY' },
              { label: 'USD', value: 'USD' },
            ]"
          />
        </Form.Item>
        <Form.Item
          v-if="form.kind !== 'yearly_unlock' && form.kind !== 'reading_pack'"
          label="钻石发放"
        >
          <InputNumber v-model:value="form.diamond_grant" :min="1" style="width: 100%" />
        </Form.Item>
        <Form.Item v-if="form.kind === 'reading_pack'" label="解读次数">
          <InputNumber v-model:value="form.reading_grant" :min="1" style="width: 100%" />
        </Form.Item>
        <Form.Item v-if="form.kind === 'month_subscription' || form.kind === 'year_subscription'" label="周期数量">
          <InputNumber v-model:value="form.period_count" :min="1" style="width: 100%" />
        </Form.Item>
        <Form.Item label="App Store 商品号">
          <Input v-model:value="form.app_store_product_id" />
        </Form.Item>
        <Form.Item label="Play 商品号">
          <Input v-model:value="form.play_product_id" />
        </Form.Item>
        <Form.Item
          :label="form.kind === 'year_subscription' ? '专属活动权益文案' : '展示文案'"
        >
          <Input.TextArea v-model:value="form.copy" :rows="3" placeholder="空表示未定稿" />
        </Form.Item>
        <Form.Item label="启用">
          <Switch v-model:checked="form.enabled" />
        </Form.Item>
      </Form>
    </Modal>
  </Page>
</template>
