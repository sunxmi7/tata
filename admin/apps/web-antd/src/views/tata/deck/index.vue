<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page, useVbenModal } from '@vben/common-ui';
import { Plus } from '@vben/icons';

import { Button, Image, Switch, Table, Tag } from 'ant-design-vue';

import { getCopy } from '#/api/tata';

import { hasTataWrite } from '../access';
import StyleForm from './modules/style-form.vue';

type FaceRow = {
  card_id: number;
  card_name: string;
  image_url?: null | string;
};

type DeckRow = {
  code: string;
  name: string;
  cover_url?: null | string;
  thumbnail?: null | string;
  back_url?: null | string;
  face_count?: number;
  card_count?: number;
  face_previews?: string[];
  faces?: FaceRow[];
  enabled?: boolean;
  description?: string;
};

const canWrite = hasTataWrite('content');
const decks = ref<DeckRow[]>([]);
const expanded = ref<string[]>([]);

const [StyleModal, styleModalApi] = useVbenModal({
  connectedComponent: StyleForm,
  destroyOnClose: true,
});

async function loadDecks() {
  const data = await getCopy('decks');
  decks.value = ((data.items as DeckRow[]) || []).map((row) => ({
    ...row,
    faces: row.faces || [],
    face_previews: row.face_previews || [],
  }));
}

function onCreateStyle() {
  styleModalApi.setData(null).open();
}

function onEditStyle(row: DeckRow) {
  styleModalApi.setData(row).open();
}

onMounted(loadDecks);
</script>

<template>
  <Page
    description="这里只管理牌组素材：78 张卡面。单牌文案在「牌目录」。售价在「付费 → 价位」。展开一行可看本套卡面。"
    title="牌组"
  >
    <template #extra>
      <Button v-if="canWrite" type="primary" @click="onCreateStyle">
        <Plus class="size-5" />
        添加牌组
      </Button>
    </template>
    <StyleModal @success="loadDecks" />
    <Table
      v-model:expanded-row-keys="expanded"
      :columns="[
        { title: '编码', dataIndex: 'code', width: 140 },
        { title: '名称', dataIndex: 'name', width: 140 },
        { title: '卡面进度', dataIndex: 'faces', width: 110 },
        { title: '卡面预览', dataIndex: 'face_previews', width: 220 },
        { title: '启用', dataIndex: 'enabled', width: 80 },
        { title: '操作', dataIndex: 'action', width: 80 },
      ]"
      :data-source="decks"
      :expandable="{}"
      row-key="code"
      :pagination="false"
      :locale="{ emptyText: '还没有牌组' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'faces'">
          <Tag :color="Number(record.face_count || 0) >= 78 ? 'green' : 'orange'">
            {{ Number(record.face_count || 0) }}/{{ Number(record.card_count || 78) }}
          </Tag>
        </template>
        <template v-else-if="column.dataIndex === 'face_previews'">
          <div class="flex flex-wrap gap-1">
            <Image
              v-for="(src, idx) in (record.face_previews || []).slice(0, 6)"
              :key="`${record.code}-${idx}`"
              :src="src"
              :width="36"
            />
            <span v-if="!(record.face_previews || []).length">—</span>
          </div>
        </template>
        <template v-else-if="column.dataIndex === 'enabled'">
          <Switch :checked="!!record.enabled" disabled />
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button v-if="canWrite" type="link" @click="onEditStyle(record as DeckRow)">编辑</Button>
        </template>
      </template>
      <template #expandedRowRender="{ record }">
        <div class="px-2 py-3">
          <div class="mb-2 text-sm text-gray-500">
            {{ record.name }}（{{ record.code }}）的 78 张卡面。售价请去「价位」页改。
          </div>
          <div class="grid grid-cols-6 gap-3 md:grid-cols-8 lg:grid-cols-10">
            <div
              v-for="face in record.faces || []"
              :key="face.card_id"
              class="flex flex-col items-center gap-1"
            >
              <Image v-if="face.image_url" :src="face.image_url" :width="56" />
              <div
                v-else
                class="flex h-14 w-14 items-center justify-center bg-gray-100 text-xs text-gray-400"
              >
                未配
              </div>
              <div class="text-center text-xs leading-tight">
                {{ String(face.card_id).padStart(2, '0') }}
                <div>{{ face.card_name }}</div>
              </div>
            </div>
          </div>
        </div>
      </template>
    </Table>
  </Page>
</template>
