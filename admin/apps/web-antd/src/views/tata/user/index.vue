<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table } from 'ant-design-vue';

import { listUsers } from '#/api/tata';

const rows = ref<Record<string, unknown>[]>([]);
onMounted(async () => {
  rows.value = await listUsers();
});
</script>

<template>
  <Page description="邮箱、微信、手机号和注销状态。注册奖励不在这里扣。" title="用户账号">
    <Table
      :columns="[
        { title: 'ID', dataIndex: 'id' },
        { title: '邮箱', dataIndex: 'email' },
        { title: '微信', dataIndex: 'wechat_openid' },
        { title: '苹果', dataIndex: 'apple_sub' },
        { title: '手机号', dataIndex: 'phone' },
        { title: '时区', dataIndex: 'timezone' },
        { title: '塔罗师试用', dataIndex: 'tarotist_trial_used' },
        { title: '注销时间', dataIndex: 'cancelled_at' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
    />
  </Page>
</template>
