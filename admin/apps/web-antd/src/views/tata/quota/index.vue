<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Descriptions } from 'ant-design-vue';

import { getQuota } from '#/api/tata';

const data = ref<Record<string, unknown>>({});
onMounted(async () => {
  data.value = await getQuota();
});
</script>

<template>
  <Page description="本期只读。没有时区下拉，也不能改每日次数。" title="额度参数">
    <Descriptions bordered :column="1">
      <Descriptions.Item label="重置时刻">{{ data.reset_at }}</Descriptions.Item>
      <Descriptions.Item label="每日免费资格">{{ data.daily_free_count }}</Descriptions.Item>
      <Descriptions.Item label="共用范围">{{ data.shared_scope }}</Descriptions.Item>
      <Descriptions.Item label="失败">{{ data.failure_policy }}</Descriptions.Item>
      <Descriptions.Item label="可编辑">{{ data.editable ? '是' : '否' }}</Descriptions.Item>
    </Descriptions>
  </Page>
</template>
