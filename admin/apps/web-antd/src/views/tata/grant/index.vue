<script lang="ts" setup>
import type { GrantList, GrantRow } from '#/api/tata';

import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Input, InputNumber, Table } from 'ant-design-vue';

import { createGrant, listGrants } from '#/api/tata';

import { useTataLookups } from '../use-tata-lookups';

const userId = ref('');
const quota = ref<number | null>(null);
const freeCount = ref<number | null>(null);
const note = ref('');
const view = ref<GrantList>({ grants: [] });
const rows = ref<GrantRow[]>([]);

const { loadLookups, operatorLabel, userLabel } = useTataLookups();

const canWrite = computed(() => view.value.can_write !== false);

async function load() {
  await loadLookups();
  const data = await listGrants(userId.value);
  view.value = data;
  rows.value = data.grants || [];
}

async function submit() {
  await createGrant({
    user_id: Number(userId.value.trim()),
    quota: quota.value || 0,
    free_count: freeCount.value || 0,
    quota_bucket: 'quota',
    reason: 'admin_grant',
    note: note.value,
  });
  note.value = '';
  quota.value = null;
  freeCount.value = null;
  await load();
}

onMounted(load);
</script>

<template>
  <Page
    description="独立加账，不改历史 IAP 订单，也不代替付费成功后的自动续跑。额度写入统一账户；免费次数记入可消耗余额。付费模块可写才能补发。"
    title="人工补发"
  >
    <div class="mb-4 flex flex-wrap items-center gap-2">
      <Input v-model:value="userId" placeholder="用户 ID" style="width: 160px" @press-enter="load" />
      <Button @click="load">查询流水</Button>
      <InputNumber v-model:value="quota" :min="0" placeholder="额度" style="width: 120px" />
      <InputNumber v-model:value="freeCount" :min="0" placeholder="免费次数" style="width: 140px" />
      <Input v-model:value="note" placeholder="原因说明" style="width: 240px" />
      <Button type="primary" :disabled="!canWrite" @click="submit">补发</Button>
    </div>
    <p v-if="view.user_id" class="mb-4">
      用户 {{ userLabel(view.user_id) }} · 额度 {{ view.quota_remaining ?? view.balance ?? 0 }} / 补发免费次数
      {{ view.free_count_remaining }} / 邀请次数 {{ view.invite_remaining ?? 0 }}
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
        { title: '额度流水', dataIndex: 'quota_ledger_id' },
        { title: '次数流水', dataIndex: 'count_ledger_id' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
      :locale="{ emptyText: '暂无补发记录' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'user_id'">
          {{ userLabel(record.user_id) }}
        </template>
        <template v-else-if="column.dataIndex === 'operator_id'">
          {{ operatorLabel(record.operator_id) }}
        </template>
        <template v-else-if="column.dataIndex === 'kind'">
          {{ record.kind === 'free_count' ? '免费次数' : '额度' }}
        </template>
        <template v-else-if="column.dataIndex === 'quota_ledger_id'">
          {{ record.quota_ledger_id ? `#${record.quota_ledger_id}` : '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'count_ledger_id'">
          {{ record.count_ledger_id ? `#${record.count_ledger_id}` : '—' }}
        </template>
      </template>
    </Table>
  </Page>
</template>
