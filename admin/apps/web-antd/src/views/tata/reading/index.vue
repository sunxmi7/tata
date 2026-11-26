<script lang="ts" setup>
import type { ReadingRow, ReadingUsageDetail } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Drawer, Table, message } from 'ant-design-vue';

import { getReadingUsages, listReadings, retryReading } from '#/api/tata';

import { hasTataWrite } from '../access';

const rows = ref<ReadingRow[]>([]);
const open = ref(false);
const loading = ref(false);
const detail = ref<ReadingUsageDetail | null>(null);

function formatTokens(value: unknown) {
  const n = Number(value ?? 0);
  return Number.isFinite(n) ? n.toLocaleString('zh-CN') : '0';
}
function questionTitle(question: unknown) {
  const text = String(question ?? '').trim();
  return text || '（未填写问题）';
}
const typeFallback: Record<string, string> = {
  normal: '普通占卜',
  explore: '探索',
  daily: '今日一牌',
  tarotist: '占卜师',
};
const statusLabel: Record<string, string> = {
  QUESTION_CREATED: '问题已提交',
  SPREAD_SELECTED: '牌阵已选择',
  SHUFFLING: '洗牌中',
  CUTTING: '切牌中',
  CARDS_FIXED: '牌面已固定',
  INTERPRETING: '解读生成中',
  COMPLETED: '已完成',
  FAILED_RETRYABLE: '失败可重试',
};
function sessionTypeText(row: ReadingRow) {
  const label = String(row.conversation_label ?? '').trim();
  if (label) return label;
  return typeFallback[row.type] ?? row.type ?? '—';
}
function statusText(row: ReadingRow) {
  const label = String(row.stage_label ?? '').trim();
  if (label && label !== row.status) {
    return label;
  }
  return statusLabel[row.status] ?? row.status;
}
function userText(row: ReadingRow) {
  const name = String(row.user_display ?? '').trim();
  if (name) return name;
  if (row.user_id) return `#${row.user_id}`;
  return '未登录';
}
function createdText(value: unknown) {
  if (!value) return '—';
  const d = new Date(String(value));
  if (Number.isNaN(d.getTime())) return String(value);
  return d.toLocaleString('zh-CN', { hour12: false });
}
function priceText(
  value: number | null | undefined,
  currency?: string | null,
  totalTokens?: number | null,
) {
  if (value == null) {
    if (!totalTokens) return '—';
    return '未配置';
  }
  const yuan = value / 1_000_000;
  const amount = yuan.toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 6,
  });
  if ((currency ?? 'CNY') === 'CNY') return `¥${amount}`;
  return `${amount} ${currency}`;
}
async function load() {
  rows.value = await listReadings();
}
async function retry(id: number) {
  await retryReading(id);
  message.success('已重试');
  await load();
}
async function showSteps(row: ReadingRow) {
  open.value = true;
  loading.value = true;
  detail.value = null;
  try {
    detail.value = await getReadingUsages(row.id);
  } finally {
    loading.value = false;
  }
}
onMounted(load);
</script>

<template>
  <Page
    description="一行是一条问题会话。列表看用户、类型、问题、总 token、总价与创建时间；点明细看每一步的步骤名、token 与价格。"
    title="占卜记录"
  >
    <Table
      :columns="[
        { title: '用户名', dataIndex: 'user_display', width: 140, ellipsis: true },
        { title: '会话类型', dataIndex: 'conversation_label', width: 110 },
        { title: '问题', dataIndex: 'question', ellipsis: true },
        { title: '状态', dataIndex: 'stage_label', width: 120 },
        { title: '总 token', dataIndex: 'total_tokens', align: 'right', width: 110 },
        { title: '总价格', dataIndex: 'price_minor', align: 'right', width: 120 },
        { title: '创建时间', dataIndex: 'created_at', width: 180 },
        { title: '操作', dataIndex: 'action', width: 140 },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'user_display'">
          {{ userText(record as ReadingRow) }}
        </template>
        <template v-else-if="column.dataIndex === 'conversation_label'">
          {{ sessionTypeText(record as ReadingRow) }}
        </template>
        <template v-else-if="column.dataIndex === 'question'">
          {{ questionTitle(record.question) }}
        </template>
        <template v-else-if="column.dataIndex === 'stage_label'">
          {{ statusText(record as ReadingRow) }}
        </template>
        <template v-else-if="column.dataIndex === 'total_tokens'">
          {{ formatTokens(record.total_tokens) }}
        </template>
        <template v-else-if="column.dataIndex === 'price_minor'">
          {{ priceText(record.price_minor, record.price_currency, record.total_tokens) }}
        </template>
        <template v-else-if="column.dataIndex === 'created_at'">
          {{ createdText(record.created_at) }}
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button type="link" @click="showSteps(record as ReadingRow)">明细</Button>
          <Button
            v-if="hasTataWrite('records') && record.status === 'FAILED_RETRYABLE'"
            type="link"
            @click="retry(record.id)"
          >
            重试
          </Button>
        </template>
      </template>
    </Table>
    <Drawer
      v-model:open="open"
      :title="questionTitle(detail?.question)"
      width="720"
    >
      <p class="mb-3 text-sm text-gray-500">
        合计 {{ formatTokens(detail?.total_tokens) }} token，价格 {{ priceText(detail?.price_minor, detail?.price_currency, detail?.total_tokens) }}。
      </p>
      <Table
        :columns="[
          { title: '步骤', dataIndex: 'kind_label' },
          { title: 'token', dataIndex: 'total_tokens', align: 'right', width: 120 },
          { title: '价格', dataIndex: 'price_minor', align: 'right', width: 140 },
        ]"
        :data-source="detail?.steps ?? []"
        :loading="loading"
        :locale="{ emptyText: '这次会话还没有模型调用' }"
        :pagination="false"
        row-key="id"
        size="small"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'total_tokens'">
            {{ formatTokens(record.total_tokens) }}
          </template>
          <template v-else-if="column.dataIndex === 'price_minor'">
            {{ priceText(record.price_minor, record.price_currency, record.total_tokens) }}
          </template>
        </template>
      </Table>
    </Drawer>
  </Page>
</template>
