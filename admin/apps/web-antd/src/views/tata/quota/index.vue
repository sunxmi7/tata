<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Descriptions, InputNumber, message, Select } from 'ant-design-vue';

import { getQuota, listSpreads, updateQuota } from '#/api/tata';

import { hasTataWrite } from '../access';

const data = ref<Record<string, unknown>>({});
const dailyFreeCount = ref<number>(1);
const dailySlotCount = ref<number>(1);
const followupLimit = ref<number | null>(null);
const quotaCost = ref<number | null>(null);
const registrationDelay = ref<number | null>(null);
const registrationValidDays = ref<number>(3);
const registrationGrantCount = ref<number>(1);
const registrationSpreadIds = ref<number[]>([]);
const giftSpreadOptions = ref<Array<{ label: string; value: number }>>([]);
const saving = ref(false);

const giftSpreadHint = computed(() =>
  giftSpreadOptions.value.length
    ? '只列出 3 张及以内的普通占卜。不选就是空名单'
    : '还没有 3 张及以内的普通占卜牌阵',
);

const canWrite = hasTataWrite('ai');

function applyQuota(raw: Record<string, unknown>) {
  data.value = raw;
  dailyFreeCount.value = typeof raw.daily_free_count === 'number' ? raw.daily_free_count : 1;
  dailySlotCount.value = typeof raw.daily_slot_count === 'number' ? raw.daily_slot_count : 1;
  followupLimit.value = typeof raw.followup_limit === 'number' ? raw.followup_limit : null;
  quotaCost.value = typeof raw.quota_cost === 'number' ? raw.quota_cost : null;
  registrationDelay.value = typeof raw.registration_grant_delay_minutes === 'number' ? raw.registration_grant_delay_minutes : null;
  registrationValidDays.value = typeof raw.registration_valid_days === 'number' ? raw.registration_valid_days : 3;
  registrationGrantCount.value = typeof raw.registration_grant_count === 'number' ? raw.registration_grant_count : 1;
  registrationSpreadIds.value = Array.isArray(raw.registration_spread_ids)
    ? raw.registration_spread_ids.filter((id): id is number => typeof id === 'number')
    : [];
}

async function load() {
  const [quota, spreads] = await Promise.all([getQuota(), listSpreads()]);
  applyQuota(quota);
  giftSpreadOptions.value = spreads
    .filter((item) => !item.theme_key && item.card_count >= 1 && item.card_count <= 3)
    .map((item) => ({ label: `${item.name} · ${item.card_count} 张`, value: item.id }));
}

async function save() {
  saving.value = true;
  try {
    applyQuota(await updateQuota({
      daily_free_count: dailyFreeCount.value,
      daily_slot_count: dailySlotCount.value,
      followup_limit: followupLimit.value,
      quota_cost: quotaCost.value,
      registration_grant_delay_minutes: registrationDelay.value,
      registration_valid_days: registrationValidDays.value,
      registration_grant_count: registrationGrantCount.value,
      registration_spread_ids: registrationSpreadIds.value,
    }));
    message.success('已保存额度参数');
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <Page description="每日免费次数、今日一牌次数、追问上限和会员单次额度改完立刻生效。注册赠送间隔留空不自动发，填 0 表示注册完成马上发；次数和有效天数只作用于之后新注册、或尚未发放的账号。白名单先保存，结算还不按它扣。" title="额度参数">
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
      <Descriptions.Item label="注册赠送发放间隔">
        <div class="flex items-center gap-2">
          <InputNumber
            v-model:value="registrationDelay"
            :disabled="!canWrite"
            :min="0"
            :precision="0"
            placeholder="不自动发放"
            style="width: 140px"
          />
          <span class="text-muted-foreground text-xs">分钟。留空不发，0 为注册完成立即发</span>
        </div>
      </Descriptions.Item>
      <Descriptions.Item label="注册赠送有效天数">
        <div class="flex items-center gap-2">
          <InputNumber
            v-model:value="registrationValidDays"
            :disabled="!canWrite"
            :min="1"
            :precision="0"
            style="width: 140px"
          />
          <span class="text-muted-foreground text-xs">从实际发放时刻起算，默认 3 天</span>
        </div>
      </Descriptions.Item>
      <Descriptions.Item label="注册赠送次数">
        <div class="flex items-center gap-2">
          <InputNumber
            v-model:value="registrationGrantCount"
            :disabled="!canWrite"
            :min="1"
            :precision="0"
            style="width: 140px"
          />
          <span class="text-muted-foreground text-xs">每个新账号发放几次，默认 1</span>
        </div>
      </Descriptions.Item>
      <Descriptions.Item label="注册赠送白名单">
        <Select
          v-model:value="registrationSpreadIds"
          allow-clear
          :disabled="!canWrite"
          mode="multiple"
          :options="giftSpreadOptions"
          :placeholder="giftSpreadHint"
          style="width: 360px"
        />
      </Descriptions.Item>
    </Descriptions>
    <div v-if="canWrite" class="mt-4">
      <Button :loading="saving" type="primary" @click="save">保存</Button>
    </div>
  </Page>
</template>
