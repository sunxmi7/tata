<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table } from 'ant-design-vue';

import { listLedger } from '#/api/tata';

import { useTataLookups } from '../use-tata-lookups';

const rows = ref<Record<string, unknown>[]>([]);
const { loadLookups, readingLabel, userLabel } = useTataLookups();

onMounted(async () => {
  await loadLookups();
  rows.value = await listLedger();
});
</script>

<template>
  <Page description="次数流水只读，不能手改。人工补发钻石或免费次数走「人工补发」。" title="额度流水">
    <Table
      :columns="[
        { title: '用户', dataIndex: 'user_id' },
        { title: '记录', dataIndex: 'reading_id' },
        { title: '权益', dataIndex: 'entitlement' },
        { title: '数量', dataIndex: 'amount' },
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
      </template>
    </Table>
  </Page>
</template>
