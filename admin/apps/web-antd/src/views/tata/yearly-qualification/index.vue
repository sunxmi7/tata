<script lang="ts" setup>
import type { YearlyQualification } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table } from 'ant-design-vue';

import { listYearlyQualifications } from '#/api/tata';

const rows = ref<YearlyQualification[]>([]);

function sourceLabel(source: string) {
  if (source === 'purchase') return '单买';
  if (source === 'year_subscription') return '年卡';
  if (source === 'membership') return '会员';
  return source || '未记录';
}

onMounted(async () => {
  const data = await listYearlyQualifications();
  rows.value = data.items || [];
});
</script>

<template>
  <Page
    description="按账号和年份查看年运是否已解锁。会员走额度，非会员单买后记为单买。这里只查看。"
    title="年运资格"
  >
    <Table
      :columns="[
        { title: '用户', dataIndex: 'user_id', width: 100 },
        { title: '年份', dataIndex: 'year', width: 90 },
        { title: '已解锁', dataIndex: 'unlocked', width: 90 },
        { title: '来源', dataIndex: 'unlock_source', width: 110 },
        { title: '解锁时间', dataIndex: 'unlocked_at' },
        { title: '报告状态', dataIndex: 'report_status', width: 110 },
        { title: '生成时间', dataIndex: 'generated_at' },
      ]"
      :data-source="rows"
      :row-key="(row: YearlyQualification) => `${row.user_id}-${row.year}`"
      :pagination="{ pageSize: 20 }"
      :locale="{ emptyText: '还没有年运资格' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'unlocked'">
          {{ record.unlocked ? '是' : '否' }}
        </template>
        <template v-else-if="column.dataIndex === 'unlock_source'">
          {{ sourceLabel(record.unlock_source) }}
        </template>
      </template>
    </Table>
  </Page>
</template>
