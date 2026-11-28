<script lang="ts" setup>
import type { FeedbackRow } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Image, Table } from 'ant-design-vue';

import { listFeedback } from '#/api/tata';

import { useTataLookups } from '../use-tata-lookups';

const rows = ref<FeedbackRow[]>([]);
const { loadLookups, userLabel } = useTataLookups();

onMounted(async () => {
  await loadLookups();
  rows.value = await listFeedback();
});
</script>

<template>
  <Page description="用户在 App 提交的意见反馈（使用问题 / 内容建议 / 其他）。最近 200 条，只查看。与功能心愿单分开。" title="反馈意见">
    <Table
      :columns="[
        { title: '用户', dataIndex: 'user_id', width: 180 },
        { title: '分类', dataIndex: 'category', width: 110 },
        { title: '内容', dataIndex: 'content' },
        { title: '邮箱', dataIndex: 'email', width: 180 },
        { title: '附图', dataIndex: 'image_url', width: 88 },
        { title: '提交时间', dataIndex: 'created_at', width: 200 },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
      :locale="{ emptyText: '还没有反馈' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'user_id'">
          {{ userLabel(record.user_id) }}
        </template>
        <template v-else-if="column.dataIndex === 'email'">
          {{ record.email || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'image_url'">
          <Image v-if="record.image_url" :src="record.image_url" :width="48" />
          <span v-else>—</span>
        </template>
      </template>
    </Table>
  </Page>
</template>
