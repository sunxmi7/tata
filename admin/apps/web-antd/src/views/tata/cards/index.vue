<script lang="ts" setup>
import type { TataCard } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page, useVbenModal } from '@vben/common-ui';
import { Plus } from '@vben/icons';
import { useAccessStore } from '@vben/stores';

import { Button, Image, Table, Tag, message } from 'ant-design-vue';

import { useVbenForm } from '#/adapter/form';
import { fileListFromURL, fileListToURL, imageUploadProps } from '#/api/core/upload';
import { listCards, updateCardSettings } from '#/api/tata';

import Form from './modules/form.vue';

const access = useAccessStore();
const canWrite = access.accessCodes.includes('write');
const rows = ref<TataCard[]>([]);

const [FormModal, formModalApi] = useVbenModal({
  connectedComponent: Form,
  destroyOnClose: true,
});

const [SettingsForm, settingsFormApi] = useVbenForm({
  handleSubmit: saveTemplate,
  schema: [
    {
      component: 'Input',
      componentProps: {
        placeholder: '78 张统一模板，如 https://cdn.example.com/rw/{id}.png；可用 {id} {name}',
      },
      fieldName: 'thumbnail_template',
      label: '图片模板',
    },
    {
      component: 'Upload',
      componentProps: imageUploadProps('cards'),
      fieldName: 'thumbnail_default',
      label: '默认图',
      renderComponentContent: () => {
        return {
          default: () => '上传图片',
        };
      },
    },
  ],
  commonConfig: {
    componentProps: {
      disabled: !canWrite,
    },
  },
  resetButtonOptions: {
    show: false,
  },
  showDefaultActions: canWrite,
  submitButtonOptions: {
    content: '保存牌图设置',
  },
});

const columns = [
  { title: 'ID', dataIndex: 'id', width: 72 },
  { title: '牌名', dataIndex: 'name', width: 120 },
  { title: '英文', dataIndex: 'name_en', width: 160 },
  { title: '图片', dataIndex: 'card_thumbnail', width: 88 },
  { title: '宜', dataIndex: 'yi' },
  { title: '启用', dataIndex: 'enabled', width: 80 },
  { title: '操作', dataIndex: 'action', width: 80 },
];

async function load() {
  const data = await listCards();
  rows.value = data.items || [];
  await settingsFormApi.setValues({
    thumbnail_template: data.thumbnail_template || '',
    thumbnail_default: fileListFromURL(data.thumbnail_default || ''),
  });
}

async function saveTemplate(values: Record<string, unknown>) {
  const data = await updateCardSettings({
    thumbnail_template: values.thumbnail_template,
    thumbnail_default: fileListToURL(values.thumbnail_default),
  });
  rows.value = data.items || [];
  await settingsFormApi.setValues({
    thumbnail_template: data.thumbnail_template || '',
    thumbnail_default: fileListFromURL(data.thumbnail_default || ''),
  });
  message.success('已保存');
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
    description="独立 cards 表。种子 78 张韦特牌（愚者 id=0）。可改多语言名、宜、图片和启停。牌名创建后不可改。抽牌仍用这 78 张。"
    title="牌面"
  >
    <template #extra>
      <Button v-if="canWrite" type="primary" @click="onCreate">
        <Plus class="size-5" />
        新建
      </Button>
    </template>
    <FormModal @success="load" />
    <SettingsForm class="mb-4" />
    <Table
      :columns="columns"
      :data-source="rows"
      row-key="id"
      :pagination="false"
      :scroll="{ y: 520 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'card_thumbnail'">
          <Image :src="record.card_thumbnail" :width="48" />
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
