<script lang="ts" setup>
import type { TataCard } from '#/api/tata';

import { computed, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { message } from 'ant-design-vue';

import { useVbenForm } from '#/adapter/form';
import { fileListFromURL, fileListToURL, imageUploadProps } from '#/api/core/upload';
import { createCard, updateCard } from '#/api/tata';

const emit = defineEmits(['success']);
const formData = ref<null | TataCard>(null);
const nameLocked = ref(false);
const getTitle = computed(() => (formData.value ? '编辑单牌' : '新建单牌'));

const [Form, formApi] = useVbenForm({
  layout: 'vertical',
  schema: [
    {
      component: 'Input',
      componentProps: () => ({
        disabled: nameLocked.value,
      }),
      fieldName: 'name',
      label: '牌名（简体）',
    },
    {
      component: 'Input',
      fieldName: 'name_zh_hant',
      label: '繁体',
    },
    {
      component: 'Input',
      fieldName: 'name_en',
      label: '英文',
    },
    {
      component: 'Input',
      fieldName: 'name_ja',
      label: '日文',
    },
    {
      component: 'Input',
      fieldName: 'name_ko',
      label: '韩文',
    },
    {
      component: 'Input',
      fieldName: 'yi',
      label: '今日一牌宜',
    },
    {
      component: 'Upload',
      componentProps: imageUploadProps('cards'),
      fieldName: 'thumbnail',
      label: '单牌图',
      renderComponentContent: () => {
        return {
          default: () => '上传图片',
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

const [Modal, modalApi] = useVbenModal<null | TataCard>({
  async onConfirm() {
    const { valid } = await formApi.validate();
    if (!valid) {
      return;
    }
    const data = await formApi.getValues();
    const payload = {
      name: data.name,
      name_zh_hant: data.name_zh_hant,
      name_en: data.name_en,
      name_ja: data.name_ja,
      name_ko: data.name_ko,
      yi: data.yi,
      thumbnail: fileListToURL(data.thumbnail),
      enabled: data.enabled,
    };
    modalApi.lock();
    try {
      if (formData.value) {
        await updateCard(formData.value.id, payload);
      } else {
        await createCard(payload);
      }
      message.success('已保存');
      modalApi.close();
      emit('success');
    } finally {
      modalApi.lock(false);
    }
  },
  onOpenChange(isOpen) {
    if (!isOpen) {
      formData.value = null;
      return;
    }
    const data = modalApi.getData();
    formData.value = data ?? null;
    nameLocked.value = !!data;
    if (!data) {
      formApi.reset();
      return;
    }
    formApi.setValues({
      name: data.name,
      name_zh_hant: data.name_zh_hant,
      name_en: data.name_en,
      name_ja: data.name_ja,
      name_ko: data.name_ko,
      yi: data.yi ?? '',
      thumbnail: fileListFromURL(typeof data.thumbnail === 'string' ? data.thumbnail : ''),
      enabled: data.enabled,
    });
  },
});

defineExpose({ modalApi });
</script>

<template>
  <Modal :title="getTitle">
    <Form class="mx-4" />
  </Modal>
</template>
