<script lang="ts" setup>
import type { WishlistRow } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Image, Table } from 'ant-design-vue';

import { listWishlists } from '#/api/tata';

import { useTataLookups } from '../use-tata-lookups';

const rows = ref<WishlistRow[]>([]);
const { loadLookups, userLabel } = useTataLookups();

onMounted(async () => {
  await loadLookups();
  rows.value = await listWishlists();
});
</script>

<template>
  <Page description="用户在 App 提交的功能心愿。最近 200 条，只查看。" title="功能心愿单">
    <Table
      :columns="[
        { title: '用户', dataIndex: 'user_id', width: 200 },
        { title: '内容', dataIndex: 'content' },
        { title: '附图', dataIndex: 'image_url', width: 88 },
        { title: '提交时间', dataIndex: 'created_at', width: 200 },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
      :locale="{ emptyText: '还没有心愿' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'user_id'">
          {{ userLabel(record.user_id) }}
        </template>
        <template v-else-if="column.dataIndex === 'image_url'">
          <Image v-if="record.image_url" :src="record.image_url" :width="48" />
          <span v-else>—</span>
        </template>
      </template>
    </Table>
  </Page>
</template>
