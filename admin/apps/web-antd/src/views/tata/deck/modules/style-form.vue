<script lang="ts" setup>
import type { UploadFile } from 'ant-design-vue';

import { computed, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { Image, Table, Upload, message } from 'ant-design-vue';

import { useVbenForm } from '#/adapter/form';
import { fileListFromURL, fileListToURL, imageUploadProps } from '#/api/core/upload';
import { listCards, updateCopy } from '#/api/tata';

const emit = defineEmits(['success']);

type FaceRow = {
  card_id: number;
  card_name: string;
  image_url?: null | string;
};

type StyleFormData = null | Record<string, unknown>;

const formData = ref<StyleFormData>(null);
const faceRows = ref<FaceRow[]>([]);
const faceUploads = ref<Record<number, UploadFile[]>>({});
const getTitle = computed(() => (formData.value?.code ? '编辑牌组' : '添加牌组'));

function applyFaceRows(faces: FaceRow[]) {
  faceRows.value = faces;
  const uploads: Record<number, UploadFile[]> = {};
  for (const row of faces) {
    uploads[row.card_id] = fileListFromURL(
      typeof row.image_url === 'string' ? row.image_url : '',
    );
  }
  faceUploads.value = uploads;
}

async function loadBlankFaces() {
  const data = await listCards();
  applyFaceRows(
    (data.items || []).map((card) => ({
      card_id: Number(card.card_id ?? card.id),
      card_name: String(card.card_name || card.name || `牌 ${card.id}`),
      image_url: '',
    })),
  );
}

const [Form, formApi] = useVbenForm({
  layout: 'vertical',
  schema: [
    {
      component: 'Input',
      fieldName: 'code',
      label: '编码',
      rules: 'required',
    },
    {
      component: 'Input',
      fieldName: 'name',
      label: '名称',
      rules: 'required',
    },
    {
      component: 'Upload',
      componentProps: imageUploadProps('decks'),
      fieldName: 'cover_url',
      label: '封面',
      renderComponentContent: () => {
        return {
          default: () => '上传封面',
        };
      },
    },
    {
      component: 'Upload',
      componentProps: imageUploadProps('decks'),
      fieldName: 'back_url',
      label: '卡背',
      renderComponentContent: () => {
        return {
          default: () => '上传卡背',
        };
      },
    },
    {
      component: 'Switch',
      defaultValue: true,
      fieldName: 'enabled',
      label: '启用',
    },
  ],
  showDefaultActions: false,
});

function setFaceUpload(cardId: number, list: UploadFile[]) {
  faceUploads.value = { ...faceUploads.value, [cardId]: list.slice(-1) };
}

function buildFacesPayload() {
  return faceRows.value.map((row) => ({
    card_id: row.card_id,
    image_url: fileListToURL(faceUploads.value[row.card_id]),
  }));
}

const [Modal, modalApi] = useVbenModal<StyleFormData>({
  class: 'w-[840px]',
  async onConfirm() {
    const { valid } = await formApi.validate();
    if (!valid) {
      return;
    }
    const data = await formApi.getValues();
    modalApi.lock();
    try {
      await updateCopy('decks', {
        code: data.code,
        name: data.name,
        cover_url: fileListToURL(data.cover_url),
        back_url: fileListToURL(data.back_url),
        enabled: data.enabled,
        sort_order: formData.value?.sort_order,
        faces: buildFacesPayload(),
      });
      message.success('已保存');
      modalApi.close();
      emit('success');
    } finally {
      modalApi.lock(false);
    }
  },
  async onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }
    const data = modalApi.getData();
    if (data?.code) {
      formData.value = data;
      applyFaceRows(
        ((data.faces as FaceRow[]) || []).map((row) => ({
          card_id: Number(row.card_id),
          card_name: String(row.card_name || `牌 ${row.card_id}`),
          image_url: row.image_url || '',
        })),
      );
      const cover =
        (typeof data.cover_url === 'string' && data.cover_url) ||
        (typeof data.thumbnail === 'string' && data.thumbnail) ||
        '';
      formApi.setValues({
        code: data.code,
        name: data.name,
        enabled: data.enabled ?? true,
        cover_url: fileListFromURL(cover),
        back_url: fileListFromURL(
          typeof data.back_url === 'string' ? data.back_url : '',
        ),
      });
      return;
    }
    formData.value = null;
    formApi.reset();
    try {
      await loadBlankFaces();
    } catch {
      faceRows.value = [];
      faceUploads.value = {};
    }
  },
});

defineExpose({ modalApi });
</script>

<template>
  <Modal :title="getTitle">
    <Form class="mx-4" />
    <div class="mx-4 mb-4">
      <div class="mb-2 text-sm font-medium">卡面（本套 78 张；文案在「牌目录」，售价在「价位」）</div>
      <Table
        :columns="[
          { title: '序号', dataIndex: 'card_id', width: 72 },
          { title: '牌名', dataIndex: 'card_name', width: 120 },
          { title: '预览', dataIndex: 'preview', width: 72 },
          { title: '图片', dataIndex: 'upload' },
        ]"
        :data-source="faceRows"
        :locale="{ emptyText: '正在加载 78 张牌…' }"
        :pagination="false"
        :scroll="{ y: 320 }"
        row-key="card_id"
        size="small"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'preview'">
            <Image
              v-if="fileListToURL(faceUploads[record.card_id])"
              :src="fileListToURL(faceUploads[record.card_id])"
              :width="40"
            />
            <span v-else>—</span>
          </template>
          <template v-else-if="column.dataIndex === 'upload'">
            <Upload
              v-bind="imageUploadProps('decks')"
              :file-list="faceUploads[record.card_id] || []"
              @update:file-list="(list) => setFaceUpload(record.card_id, list as UploadFile[])"
            >
              上传
            </Upload>
          </template>
        </template>
      </Table>
    </div>
  </Modal>
</template>
