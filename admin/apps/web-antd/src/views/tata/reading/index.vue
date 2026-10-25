<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import { Button, Table, message } from 'ant-design-vue';

import { listReadings, retryReading } from '#/api/tata';

const access = useAccessStore();
const rows = ref<Record<string, unknown>[]>([]);
async function load() {
  rows.value = await listReadings();
}
async function retry(id: number) {
  await retryReading(id);
  message.success('已重试');
  await load();
}
onMounted(load);
</script>

<template>
  <Page description="只能重试失败记录。不改牌，不新扣费。" title="占卜记录">
    <Table
      :columns="[
        { title: 'ID', dataIndex: 'id' },
        { title: '类型', dataIndex: 'type' },
        { title: '状态', dataIndex: 'status' },
        { title: '问题', dataIndex: 'question' },
        { title: '供应商', dataIndex: 'vendor_name' },
        { title: '模型', dataIndex: 'model_name' },
        { title: '失败原因', dataIndex: 'fail_reason' },
        { title: '操作', dataIndex: 'action' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
    >
      <template #bodyCell="{ column, record }">
        <Button
          v-if="column.dataIndex === 'action' && access.accessCodes.includes('write') && record.status === 'FAILED_RETRYABLE'"
          type="link"
          @click="retry(record.id)"
        >
          重试
        </Button>
      </template>
    </Table>
  </Page>
</template>
