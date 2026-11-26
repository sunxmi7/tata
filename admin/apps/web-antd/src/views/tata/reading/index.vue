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
function statusText(row: ReadingRow) {
  const label = String(row.stage_label ?? '').trim();
  if (label && label !== row.status) {
    return label;
  }
  return statusLabel[row.status] ?? row.status;
}
function priceText(value: number | null | undefined) {
  return value == null ? '未配置' : formatTokens(value);
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
    description="一行是一条问题会话，标题用问题。输入、输出、合计是这次会话全部步骤相加。点明细看每一步。模型单价还没配置，价格显示未配置，不估算金额。"
    title="占卜记录"
  >
    <Table
      :columns="[
        { title: '问题', dataIndex: 'question', ellipsis: true },
        { title: '状态', dataIndex: 'stage_label', width: 120 },
        { title: '输入 token', dataIndex: 'prompt_tokens', align: 'right', width: 120 },
        { title: '输出 token', dataIndex: 'completion_tokens', align: 'right', width: 120 },
        { title: '合计 token', dataIndex: 'total_tokens', align: 'right', width: 120 },
        { title: '操作', dataIndex: 'action', width: 140 },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'question'">
          {{ questionTitle(record.question) }}
        </template>
        <template v-else-if="column.dataIndex === 'stage_label'">
          {{ statusText(record as ReadingRow) }}
        </template>
        <template
          v-else-if="column.dataIndex === 'prompt_tokens' || column.dataIndex === 'completion_tokens' || column.dataIndex === 'total_tokens'"
        >
          {{ formatTokens(record[column.dataIndex]) }}
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
      width="860"
    >
      <p class="mb-3 text-sm text-gray-500">
        合计 {{ formatTokens(detail?.total_tokens) }} token（输入 {{ formatTokens(detail?.prompt_tokens) }}，输出 {{ formatTokens(detail?.completion_tokens) }}）。价格 {{ priceText(detail?.price_minor) }}。
      </p>
      <Table
        :columns="[
          { title: '步骤', dataIndex: 'kind_label', width: 120 },
          { title: '模型', dataIndex: 'model_name' },
          { title: '输入 token', dataIndex: 'prompt_tokens', align: 'right', width: 110 },
          { title: '输出 token', dataIndex: 'completion_tokens', align: 'right', width: 110 },
          { title: '合计 token', dataIndex: 'total_tokens', align: 'right', width: 110 },
          { title: '价格', dataIndex: 'price_minor', width: 100 },
        ]"
        :data-source="detail?.steps ?? []"
        :loading="loading"
        :locale="{ emptyText: '这次会话还没有模型调用' }"
        :pagination="false"
        row-key="id"
        size="small"
      >
        <template #bodyCell="{ column, record }">
          <template
            v-if="column.dataIndex === 'prompt_tokens' || column.dataIndex === 'completion_tokens' || column.dataIndex === 'total_tokens'"
          >
            {{ formatTokens(record[column.dataIndex]) }}
          </template>
          <template v-else-if="column.dataIndex === 'price_minor'">
            {{ priceText(record.price_minor) }}
          </template>
          <template v-else-if="column.dataIndex === 'model_name'">
            {{ record.model_name || '—' }}
          </template>
        </template>
      </Table>
    </Drawer>
  </Page>
</template>
