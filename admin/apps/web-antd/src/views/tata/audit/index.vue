<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table } from 'ant-design-vue';

import { listAudit } from '#/api/tata';

const rows = ref<Record<string, unknown>[]>([]);
onMounted(async () => {
  rows.value = await listAudit();
});
</script>

<template>
  <Page description="后台写操作。不记录供应商密钥。" title="审计日志">
    <Table
      :columns="[
        { title: '时间', dataIndex: 'created_at' },
        { title: '操作员', dataIndex: 'operator_id' },
        { title: '动作', dataIndex: 'action' },
        { title: '对象', dataIndex: 'object_type' },
        { title: '对象 ID', dataIndex: 'object_id' },
        { title: '说明', dataIndex: 'detail' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
    />
  </Page>
</template>
