<script lang="ts" setup>
import type { AppUser } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table } from 'ant-design-vue';

import { listUsers } from '#/api/tata';

const rows = ref<AppUser[]>([]);
onMounted(async () => {
  rows.value = await listUsers();
});
</script>

<template>
  <Page description="邮箱、微信、苹果和注销状态。没有手机号。注册奖励不在这里扣。" title="用户账号">
    <Table
      :columns="[
        { title: 'ID', dataIndex: 'id', width: 80 },
        { title: '邮箱', dataIndex: 'email' },
        { title: '微信', dataIndex: 'wechat_openid' },
        { title: '苹果', dataIndex: 'apple_sub' },
        { title: '时区', dataIndex: 'timezone', width: 160 },
        { title: '塔罗师试用', dataIndex: 'tarotist_trial_used', width: 110 },
        { title: '创建时间', dataIndex: 'created_at', width: 180 },
        { title: '注销时间', dataIndex: 'cancelled_at', width: 180 },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
    />
  </Page>
</template>
