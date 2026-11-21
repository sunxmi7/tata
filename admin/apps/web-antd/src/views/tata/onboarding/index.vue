<script lang="ts" setup>
import type { DeviceGuide } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table } from 'ant-design-vue';

import { listOnboarding } from '#/api/tata';

const rows = ref<DeviceGuide[]>([]);

onMounted(async () => {
  const data = await listOnboarding();
  rows.value = data.items || [];
});
</script>

<template>
  <Page
    description="设备只决定引导出不出现。解读资格记在账号上，每账号一次。这里只查看。"
    title="新人引导"
  >
    <Table
      :columns="[
        { title: '设备', dataIndex: 'device_id' },
        { title: '引导已完成', dataIndex: 'guide_completed', width: 120 },
        { title: '关联用户', dataIndex: 'user_id', width: 110 },
        { title: '账号已领解读', dataIndex: 'reading_trial_used', width: 130 },
        { title: '记录时间', dataIndex: 'created_at' },
      ]"
      :data-source="rows"
      row-key="device_id"
      :pagination="{ pageSize: 20 }"
      :locale="{ emptyText: '还没有引导记录' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'guide_completed'">
          {{ record.guide_completed ? '是' : '否' }}
        </template>
        <template v-else-if="column.dataIndex === 'reading_trial_used'">
          {{ record.reading_trial_used ? '是' : '否' }}
        </template>
      </template>
    </Table>
  </Page>
</template>
