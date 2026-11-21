<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Descriptions, InputNumber, message } from 'ant-design-vue';

import { getQuota, updateQuota } from '#/api/tata';

import { hasTataWrite } from '../access';

const data = ref<Record<string, unknown>>({});
const dailyFreeCount = ref<number>(1);
const dailySlotCount = ref<number>(1);
const followupLimit = ref<number | null>(null);
const quotaCost = ref<number | null>(null);
const saving = ref(false);

const canWrite = hasTataWrite('ai');

async function load() {
  data.value = await getQuota();
  dailyFreeCount.value = typeof data.value.daily_free_count === 'number' ? data.value.daily_free_count : 1;
  dailySlotCount.value = typeof data.value.daily_slot_count === 'number' ? data.value.daily_slot_count : 1;
  const raw = data.value.followup_limit;
  followupLimit.value = typeof raw === 'number' ? raw : null;
  const cost = data.value.quota_cost;
  quotaCost.value = typeof cost === 'number' ? cost : null;
}

async function save() {
  saving.value = true;
  try {
    data.value = await updateQuota({
      daily_free_count: dailyFreeCount.value,
      daily_slot_count: dailySlotCount.value,
      followup_limit: followupLimit.value,
      quota_cost: quotaCost.value,
    });
    dailyFreeCount.value = typeof data.value.daily_free_count === 'number' ? data.value.daily_free_count : 1;
    dailySlotCount.value = typeof data.value.daily_slot_count === 'number' ? data.value.daily_slot_count : 1;
    const raw = data.value.followup_limit;
    followupLimit.value = typeof raw === 'number' ? raw : null;
    const cost = data.value.quota_cost;
    quotaCost.value = typeof cost === 'number' ? cost : null;
    message.success('已保存额度参数');
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <Page description="免费用户每日免费次数全局共用（含普通≤3 张与今日/明日）。会员单次额度空表示未配置，不编造。重置时刻暂不可改。" title="额度参数">
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
          <span class="text-muted-foreground text-xs">免费用户每天几次，不按玩法拆分</span>
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
      <Descriptions.Item label="会员单次额度">
        <div class="flex items-center gap-2">
          <InputNumber
            v-model:value="quotaCost"
            :disabled="!canWrite"
            :min="1"
            :precision="0"
            placeholder="未配置"
            style="width: 140px"
          />
          <span class="text-muted-foreground text-xs">会员每次玩法扣统一额度；空为未配置</span>
        </div>
      </Descriptions.Item>
    </Descriptions>
    <div v-if="canWrite" class="mt-4">
      <Button :loading="saving" type="primary" @click="save">保存</Button>
    </div>
  </Page>
</template>
