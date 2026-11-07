<script lang="ts" setup>
import { useVbenModal } from '@vben/common-ui';

import { message } from 'ant-design-vue';

import { useVbenForm } from '#/adapter/form';
import { fileListFromURL, fileListToURL, imageUploadProps } from '#/api/core/upload';
import { updateCopy } from '#/api/tata';

const emit = defineEmits(['success']);

type CardFormData = null | Record<string, unknown>;

const [Form, formApi] = useVbenForm({
  layout: 'vertical',
  schema: [
    {
      component: 'Input',
      componentProps: {
        disabled: true,
      },
      fieldName: 'card_name',
      label: '牌',
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
  ],
  showDefaultActions: false,
});

const [Modal, modalApi] = useVbenModal<CardFormData>({
  async onConfirm() {
    const { valid } = await formApi.validate();
    if (!valid) {
      return;
    }
    const data = await formApi.getValues();
    modalApi.lock();
    try {
      await updateCopy('cards', {
        card_name: data.card_name,
        thumbnail: fileListToURL(data.thumbnail),
      });
      message.success('已保存');
      modalApi.close();
      emit('success');
    } finally {
      modalApi.lock(false);
    }
  },
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }
    const data = modalApi.getData();
    if (!data) {
      formApi.reset();
      return;
    }
    formApi.setValues({
      card_name: data.card_name,
      thumbnail: fileListFromURL(
        typeof data.thumbnail === 'string' ? data.thumbnail : '',
      ),
    });
  },
});

defineExpose({ modalApi });
</script>

<template>
  <Modal title="单牌图">
    <Form class="mx-4" />
  </Modal>
</template>
