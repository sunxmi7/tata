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
      fieldName: 'name_en',
      label: '英文名称',
    },
    {
      component: 'Input',
      fieldName: 'description',
      label: '含义说明',
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
      component: 'Select',
      componentProps: {
        options: [
          { label: '不分组', value: '' },
          { label: '快速指引', value: 'quick' },
          { label: '进阶分析', value: 'advanced' },
          { label: '深度全景', value: 'panorama' },
        ],
      },
      defaultValue: '',
      fieldName: 'depth_group',
      label: '普通占卜分组',
    },
    {
      component: 'Input',
      fieldName: 'theme_key',
      label: '探索主题键',
    },
    {
      component: 'Input',
      fieldName: 'theme_title',
      label: '探索主题',
    },
    {
      component: 'Input',
      fieldName: 'theme_subtitle',
      label: '主题副标题',
    },
    {
      component: 'Input',
      fieldName: 'short_sentence',
      label: '一句说明',
    },
    {
      component: 'Switch',
      defaultValue: false,
      fieldName: 'quota_priced',
      label: '已标额度扣次',
    },
    {
      component: 'InputNumber',
      componentProps: {
        min: 0,
      },
      fieldName: 'quota_cost',
      label: '额度扣次',
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
      component: 'Textarea',
      componentProps: {
        placeholder: '每行一条；探索牌阵问题页点击后填入输入框，不直接提交',
        rows: 4,
      },
      defaultValue: '',
      fieldName: 'quickQuestionsText',
      label: '快捷问题',
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
      label: '旧收费开关',
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
    if (data.quota_priced && (data.quota_cost === null || data.quota_cost === undefined || data.quota_cost === '')) {
      message.error('请填写额度扣次');
      return;
    }
    let positions: Spread['positions'];
    try {
      positions = JSON.parse(String(data.positionsText ?? '')) as Spread['positions'];
    } catch {
      message.error('牌位 JSON 不正确');
      return;
    }
    const quick_questions = String(data.quickQuestionsText ?? '')
      .split(/\r?\n/)
      .map((line: string) => line.trim())
      .filter(Boolean);
    modalApi.lock();
    try {
      const body = {
        name: data.name,
        name_en: data.name_en ?? '',
        description: data.description,
        card_count: data.card_count,
        positions,
        quick_questions,
        enabled: data.enabled,
        charged: data.charged,
        recommended: data.recommended,
        thumbnail: fileListToURL(data.thumbnail),
        depth_group: data.depth_group ?? '',
        theme_key: data.theme_key ?? '',
        theme_title: data.theme_title ?? '',
        theme_subtitle: data.theme_subtitle ?? '',
        short_sentence: data.short_sentence ?? '',
        quota_priced: Boolean(data.quota_priced),
        quota_cost: data.quota_priced ? data.quota_cost : null,
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
        name_en: data.name_en ?? '',
        description: data.description,
        card_count: data.card_count,
        positionsText: JSON.stringify(data.positions, null, 2),
        quickQuestionsText: (data.quick_questions ?? []).join('\n'),
        enabled: data.enabled,
        charged: data.charged,
        recommended: data.recommended ?? false,
        thumbnail: fileListFromURL(data.thumbnail),
        depth_group: data.depth_group ?? '',
        theme_key: data.theme_key ?? '',
        theme_title: data.theme_title ?? '',
        theme_subtitle: data.theme_subtitle ?? '',
        short_sentence: data.short_sentence ?? '',
        quota_priced: Boolean(data.quota_priced),
        quota_cost: data.quota_cost,
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
