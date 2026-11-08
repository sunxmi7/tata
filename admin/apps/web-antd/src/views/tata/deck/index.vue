<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page, useVbenModal } from '@vben/common-ui';
import { Plus } from '@vben/icons';
import { useAccessStore } from '@vben/stores';

import { Button, Image, Switch, Table } from 'ant-design-vue';

import { getCopy } from '#/api/tata';

import StyleForm from './modules/style-form.vue';

const access = useAccessStore();
const canWrite = access.accessCodes.includes('write');
const decks = ref<Record<string, unknown>[]>([]);

const [StyleModal, styleModalApi] = useVbenModal({
  connectedComponent: StyleForm,
  destroyOnClose: true,
});

async function loadDecks() {
  const data = await getCopy('decks');
  decks.value = (data.items as Record<string, unknown>[]) || [];
}

function onCreateStyle() {
  styleModalApi.setData(null).open();
}

function onEditStyle(row: Record<string, unknown>) {
  styleModalApi.setData(row).open();
}

onMounted(loadDecks);
</script>

<template>
  <Page description="整套牌的风格（如莱德韦特）。单牌目录在侧栏「牌面」。" title="牌组">
    <template #extra>
      <Button v-if="canWrite" type="primary" @click="onCreateStyle">
        <Plus class="size-5" />
        添加风格
      </Button>
    </template>
    <StyleModal @success="loadDecks" />
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
  </Page>
</template>
