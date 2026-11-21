<script lang="ts" setup>
import type { LedgerRow } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table } from 'ant-design-vue';

import { listLedger } from '#/api/tata';

import { useTataLookups } from '../use-tata-lookups';

const rows = ref<LedgerRow[]>([]);
const { loadLookups, readingLabel, userLabel } = useTataLookups();

function sourceLabel(source: string) {
  if (source === 'subscription') return '套餐赠送';
  if (source === 'pack') return '补充包';
  if (source === 'admin') return '人工补发';
  if (source === 'registration_gift') return '注册赠送';
  if (source === 'invite') return '邀请';
  if (source === 'redeem') return '兑换免费次数';
  return source;
}

function drawText(record: LedgerRow, field: 'source' | 'expires_at' | 'remaining_after') {
  const draws = record.draws || [];
  if (!draws.length) return '';
  return draws
    .map((draw) => {
      if (field === 'source') return sourceLabel(draw.source);
      if (field === 'expires_at') return draw.expires_at || '';
      return String(draw.remaining_after);
    })
    .join('；');
}

onMounted(async () => {
  await loadLookups();
  rows.value = await listLedger();
});
</script>

<template>
  <Page description="只读。会员扣次会记下来源批次、到期时间和扣完后的剩余。一条扣次跨多个批次时，用分号分开。任务状态是这条解读当时查到的状态。" title="额度流水">
    <Table
      :columns="[
        { title: '用户', dataIndex: 'user_id' },
        { title: '记录', dataIndex: 'reading_id' },
        { title: '任务状态', dataIndex: 'reading_status' },
        { title: '权益', dataIndex: 'entitlement' },
        { title: '来源', dataIndex: 'source' },
        { title: '数量', dataIndex: 'amount' },
        { title: '剩余', dataIndex: 'remaining_after' },
        { title: '到期', dataIndex: 'expires_at' },
        { title: '原因', dataIndex: 'reason' },
        { title: '结果', dataIndex: 'result' },
        { title: '时间', dataIndex: 'created_at' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
      :locale="{ emptyText: '暂无流水' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'user_id'">
          {{ userLabel(record.user_id) }}
        </template>
        <template v-else-if="column.dataIndex === 'reading_id'">
          {{ readingLabel(record.reading_id) }}
        </template>
        <template v-else-if="column.dataIndex === 'source'">
          {{ drawText(record, 'source') }}
        </template>
        <template v-else-if="column.dataIndex === 'remaining_after'">
          {{ drawText(record, 'remaining_after') }}
        </template>
        <template v-else-if="column.dataIndex === 'expires_at'">
          {{ drawText(record, 'expires_at') }}
        </template>
      </template>
    </Table>
  </Page>
</template>
