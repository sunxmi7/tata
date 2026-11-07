<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page, useVbenModal } from '@vben/common-ui';
import { Plus } from '@vben/icons';
import { useAccessStore } from '@vben/stores';

import { Button, Image, Switch, Table, Tabs, message } from 'ant-design-vue';

import { useVbenForm } from '#/adapter/form';
import { fileListFromURL, fileListToURL, imageUploadProps } from '#/api/core/upload';
import { getCopy, updateCopy } from '#/api/tata';

import CardForm from './modules/card-form.vue';
import StyleForm from './modules/style-form.vue';

const access = useAccessStore();
const canWrite = access.accessCodes.includes('write');
const decks = ref<Record<string, unknown>[]>([]);
const cards = ref<Record<string, unknown>[]>([]);

const [StyleModal, styleModalApi] = useVbenModal({
  connectedComponent: StyleForm,
  destroyOnClose: true,
});
const [CardModal, cardModalApi] = useVbenModal({
  connectedComponent: CardForm,
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

async function loadDecks() {
  const data = await getCopy('decks');
  decks.value = (data.items as Record<string, unknown>[]) || [];
}

async function loadCards() {
  const data = await getCopy('cards');
  cards.value = (data.items as Record<string, unknown>[]) || [];
  await settingsFormApi.setValues({
    thumbnail_template: (data.thumbnail_template as string) || '',
    thumbnail_default: fileListFromURL(
      typeof data.thumbnail_default === 'string' ? data.thumbnail_default : '',
    ),
  });
}

async function saveTemplate(values: Record<string, unknown>) {
  const data = await updateCopy('cards', {
    thumbnail_template: values.thumbnail_template,
    thumbnail_default: fileListToURL(values.thumbnail_default),
  });
  cards.value = (data.items as Record<string, unknown>[]) || [];
  await settingsFormApi.setValues({
    thumbnail_template: (data.thumbnail_template as string) || '',
    thumbnail_default: fileListFromURL(
      typeof data.thumbnail_default === 'string' ? data.thumbnail_default : '',
    ),
  });
  message.success('已保存');
}

function onCreateStyle() {
  styleModalApi.setData(null).open();
}

function onEditStyle(row: Record<string, unknown>) {
  styleModalApi.setData(row).open();
}

function onEditCard(row: Record<string, unknown>) {
  cardModalApi.setData(row).open();
}

onMounted(async () => {
  await Promise.all([loadDecks(), loadCards()]);
});
</script>

<template>
  <Page
    description="风格是整套牌（如莱德韦特）。单牌图是 78 张牌的图片，没配则用默认牌背。不能新增第 79 张。"
    title="牌面"
  >
    <StyleModal @success="loadDecks" />
    <CardModal @success="loadCards" />
    <Tabs>
      <Tabs.TabPane key="styles" tab="风格套装">
        <div class="mb-4">
          <Button v-if="canWrite" type="primary" @click="onCreateStyle">
            <Plus class="size-5" />
            添加风格
          </Button>
        </div>
        <Table
          :columns="[
            { title: '编码', dataIndex: 'code', width: 160 },
            { title: '名称', dataIndex: 'name', width: 160 },
            { title: '预览图', dataIndex: 'preview', width: 88 },
            { title: '启用', dataIndex: 'enabled', width: 80 },
            { title: '操作', dataIndex: 'action', width: 80 },
          ]"
          :data-source="decks"
          row-key="code"
          :pagination="false"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'preview'">
              <Image :src="record.preview" :width="48" />
            </template>
            <template v-else-if="column.dataIndex === 'enabled'">
              <Switch :checked="record.enabled" disabled />
            </template>
            <template v-else-if="column.dataIndex === 'action'">
              <Button v-if="canWrite" type="link" @click="onEditStyle(record)">编辑</Button>
            </template>
          </template>
        </Table>
      </Tabs.TabPane>
      <Tabs.TabPane key="cards" tab="单牌图">
        <SettingsForm class="mb-4" />
        <Table
          :columns="[
            { title: '牌', dataIndex: 'card_name', width: 140 },
            { title: '图片', dataIndex: 'card_thumbnail', width: 88 },
            { title: '操作', dataIndex: 'action', width: 80 },
          ]"
          :data-source="cards"
          row-key="card_name"
          :pagination="false"
          :scroll="{ y: 480 }"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'card_thumbnail'">
              <Image :src="record.card_thumbnail" :width="48" />
            </template>
            <template v-else-if="column.dataIndex === 'action'">
              <Button v-if="canWrite" type="link" @click="onEditCard(record)">编辑</Button>
            </template>
          </template>
        </Table>
      </Tabs.TabPane>
    </Tabs>
  </Page>
</template>
