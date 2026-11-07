<script lang="ts" setup>
import { computed, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { message } from 'ant-design-vue';

import { useVbenForm } from '#/adapter/form';
import { fileListFromURL, fileListToURL, imageUploadProps } from '#/api/core/upload';
import { updateCopy } from '#/api/tata';

const emit = defineEmits(['success']);

type StyleFormData = null | Record<string, unknown>;

const formData = ref<StyleFormData>(null);
const getTitle = computed(() => (formData.value?.code ? '编辑风格' : '添加风格'));

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
      fieldName: 'thumbnail',
      label: '预览图',
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

const [Modal, modalApi] = useVbenModal<StyleFormData>({
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
        thumbnail: fileListToURL(data.thumbnail),
        enabled: data.enabled,
        sort_order: formData.value?.sort_order,
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
    if (data?.code) {
      formData.value = data;
      formApi.setValues({
        code: data.code,
        name: data.name,
        enabled: data.enabled ?? true,
        thumbnail: fileListFromURL(
          typeof data.thumbnail === 'string' ? data.thumbnail : '',
        ),
      });
    } else {
      formData.value = null;
      formApi.reset();
    }
  },
});

defineExpose({ modalApi });
</script>

<template>
  <Modal :title="getTitle">
    <Form class="mx-4" />
  </Modal>
</template>
