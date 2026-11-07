<script lang="ts" setup>
import type { Spread } from '#/api/tata';

import { computed, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { message } from 'ant-design-vue';

import { useVbenForm } from '#/adapter/form';
import { fileListFromURL, fileListToURL, imageUploadProps } from '#/api/core/upload';
import { createSpread, updateSpread } from '#/api/tata';

const emit = defineEmits(['success']);
const formData = ref<null | Spread>(null);

const getTitle = computed(() => (formData.value?.id ? '编辑牌阵' : '新建牌阵'));

const [Form, formApi] = useVbenForm({
  layout: 'vertical',
  schema: [
    {
      component: 'Input',
      fieldName: 'name',
      label: '名称',
      rules: 'required',
    },
    {
      component: 'Input',
      fieldName: 'description',
      label: '描述',
    },
    {
      component: 'InputNumber',
      componentProps: {
        min: 1,
      },
      defaultValue: 3,
      fieldName: 'card_count',
      label: '牌数',
      rules: 'required',
    },
    {
      component: 'Upload',
      componentProps: imageUploadProps('spreads'),
      fieldName: 'thumbnail',
      label: '示意图',
      renderComponentContent: () => {
        return {
          default: () => '上传图片',
        };
      },
    },
    {
      component: 'Textarea',
      componentProps: {
        rows: 6,
      },
      defaultValue:
        '[{"index":1,"name":"过去","meaning":""},{"index":2,"name":"现在","meaning":""},{"index":3,"name":"未来","meaning":""}]',
      fieldName: 'positionsText',
      label: '牌位 JSON',
      rules: 'required',
    },
    {
      component: 'Switch',
      defaultValue: true,
      fieldName: 'enabled',
      label: '启用',
    },
    {
      component: 'Switch',
      defaultValue: false,
      fieldName: 'recommended',
      label: '推荐',
    },
    {
      component: 'Switch',
      defaultValue: false,
      fieldName: 'charged',
      label: '是否收费',
    },
  ],
  showDefaultActions: false,
});

const [Modal, modalApi] = useVbenModal<null | Spread>({
  async onConfirm() {
    const { valid } = await formApi.validate();
    if (!valid) {
      return;
    }
    const data = await formApi.getValues();
    let positions: Spread['positions'];
    try {
      positions = JSON.parse(String(data.positionsText ?? '')) as Spread['positions'];
    } catch {
      message.error('牌位 JSON 不正确');
      return;
    }
    modalApi.lock();
    try {
      const body = {
        name: data.name,
        description: data.description,
        card_count: data.card_count,
        positions,
        enabled: data.enabled,
        charged: data.charged,
        recommended: data.recommended,
        thumbnail: fileListToURL(data.thumbnail),
      };
      if (formData.value?.id) {
        await updateSpread(formData.value.id, body);
      } else {
        await createSpread(body);
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
      return;
    }
    const data = modalApi.getData();
    if (data?.id) {
      formData.value = data;
      formApi.setValues({
        name: data.name,
        description: data.description,
        card_count: data.card_count,
        positionsText: JSON.stringify(data.positions, null, 2),
        enabled: data.enabled,
        charged: data.charged,
        recommended: data.recommended ?? false,
        thumbnail: fileListFromURL(data.thumbnail),
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
