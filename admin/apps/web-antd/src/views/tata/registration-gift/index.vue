<script lang="ts" setup>
import type { RegistrationGift } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table } from 'ant-design-vue';

import { listRegistrationGifts } from '#/api/tata';

const rows = ref<RegistrationGift[]>([]);

onMounted(async () => {
  const data = await listRegistrationGifts();
  rows.value = data.items || [];
});
</script>

<template>
  <Page
    description="间隔留空时只记注册时间，不自动发放。发放后从实际发放时刻起算有效期。这里只查看。"
    title="注册赠送"
  >
    <Table
      :columns="[
        { title: '用户', dataIndex: 'user_id', width: 100 },
        { title: '注册时间', dataIndex: 'registered_at' },
        { title: '发放时间', dataIndex: 'granted_at' },
        { title: '可使用时间', dataIndex: 'usable_at' },
        { title: '失效时间', dataIndex: 'expires_at' },
        { title: '总数', dataIndex: 'total', width: 80 },
        { title: '剩余', dataIndex: 'remaining', width: 80 },
        { title: '使用时间', dataIndex: 'used_at' },
      ]"
      :data-source="rows"
      row-key="user_id"
      :pagination="{ pageSize: 20 }"
      :locale="{ emptyText: '还没有注册赠送记录' }"
    />
  </Page>
</template>
