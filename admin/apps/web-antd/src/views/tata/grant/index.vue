<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Input, InputNumber, Select, Table } from 'ant-design-vue';

import { createGrant, listGrants } from '#/api/tata';

const userId = ref('');
const diamonds = ref<number | null>(null);
const freeCount = ref<number | null>(null);
const bucket = ref('pack');
const note = ref('');
const view = ref<Record<string, unknown>>({});
const rows = ref<Record<string, unknown>[]>([]);

const canWrite = computed(() => view.value.can_write !== false);

async function load() {
  const data = await listGrants(userId.value);
  view.value = data;
  rows.value = (data.grants as Record<string, unknown>[]) || [];
}

async function submit() {
  await createGrant({
    user_id: Number(userId.value.trim()),
    diamonds: diamonds.value || 0,
    free_count: freeCount.value || 0,
    diamond_bucket: bucket.value,
    reason: 'admin_grant',
    note: note.value,
  });
  note.value = '';
  diamonds.value = null;
  freeCount.value = null;
  await load();
}

onMounted(load);
</script>

<template>
  <Page
    description="独立加账，不改历史 IAP 订单，也不代替付费成功后的自动续跑。钻石默认补到 pack_diamonds（流水桶 consumable）；选 subscription 则补到 subscription_diamonds。免费次数记入可消耗余额。role=admin 可写，readonly 只可查。"
    title="人工补发"
  >
    <div class="mb-4 flex flex-wrap items-center gap-2">
      <Input v-model:value="userId" placeholder="用户 ID" style="width: 160px" @press-enter="load" />
      <Button @click="load">查询流水</Button>
      <InputNumber v-model:value="diamonds" :min="0" placeholder="钻石" style="width: 120px" />
      <Select
        v-model:value="bucket"
        style="width: 280px"
        :options="[
          { label: 'pack（pack_diamonds，默认）', value: 'pack' },
          { label: 'subscription（subscription_diamonds）', value: 'subscription' },
        ]"
      />
      <InputNumber v-model:value="freeCount" :min="0" placeholder="免费次数" style="width: 140px" />
      <Input v-model:value="note" placeholder="原因说明" style="width: 240px" />
      <Button type="primary" :disabled="!canWrite" @click="submit">补发</Button>
    </div>
    <p v-if="view.user_id" class="mb-4">
      次卡钻石 {{ view.pack_diamonds }} / 订阅钻石 {{ view.subscription_diamonds }} / 补发免费次数
      {{ view.free_count_remaining }}
    </p>
    <p class="mb-4 text-sm opacity-70">{{ view.permission_note }}</p>
    <Table
      :columns="[
        { title: '时间', dataIndex: 'created_at' },
        { title: '用户', dataIndex: 'user_id' },
        { title: '操作者', dataIndex: 'operator_id' },
        { title: '类型', dataIndex: 'kind' },
        { title: '数量', dataIndex: 'amount' },
        { title: '桶', dataIndex: 'bucket' },
        { title: '流水桶', dataIndex: 'ledger_bucket' },
        { title: '原因', dataIndex: 'reason' },
        { title: '说明', dataIndex: 'note' },
        { title: '钻石流水', dataIndex: 'diamond_ledger_id' },
        { title: '次数流水', dataIndex: 'count_ledger_id' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
    />
  </Page>
</template>
