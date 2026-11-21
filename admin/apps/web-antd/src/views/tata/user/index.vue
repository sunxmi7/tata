<script lang="ts" setup>
import type { AppUser } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Image, Table, Tag } from 'ant-design-vue';

import { listUsers } from '#/api/tata';

const rows = ref<AppUser[]>([]);
onMounted(async () => {
  rows.value = await listUsers();
});

function prefs(row: AppUser) {
  return row.explore_preferences ?? [];
}
</script>

<template>
  <Page description="头像、昵称、性别、探索偏好与注销状态。没有手机号。注册奖励不在这里扣。画像占比见「用户统计」。" title="用户账号">
    <Table
      :columns="[
        { title: 'ID', dataIndex: 'id', width: 72 },
        { title: '头像', dataIndex: 'avatar_url', width: 72 },
        { title: '昵称', dataIndex: 'nickname', width: 120 },
        { title: '性别', dataIndex: 'gender', width: 96 },
        { title: '生日', dataIndex: 'birth_date', width: 120 },
        { title: '探索偏好', dataIndex: 'explore_preferences' },
        { title: '邮箱', dataIndex: 'email' },
        { title: '微信', dataIndex: 'wechat_openid', ellipsis: true },
        { title: '苹果', dataIndex: 'apple_sub', ellipsis: true },
        { title: '时区', dataIndex: 'timezone', width: 140 },
        { title: '状态', dataIndex: 'cancelled_at', width: 180 },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
      :scroll="{ x: 1480 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'avatar_url'">
          <Image v-if="record.avatar_url" :src="record.avatar_url" :width="40" :height="40" />
          <span v-else>—</span>
        </template>
        <template v-else-if="column.dataIndex === 'nickname'">
          {{ record.nickname || record.name || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'gender'">
          {{ record.gender || '未填写' }}
        </template>
        <template v-else-if="column.dataIndex === 'birth_date'">
          {{ record.birth_date || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'explore_preferences'">
          <template v-if="prefs(record).length">
            <Tag v-for="item in prefs(record)" :key="item" class="mb-1 mr-1">{{ item }}</Tag>
          </template>
          <span v-else>未选择</span>
        </template>
        <template v-else-if="column.dataIndex === 'cancelled_at'">
          <Tag v-if="record.cancelled_at" color="red">已注销 {{ record.cancelled_at }}</Tag>
          <Tag v-else color="green">正常</Tag>
        </template>
      </template>
    </Table>
  </Page>
</template>
