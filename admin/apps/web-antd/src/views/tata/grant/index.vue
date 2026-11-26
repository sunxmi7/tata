<script lang="ts" setup>
import type { GrantList, GrantRow } from '#/api/tata';

import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Input, InputNumber, Select, Table } from 'ant-design-vue';

import { createGrant, listGrants } from '#/api/tata';

import { useTataLookups } from '../use-tata-lookups';

const userId = ref('');
const quota = ref<number | null>(null);
const bucket = ref<'pack' | 'subscription'>('pack');
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
    quota_bucket: bucket.value,
    reason: 'admin_grant',
    note: note.value,
  });
  note.value = '';
  quota.value = null;
  await load();
}

onMounted(load);
</script>

<template>
  <Page
    description="只补会员额度，写入套餐赠送或补充包。不改历史订单。付费模块可写才能补发。"
    title="人工补发"
  >
    <div class="mb-4 flex flex-wrap items-center gap-2">
      <Input v-model:value="userId" placeholder="用户 ID" style="width: 160px" @press-enter="load" />
      <Button @click="load">查询流水</Button>
      <InputNumber v-model:value="quota" :min="1" placeholder="额度" style="width: 120px" />
      <Select
        v-model:value="bucket"
        :options="[
          { label: '补充包', value: 'pack' },
          { label: '套餐赠送', value: 'subscription' },
        ]"
        style="width: 140px"
      />
      <Input v-model:value="note" placeholder="原因说明" style="width: 240px" />
      <Button type="primary" :disabled="!canWrite" @click="submit">补发</Button>
    </div>
    <p v-if="view.user_id" class="mb-4">
      用户 {{ userLabel(view.user_id) }} · 剩余额度 {{ view.quota_remaining ?? view.balance ?? 0 }}
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
          {{
            record.kind === 'free_count'
              ? '历史记录'
              : record.bucket === 'subscription'
                ? '套餐赠送'
                : '补充包'
          }}
        </template>
        <template v-else-if="column.dataIndex === 'quota_ledger_id'">
          {{ record.quota_ledger_id ? `#${record.quota_ledger_id}` : '—' }}
        </template>
      </template>
    </Table>
  </Page>
</template>
