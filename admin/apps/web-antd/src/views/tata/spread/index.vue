<script lang="ts" setup>
import type { Spread } from '#/api/tata';

import { computed, onMounted, ref } from 'vue';

import { Page, useVbenModal } from '@vben/common-ui';
import { Plus } from '@vben/icons';

import { Button, Table, Tabs, Tag } from 'ant-design-vue';

import { listSpreads } from '#/api/tata';

import { hasTataWrite } from '../access';
import Form from './modules/form.vue';

const rows = ref<Spread[]>([]);
const activeTab = ref<'explore' | 'ordinary'>('ordinary');

const [FormModal, formModalApi] = useVbenModal({
  connectedComponent: Form,
  destroyOnClose: true,
});

const ordinaryColumns = [
  { title: '名称', dataIndex: 'name', width: 180 },
  { title: '英文名', dataIndex: 'name_en', width: 220 },
  { title: '含义说明', dataIndex: 'description' },
  { title: '牌数', dataIndex: 'card_count', width: 72 },
  { title: '分组', dataIndex: 'depth_label', width: 110 },
  { title: '额度消耗', dataIndex: 'quota_cost', width: 100 },
  { title: '启用', dataIndex: 'enabled', width: 80 },
  { title: '操作', dataIndex: 'action', width: 80 },
];

const exploreColumns = [
  { title: '名称', dataIndex: 'name', width: 180 },
  { title: '英文名', dataIndex: 'name_en', width: 220 },
  { title: '含义说明', dataIndex: 'description' },
  { title: '主题键', dataIndex: 'theme_key', width: 120 },
  { title: '主题', dataIndex: 'theme_title', width: 120 },
  { title: '快捷问题', dataIndex: 'quick_questions', width: 88 },
  { title: '牌数', dataIndex: 'card_count', width: 72 },
  { title: '额度消耗', dataIndex: 'quota_cost', width: 100 },
  { title: '启用', dataIndex: 'enabled', width: 80 },
  { title: '操作', dataIndex: 'action', width: 80 },
];

const ordinaryRows = computed(() =>
  rows.value.filter((item) => !String(item.theme_key || '').trim()),
);
const exploreRows = computed(() =>
  rows.value.filter((item) => !!String(item.theme_key || '').trim()),
);

const columns = computed(() =>
  activeTab.value === 'ordinary' ? ordinaryColumns : exploreColumns,
);
const tableRows = computed(() =>
  activeTab.value === 'ordinary' ? ordinaryRows.value : exploreRows.value,
);

function canWrite() {
  return hasTataWrite('content');
}

async function load() {
  rows.value = await listSpreads();
}

function onCreate() {
  formModalApi
    .setData({
      mode: activeTab.value,
    })
    .open();
}

function onEdit(row: Spread) {
  formModalApi
    .setData({
      mode: String(row.theme_key || '').trim() ? 'explore' : 'ordinary',
      spread: row,
    })
    .open();
}

onMounted(load);
</script>

<template>
  <Page
    description="普通占卜与探索分开展示。可改英文名、含义说明与额度消耗；探索可改主题键、主题名、副标题、一句说明和快捷问题。"
    title="牌阵"
  >
    <template #extra>
      <Button v-if="canWrite()" type="primary" @click="onCreate">
        <Plus class="size-5" />
        新建
      </Button>
    </template>
    <FormModal @success="load" />
    <Tabs v-model:active-key="activeTab">
      <Tabs.TabPane key="ordinary" tab="普通占卜" />
      <Tabs.TabPane key="explore" tab="探索" />
    </Tabs>
    <Table
      :columns="columns"
      :data-source="tableRows"
      row-key="id"
      :pagination="false"
      :locale="{
        emptyText: activeTab === 'ordinary' ? '还没有普通占卜牌阵' : '还没有探索牌阵',
      }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'name'">
          {{ record.name }}
        </template>
        <template v-else-if="column.dataIndex === 'name_en'">
          {{ record.name_en || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'description'">
          {{ record.description || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'card_count'">
          {{ record.card_count }}
        </template>
        <template v-else-if="column.dataIndex === 'enabled'">
          <Tag :color="record.enabled ? 'green' : 'default'">
            {{ record.enabled ? '启用' : '停用' }}
          </Tag>
        </template>
        <template v-else-if="column.dataIndex === 'theme_key'">
          {{ record.theme_key || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'theme_title'">
          {{ record.theme_title || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'quick_questions'">
          {{
            Array.isArray(record.quick_questions) && record.quick_questions.length
              ? `${record.quick_questions.length} 条`
              : '—'
          }}
        </template>
        <template v-else-if="column.dataIndex === 'depth_label'">
          {{ record.depth_label || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'quota_cost'">
          {{ record.quota_priced ? record.quota_cost : '未标价' }}
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button v-if="canWrite()" type="link" @click="onEdit(record as Spread)">
            编辑
          </Button>
        </template>
      </template>
    </Table>
  </Page>
</template>
