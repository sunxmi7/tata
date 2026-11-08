<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import { Button, Descriptions, InputNumber, message } from 'ant-design-vue';

import { getQuota, updateQuota } from '#/api/tata';

const access = useAccessStore();
const data = ref<Record<string, unknown>>({});
const dailyFreeCount = ref<number>(1);
const dailySlotCount = ref<number>(1);
const followupLimit = ref<number | null>(null);
const saving = ref(false);

const canWrite = access.accessCodes.includes('write');

async function load() {
  data.value = await getQuota();
  dailyFreeCount.value = typeof data.value.daily_free_count === 'number' ? data.value.daily_free_count : 1;
  dailySlotCount.value = typeof data.value.daily_slot_count === 'number' ? data.value.daily_slot_count : 1;
  const raw = data.value.followup_limit;
  followupLimit.value = typeof raw === 'number' ? raw : null;
}

async function save() {
  saving.value = true;
  try {
    data.value = await updateQuota({
      daily_free_count: dailyFreeCount.value,
      daily_slot_count: dailySlotCount.value,
      followup_limit: followupLimit.value,
    });
    dailyFreeCount.value = typeof data.value.daily_free_count === 'number' ? data.value.daily_free_count : 1;
    dailySlotCount.value = typeof data.value.daily_slot_count === 'number' ? data.value.daily_slot_count : 1;
    const raw = data.value.followup_limit;
    followupLimit.value = typeof raw === 'number' ? raw : null;
    message.success('已保存额度参数');
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <Page description="每日免费次数和今日一牌次数可随时改，方便测试。追问上限空表示不限制。重置时刻暂不可改。" title="额度参数">
    <Descriptions bordered :column="1">
      <Descriptions.Item label="重置时刻">{{ data.reset_at }}</Descriptions.Item>
      <Descriptions.Item label="每日免费次数">
        <div class="flex items-center gap-2">
          <InputNumber
            v-model:value="dailyFreeCount"
            :disabled="!canWrite"
            :min="1"
            :precision="0"
            style="width: 140px"
          />
          <span class="text-muted-foreground text-xs">普通占卜每天免费几次</span>
        </div>
      </Descriptions.Item>
      <Descriptions.Item label="今日一牌次数">
        <div class="flex items-center gap-2">
          <InputNumber
            v-model:value="dailySlotCount"
            :disabled="!canWrite"
            :min="1"
            :precision="0"
            style="width: 140px"
          />
          <span class="text-muted-foreground text-xs">每天可抽今日一牌几次</span>
        </div>
      </Descriptions.Item>
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
        </div>
      </Descriptions.Item>
    </Descriptions>
    <div v-if="canWrite" class="mt-4">
      <Button :loading="saving" type="primary" @click="save">保存</Button>
    </div>
  </Page>
</template>
