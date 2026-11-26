<script lang="ts" setup>
import type { TataCard } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page, useVbenModal } from '@vben/common-ui';
import { Plus } from '@vben/icons';

import { Button, Image, Table, Tag } from 'ant-design-vue';

import { listCards } from '#/api/tata';

import { hasTataWrite } from '../access';
import Form from './modules/form.vue';

const canWrite = hasTataWrite('content');
const rows = ref<TataCard[]>([]);

const [FormModal, formModalApi] = useVbenModal({
  connectedComponent: Form,
  destroyOnClose: true,
});

const columns = [
  { title: 'ID', dataIndex: 'id', width: 72 },
  { title: '牌名', dataIndex: 'name', width: 120 },
  { title: '英文', dataIndex: 'name_en', width: 160 },
  { title: '预览', dataIndex: 'card_thumbnail', width: 88 },
  { title: '宜', dataIndex: 'yi' },
  { title: '启用', dataIndex: 'enabled', width: 80 },
  { title: '操作', dataIndex: 'action', width: 80 },
];

async function load() {
  const data = await listCards();
  rows.value = data.items || [];
}

function onCreate() {
  formModalApi.setData(null).open();
}

function onEdit(row: TataCard) {
  formModalApi.setData(row).open();
}

onMounted(load);
</script>

<template>
  <Page
    description="管理每张牌的文案：多语言名、正逆位寄语、今日宜、启停。各风格卡面在「牌组」。预览默认来自莱德韦特。"
    title="牌目录"
  >
    <template #extra>
      <Button v-if="canWrite" type="primary" @click="onCreate">
        <Plus class="size-5" />
        新建
      </Button>
    </template>
    <FormModal @success="load" />
    <Table
      :columns="columns"
      :data-source="rows"
      row-key="id"
      :pagination="false"
      :scroll="{ y: 520 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'card_thumbnail'">
          <Image v-if="record.card_thumbnail" :src="record.card_thumbnail" :width="48" />
          <span v-else>—</span>
        </template>
        <template v-else-if="column.dataIndex === 'enabled'">
          <Tag :color="record.enabled ? 'green' : 'default'">
            {{ record.enabled ? '启用' : '停用' }}
          </Tag>
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button v-if="canWrite" type="link" @click="onEdit(record as TataCard)">
            编辑
          </Button>
        </template>
      </template>
    </Table>
  </Page>
</template>
