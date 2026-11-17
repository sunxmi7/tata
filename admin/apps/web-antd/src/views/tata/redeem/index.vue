<script lang="ts" setup>
import type { RedeemCode } from '#/api/tata';

import { computed, onMounted, ref } from 'vue';

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
  message,
} from 'ant-design-vue';

import {
  createRedeemCodes,
  listRedeemCodes,
  updateRedeemCode,
} from '#/api/tata';

import { hasTataWrite } from '../access';

const rows = ref<RedeemCode[]>([]);
const note = ref('');
const open = ref(false);
const editing = ref<RedeemCode | null>(null);
const form = ref(emptyForm());
const canWrite = computed(() => hasTataWrite('pay'));

function emptyForm() {
  return {
    kind: 'diamonds',
    amount: null as number | null,
    benefit_name: '',
    code: '',
    count: 1,
    max_redemptions: 1 as number | null,
    expires_at: '',
    enabled: true,
  };
}

async function load() {
  const data = await listRedeemCodes();
  rows.value = data.items || [];
  note.value = data.permission_note || '';
}

function showCreate() {
  editing.value = null;
  form.value = emptyForm();
  open.value = true;
}

function showEdit(row: RedeemCode) {
  editing.value = row;
  form.value = {
    kind: String(row.kind || 'diamonds'),
    amount: row.amount,
    benefit_name: String(row.benefit_name || ''),
    code: String(row.code || ''),
    count: 1,
    max_redemptions: row.max_redemptions,
    expires_at: String(row.expires_at || ''),
    enabled: row.enabled !== false,
  };
  open.value = true;
}

async function copyCode(code: string) {
  await navigator.clipboard.writeText(code);
  message.success('已复制');
}

async function save() {
  if (editing.value) {
    await updateRedeemCode(Number(editing.value.id), {
      enabled: form.value.enabled,
      benefit_name: form.value.benefit_name,
      max_redemptions: form.value.max_redemptions ?? 0,
      expires_at: form.value.expires_at.trim(),
    });
    message.success('已保存');
  } else {
    const body: Record<string, unknown> = {
      kind: form.value.kind,
      amount: form.value.amount,
      benefit_name: form.value.benefit_name,
      count: form.value.count || 1,
      enabled: form.value.enabled,
    };
    const custom = form.value.code.trim();
    if (custom) {
      body.code = custom;
    }
    if (form.value.max_redemptions) {
      body.max_redemptions = form.value.max_redemptions;
    }
    const expires = form.value.expires_at.trim();
    if (expires) {
      body.expires_at = expires;
    }
    const created = await createRedeemCodes(body as Parameters<typeof createRedeemCodes>[0]);
    const codes = (created.items || []).map((item) => item.code).join('、');
    message.success(codes ? `已生成：${codes}` : '已生成');
  }
  open.value = false;
  await load();
}

onMounted(load);
</script>

<template>
  <Page
    description="生成后用户可在 App 兑换码页核销。空码则自动生成 XXXX-XXXX。不填核销上限表示不限人数。付费模块可写才能生成或停用。"
    title="兑换码"
  >
    <template #extra>
      <Button v-if="canWrite" type="primary" @click="showCreate">生成兑换码</Button>
    </template>
    <p v-if="note" class="mb-4 text-sm opacity-70">{{ note }}</p>
    <Table
      :columns="[
        { title: '兑换码', dataIndex: 'code' },
        { title: '类型', dataIndex: 'kind', width: 120 },
        { title: '数量', dataIndex: 'amount', width: 80 },
        { title: '权益名', dataIndex: 'benefit_name' },
        { title: '已核销 / 上限', dataIndex: 'usage', width: 140 },
        { title: '过期', dataIndex: 'expires_at' },
        { title: '启用', dataIndex: 'enabled', width: 80 },
        { title: '创建时间', dataIndex: 'created_at' },
        { title: '操作', dataIndex: 'action', width: 140 },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
      :locale="{ emptyText: '暂无兑换码' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'kind'">
          {{ record.kind === 'free_count' ? '免费次数' : '额度' }}
        </template>
        <template v-else-if="column.dataIndex === 'usage'">
          {{ record.redeemed_count }}
          /
          {{ record.max_redemptions == null ? '不限' : record.max_redemptions }}
        </template>
        <template v-else-if="column.dataIndex === 'expires_at'">
          {{ record.expires_at || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'enabled'">
          {{ record.enabled ? '是' : '否' }}
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button type="link" @click="copyCode(String(record.code))">复制</Button>
          <Button v-if="canWrite" type="link" @click="showEdit(record as RedeemCode)">编辑</Button>
        </template>
      </template>
    </Table>
    <Modal
      v-model:open="open"
      :title="editing ? '编辑兑换码' : '生成兑换码'"
      @ok="save"
    >
      <Form layout="vertical">
        <Form.Item label="类型">
          <Select
            v-model:value="form.kind"
            :disabled="!!editing"
            :options="[
              { label: '额度', value: 'diamonds' },
              { label: '免费次数', value: 'free_count' },
            ]"
          />
        </Form.Item>
        <Form.Item label="数量">
          <InputNumber
            v-model:value="form.amount"
            :disabled="!!editing"
            :min="1"
            style="width: 100%"
          />
        </Form.Item>
        <Form.Item label="权益名称">
          <Input v-model:value="form.benefit_name" placeholder="例如 新春礼" />
        </Form.Item>
        <Form.Item v-if="!editing" label="自定义码（可空，空则自动生成）">
          <Input v-model:value="form.code" placeholder="仅 count=1 时可填" />
        </Form.Item>
        <Form.Item v-if="!editing" label="生成个数">
          <InputNumber v-model:value="form.count" :min="1" :max="100" style="width: 100%" />
        </Form.Item>
        <Form.Item label="核销上限（空或 0 表示不限）">
          <InputNumber v-model:value="form.max_redemptions" :min="0" style="width: 100%" />
        </Form.Item>
        <Form.Item label="过期时间 RFC3339（可空）">
          <Input v-model:value="form.expires_at" placeholder="2026-12-31T16:00:00Z" />
        </Form.Item>
        <Form.Item label="启用">
          <Switch v-model:checked="form.enabled" />
        </Form.Item>
      </Form>
    </Modal>
  </Page>
</template>
