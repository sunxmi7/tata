<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table } from 'ant-design-vue';

import { listLedger } from '#/api/tata';

const rows = ref<Record<string, unknown>[]>([]);
onMounted(async () => {
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
    />
  </Page>
</template>
