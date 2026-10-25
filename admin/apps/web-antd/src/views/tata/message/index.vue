<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table } from 'ant-design-vue';

import { listMessages } from '#/api/tata';

const rows = ref<Record<string, unknown>[]>([]);
onMounted(async () => {
  rows.value = await listMessages();
});
</script>

<template>
  <Page description="同一次追问的多个气泡共用一个回合号。不重新抽牌，不另扣额度。" title="追问消息">
    <Table
      :columns="[
        { title: '记录', dataIndex: 'reading_id' },
        { title: '回合', dataIndex: 'turn_id' },
        { title: '顺序', dataIndex: 'seq' },
        { title: '角色', dataIndex: 'role' },
        { title: '内容', dataIndex: 'content' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
    />
  </Page>
</template>
