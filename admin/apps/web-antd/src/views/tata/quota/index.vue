<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import { Button, Descriptions, InputNumber, message } from 'ant-design-vue';

import { getQuota, updateQuota } from '#/api/tata';

const access = useAccessStore();
const data = ref<Record<string, unknown>>({});
const followupLimit = ref<number | null>(null);
const saving = ref(false);

const canWrite = access.accessCodes.includes('write');

async function load() {
  data.value = await getQuota();
  const raw = data.value.followup_limit;
  followupLimit.value = typeof raw === 'number' ? raw : null;
}

async function saveFollowup() {
  saving.value = true;
  try {
    data.value = await updateQuota({ followup_limit: followupLimit.value });
    const raw = data.value.followup_limit;
    followupLimit.value = typeof raw === 'number' ? raw : null;
    message.success('已保存追问上限');
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <Page description="每日免费次数本期只读。每条解读的追问上限可改，空表示不限制。" title="额度参数">
    <Descriptions bordered :column="1">
      <Descriptions.Item label="重置时刻">{{ data.reset_at }}</Descriptions.Item>
      <Descriptions.Item label="每日免费资格">{{ data.daily_free_count }}</Descriptions.Item>
      <Descriptions.Item label="共用范围">{{ data.shared_scope }}</Descriptions.Item>
      <Descriptions.Item label="失败">{{ data.failure_policy }}</Descriptions.Item>
      <Descriptions.Item label="追问上限">
        <div class="flex items-center gap-2">
          <InputNumber
            v-model:value="followupLimit"
            :disabled="!canWrite"
            :min="1"
            placeholder="不限制"
            style="width: 140px"
          />
          <Button v-if="canWrite" :loading="saving" type="primary" @click="saveFollowup">保存</Button>
        </div>
      </Descriptions.Item>
    </Descriptions>
  </Page>
</template>
