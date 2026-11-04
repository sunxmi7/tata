<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table } from 'ant-design-vue';

import { listCalendar } from '#/api/tata';

import { useTataLookups } from '../use-tata-lookups';

const rows = ref<Record<string, unknown>[]>([]);
const { loadLookups, userLabel } = useTataLookups();

onMounted(async () => {
  await loadLookups();
  rows.value = await listCalendar();
});
</script>

<template>
  <Page description="已删除的占卜不会出现在这里。" title="日历">
    <Table
      :columns="[
        { title: '日期', dataIndex: 'business_date' },
        { title: '用户', dataIndex: 'user_id' },
        { title: '类型', dataIndex: 'type' },
        { title: '问题', dataIndex: 'question' },
        { title: '进度', dataIndex: 'progress' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
      :locale="{ emptyText: '暂无日历记录' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'user_id'">
          {{ userLabel(record.user_id) }}
        </template>
      </template>
    </Table>
  </Page>
</template>
