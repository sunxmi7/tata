<script lang="ts" setup>
import type { Spread } from '#/api/tata';

import { computed, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { message } from 'ant-design-vue';

import { useVbenForm } from '#/adapter/form';
import { createSpread, updateSpread } from '#/api/tata';

type SpreadFormPayload = {
  mode?: 'explore' | 'ordinary';
  spread?: null | Spread;
};

const emit = defineEmits(['success']);
const formData = ref<null | Spread>(null);
const formMode = ref<'explore' | 'ordinary'>('ordinary');

const getTitle = computed(() => {
  const kind = formMode.value === 'explore' ? '探索牌阵' : '普通占卜牌阵';
  return formData.value?.id ? `编辑${kind}` : `新建${kind}`;
});

const [Form, formApi] = useVbenForm({
  layout: 'vertical',
  schema: [
    {
      component: 'Input',
      fieldName: '_mode',
      formItemClass: 'hidden',
      hideLabel: true,
    },
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
      dependencies: {
        show: (values) => values._mode !== 'explore',
        triggerFields: ['_mode'],
      },
      fieldName: 'depth_group',
      label: '普通占卜分组',
    },
    {
      component: 'Input',
      dependencies: {
        show: (values) => values._mode === 'explore',
        triggerFields: ['_mode'],
      },
      fieldName: 'theme_key',
      label: '探索主题键',
      rules: 'required',
    },
    {
      component: 'Input',
      dependencies: {
        show: (values) => values._mode === 'explore',
        triggerFields: ['_mode'],
      },
      fieldName: 'theme_title',
      label: '探索主题',
    },
    {
      component: 'Input',
      dependencies: {
        show: (values) => values._mode === 'explore',
        triggerFields: ['_mode'],
      },
      fieldName: 'theme_subtitle',
      label: '主题副标题',
    },
    {
      component: 'Input',
      dependencies: {
        show: (values) => values._mode === 'explore',
        triggerFields: ['_mode'],
      },
      fieldName: 'short_sentence',
      label: '一句说明',
    },
    {
      component: 'Switch',
      defaultValue: false,
      fieldName: 'quota_priced',
      label: '已标额度消耗',
    },
    {
      component: 'InputNumber',
      componentProps: {
        min: 0,
      },
      fieldName: 'quota_cost',
      label: '额度消耗',
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
      dependencies: {
        show: (values) => values._mode === 'explore',
        triggerFields: ['_mode'],
      },
      fieldName: 'quickQuestionsText',
      label: '快捷问题',
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

const [Modal, modalApi] = useVbenModal<null | SpreadFormPayload>({
  async onConfirm() {
    const { valid } = await formApi.validate();
    if (!valid) {
      return;
    }
    const data = await formApi.getValues();
    if (data.quota_priced && (data.quota_cost === null || data.quota_cost === undefined || data.quota_cost === '')) {
      message.error('请填写额度消耗');
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
    const isExplore = formMode.value === 'explore';
    modalApi.lock();
    try {
      // 不提交 charged：更新时服务端保留库内原值；新建默认 false。
      const body = {
        name: data.name,
        name_en: data.name_en ?? '',
        description: data.description,
        card_count: data.card_count,
        positions,
        quick_questions: isExplore ? quick_questions : [],
        enabled: data.enabled,
        recommended: formData.value?.recommended ?? false,
        thumbnail: formData.value?.thumbnail ?? '',
        depth_group: isExplore ? '' : (data.depth_group ?? ''),
        theme_key: isExplore ? String(data.theme_key ?? '').trim() : '',
        theme_title: isExplore ? (data.theme_title ?? '') : '',
        theme_subtitle: isExplore ? (data.theme_subtitle ?? '') : '',
        short_sentence: isExplore ? (data.short_sentence ?? '') : '',
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
    const payload = modalApi.getData() || {};
    const data = payload.spread ?? null;
    formMode.value = payload.mode === 'explore' || !!String(data?.theme_key || '').trim()
      ? 'explore'
      : 'ordinary';
    if (data?.id) {
      formData.value = data;
      formApi.setValues({
        _mode: formMode.value,
        name: data.name,
        name_en: data.name_en ?? '',
        description: data.description,
        card_count: data.card_count,
        positionsText: JSON.stringify(data.positions, null, 2),
        quickQuestionsText: (data.quick_questions ?? []).join('\n'),
        enabled: data.enabled,
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
      formApi.setValues({
        _mode: formMode.value,
        depth_group: formMode.value === 'ordinary' ? 'quick' : '',
        theme_key: '',
      });
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
