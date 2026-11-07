<script lang="ts" setup>
import type { Spread } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page, useVbenModal } from '@vben/common-ui';
import { Plus } from '@vben/icons';
import { useAccessStore } from '@vben/stores';

import { Button, Image, Table, Tag } from 'ant-design-vue';

import { listSpreads } from '#/api/tata';

import Form from './modules/form.vue';

const access = useAccessStore();
const rows = ref<Spread[]>([]);

const [FormModal, formModalApi] = useVbenModal({
  connectedComponent: Form,
  destroyOnClose: true,
});

const columns = [
  { title: '名称', dataIndex: 'name' },
  { title: '牌数', dataIndex: 'card_count' },
  { title: '示意图', dataIndex: 'preview', width: 88 },
  { title: '启用', dataIndex: 'enabled' },
  { title: '推荐', dataIndex: 'recommended' },
  { title: '收费', dataIndex: 'charged' },
  { title: '操作', dataIndex: 'action' },
];

function canWrite() {
  return access.accessCodes.includes('write');
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
  <Page description="牌阵配置。没有价格字段。" title="牌阵">
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
          <Image :src="record.preview" :width="48" />
        </template>
        <template v-else-if="column.dataIndex === 'enabled'">
          <Tag :color="record.enabled ? 'green' : 'default'">
            {{ record.enabled ? '启用' : '停用' }}
          </Tag>
        </template>
        <template v-else-if="column.dataIndex === 'recommended'">
          {{ record.recommended ? '是' : '否' }}
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
