<script lang="ts" setup>
import type { QuotaLot } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table } from 'ant-design-vue';

import { listQuotaLots } from '#/api/tata';

const rows = ref<QuotaLot[]>([]);

function sourceLabel(source: string) {
  if (source === 'subscription') return '套餐赠送';
  if (source === 'pack') return '补充包';
  if (source === 'admin') return '人工补发';
  return source;
}

onMounted(async () => {
  const data = await listQuotaLots();
  rows.value = data.items || [];
});
</script>

<template>
  <Page
    description="会员额度按批次记账。套餐赠送和补充包分开。到期后剩余作废，不冻结。"
    title="额度批次"
  >
    <Table
      :columns="[
        { title: '用户', dataIndex: 'user_id', width: 100 },
        { title: '来源', dataIndex: 'source', width: 110 },
        { title: '总数', dataIndex: 'total', width: 80 },
        { title: '剩余', dataIndex: 'remaining', width: 80 },
        { title: '发放时间', dataIndex: 'granted_at' },
        { title: '到期时间', dataIndex: 'expires_at' },
        { title: '已到期', dataIndex: 'expired', width: 90 },
        { title: '原因', dataIndex: 'reason' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
      :locale="{ emptyText: '还没有额度批次' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'source'">
          {{ sourceLabel(record.source) }}
        </template>
        <template v-else-if="column.dataIndex === 'expired'">
          {{ record.expired ? '是' : '否' }}
        </template>
      </template>
    </Table>
  </Page>
</template>
