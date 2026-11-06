<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import { Button, Input, Switch, Table, Tabs, message } from 'ant-design-vue';

import { getCopy, updateCopy } from '#/api/tata';

const access = useAccessStore();
const canWrite = access.accessCodes.includes('write');
const decks = ref<Record<string, unknown>[]>([]);
const newDeck = ref({ code: '', name: '', thumbnail: '' });
const cards = ref<Record<string, unknown>[]>([]);
const thumbnailTemplate = ref('');
const thumbnailDefault = ref('');
const saving = ref(false);

async function loadDecks() {
  const data = await getCopy('decks');
  decks.value = (data.items as Record<string, unknown>[]) || [];
}

async function loadCards() {
  const data = await getCopy('cards');
  thumbnailTemplate.value = (data.thumbnail_template as string) || '';
  thumbnailDefault.value = (data.thumbnail_default as string) || '';
  cards.value = (data.items as Record<string, unknown>[]) || [];
}

async function saveDeck(row: Record<string, unknown>) {
  const data = await updateCopy('decks', {
    code: row.code,
    name: row.name,
    thumbnail: row.thumbnail ?? '',
    enabled: row.enabled,
    sort_order: row.sort_order,
  });
  decks.value = (data.items as Record<string, unknown>[]) || [];
  message.success('已保存');
}

async function addDeck() {
  if (!newDeck.value.code || !newDeck.value.name) {
    message.warning('请填写编码和名称');
    return;
  }
  const data = await updateCopy('decks', {
    code: newDeck.value.code,
    name: newDeck.value.name,
    thumbnail: newDeck.value.thumbnail,
    enabled: true,
  });
  decks.value = (data.items as Record<string, unknown>[]) || [];
  newDeck.value = { code: '', name: '', thumbnail: '' };
  message.success('已添加风格');
}

async function saveTemplate() {
  saving.value = true;
  try {
    const data = await updateCopy('cards', {
      thumbnail_template: thumbnailTemplate.value,
      thumbnail_default: thumbnailDefault.value,
    });
    thumbnailTemplate.value = (data.thumbnail_template as string) || '';
    thumbnailDefault.value = (data.thumbnail_default as string) || '';
    cards.value = (data.items as Record<string, unknown>[]) || [];
    message.success('已保存牌图设置');
  } finally {
    saving.value = false;
  }
}

async function saveCard(row: Record<string, unknown>) {
  const data = await updateCopy('cards', {
    card_name: row.card_name,
    thumbnail: row.thumbnail ?? '',
  });
  thumbnailTemplate.value = (data.thumbnail_template as string) || '';
  thumbnailDefault.value = (data.thumbnail_default as string) || '';
  cards.value = (data.items as Record<string, unknown>[]) || [];
  message.success('已保存');
}

onMounted(async () => {
  await Promise.all([loadDecks(), loadCards()]);
});
</script>

<template>
  <Page
    description="风格是整套牌（如莱德韦特）。单牌图是 78 张牌的图片 URL，没配则用默认牌背。不能新增第 79 张。"
    title="牌面"
  >
    <Tabs>
      <Tabs.TabPane key="styles" tab="风格套装">
        <div class="mb-4 flex items-center gap-2">
          <Input v-model:value="newDeck.code" placeholder="编码，如 rider-waite" />
          <Input v-model:value="newDeck.name" placeholder="展示名，如 莱德韦特" />
          <Input v-model:value="newDeck.thumbnail" placeholder="套装预览图 URL，空则默认牌背" />
          <Button v-if="canWrite" type="primary" @click="addDeck">添加风格</Button>
        </div>
        <Table
          :columns="[
            { title: '编码', dataIndex: 'code', width: 160 },
            { title: '名称', dataIndex: 'name', width: 160 },
            { title: '预览图', dataIndex: 'thumbnail' },
            { title: '启用', dataIndex: 'enabled', width: 80 },
            { title: '操作', dataIndex: 'action', width: 80 },
          ]"
          :data-source="decks"
          row-key="code"
          :pagination="false"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'name'">
              <Input v-model:value="record.name" />
            </template>
            <template v-else-if="column.dataIndex === 'thumbnail'">
              <Input v-model:value="record.thumbnail" placeholder="空则默认牌背" />
            </template>
            <template v-else-if="column.dataIndex === 'enabled'">
              <Switch v-model:checked="record.enabled" :disabled="!canWrite" />
            </template>
            <template v-else-if="column.dataIndex === 'action'">
              <Button v-if="canWrite" type="link" @click="saveDeck(record)">保存</Button>
            </template>
          </template>
        </Table>
      </Tabs.TabPane>
      <Tabs.TabPane key="cards" tab="单牌图">
        <div class="mb-4 flex flex-col gap-2">
          <div class="flex items-center gap-2">
            <Input
              v-model:value="thumbnailTemplate"
              placeholder="78 张统一模板，如 https://cdn.example.com/rw/{id}.png；可用 {id} {name}"
            />
          </div>
          <div class="flex items-center gap-2">
            <Input
              v-model:value="thumbnailDefault"
              placeholder="默认图，空则用内置 /uploads/card-default.png"
            />
            <Button v-if="canWrite" type="primary" :loading="saving" @click="saveTemplate">保存牌图设置</Button>
          </div>
        </div>
        <Table
          :columns="[
            { title: '牌', dataIndex: 'card_name', width: 140 },
            { title: '图片 URL', dataIndex: 'thumbnail' },
            { title: '生效', dataIndex: 'card_thumbnail', width: 220 },
            { title: '操作', dataIndex: 'action', width: 80 },
          ]"
          :data-source="cards"
          row-key="card_name"
          :pagination="false"
          :scroll="{ y: 480 }"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'thumbnail'">
              <Input v-model:value="record.thumbnail" placeholder="空则用模板或默认图" />
            </template>
            <template v-else-if="column.dataIndex === 'card_thumbnail'">
              <span>{{ record.card_thumbnail || '默认牌背' }}</span>
            </template>
            <template v-else-if="column.dataIndex === 'action'">
              <Button v-if="canWrite" type="link" @click="saveCard(record)">保存</Button>
            </template>
          </template>
        </Table>
      </Tabs.TabPane>
    </Tabs>
  </Page>
</template>
