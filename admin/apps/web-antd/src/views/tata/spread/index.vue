<script lang="ts" setup>
import type { Spread } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page, useVbenModal } from '@vben/common-ui';
import { Plus } from '@vben/icons';

import { Button, Image, Table, Tag } from 'ant-design-vue';

import { listSpreads } from '#/api/tata';

import { hasTataWrite } from '../access';
import Form from './modules/form.vue';

const rows = ref<Spread[]>([]);

const [FormModal, formModalApi] = useVbenModal({
  connectedComponent: Form,
  destroyOnClose: true,
});

const columns = [
  { title: '名称', dataIndex: 'name', width: 180 },
  { title: '英文名', dataIndex: 'name_en', width: 220 },
  { title: '含义说明', dataIndex: 'description' },
  { title: '主题', dataIndex: 'theme_title', width: 120 },
  { title: '牌数', dataIndex: 'card_count', width: 72 },
  { title: '分组', dataIndex: 'depth_label', width: 110 },
  { title: '额度扣次', dataIndex: 'quota_cost', width: 100 },
  { title: '示意图', dataIndex: 'preview', width: 88 },
  { title: '启用', dataIndex: 'enabled', width: 80 },
  { title: '推荐', dataIndex: 'recommended', width: 72 },
  { title: '收费', dataIndex: 'charged', width: 72 },
  { title: '操作', dataIndex: 'action', width: 80 },
];

function canWrite() {
  return hasTataWrite('content');
}

async function load() {
  rows.value = await listSpreads();
}

function onCreate() {
  formModalApi.setData(null).open();
}

function onEdit(row: Spread) {
  formModalApi.setData(row).open();
}

onMounted(load);
</script>

<template>
  <Page
    description="普通占卜按快速指引、进阶分析、深度全景分组，可改英文名和含义说明。示意图没上传时显示未上传，不用牌背代替。探索牌阵按主题挂在同一张表。扣次是额度。"
    title="牌阵"
  >
    <template #extra>
      <Button v-if="canWrite()" type="primary" @click="onCreate">
        <Plus class="size-5" />
        新建
      </Button>
    </template>
    <FormModal @success="load" />
    <Table :columns="columns" :data-source="rows" row-key="id" :pagination="false">
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'preview'">
          <Image v-if="record.thumbnail" :src="record.preview" :width="48" />
          <span v-else>未上传</span>
        </template>
        <template v-else-if="column.dataIndex === 'name'">
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
        <template v-else-if="column.dataIndex === 'recommended'">
          {{ record.recommended ? '是' : '否' }}
        </template>
        <template v-else-if="column.dataIndex === 'theme_title'">
          {{ record.theme_title || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'depth_label'">
          {{ record.depth_label || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'quota_cost'">
          {{ record.quota_priced ? record.quota_cost : '未标价' }}
        </template>
        <template v-else-if="column.dataIndex === 'charged'">
          {{ record.charged ? '是' : '否' }}
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button v-if="canWrite()" type="link" @click="onEdit(record as Spread)">编辑</Button>
        </template>
      </template>
    </Table>
  </Page>
</template>
