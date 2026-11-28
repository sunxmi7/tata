<script lang="ts" setup>
import type { Membership, MembershipList } from '#/api/tata';

import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Input, Select, Table } from 'ant-design-vue';

import { grantMembership, listMemberships } from '#/api/tata';

import { useTataLookups } from '../use-tata-lookups';

const userId = ref('');
const kind = ref<'month' | 'year'>('month');
const note = ref('');
const view = ref<MembershipList>({ items: [] });
const rows = ref<Membership[]>([]);

const { loadLookups, userLabel } = useTataLookups();

const canWrite = computed(() => view.value.can_write !== false);

function kindLabel(value: string) {
  if (value === 'year') return '年卡';
  if (value === 'month') return '月卡';
  return value;
}

function platformLabel(value: string) {
  if (value === 'admin') return '后台开通';
  if (!value) return '—';
  return value;
}

async function load() {
  await loadLookups();
  const id = Number(userId.value.trim());
  const data = await listMemberships(id > 0 ? id : undefined);
  view.value = data;
  rows.value = data.items || [];
}

async function submit() {
  await grantMembership({
    user_id: Number(userId.value.trim()),
    sku_kind: kind.value,
    note: note.value,
  });
  note.value = '';
  await load();
}

onMounted(load);
</script>

<template>
  <Page
    description="开通或延长有效会员。月卡 30 天，年卡 365 天。已有会员从到期日往后加，不写商店订单。付费模块可写才能开通。"
    title="会员"
  >
    <div class="mb-4 flex flex-wrap items-center gap-2">
      <Input v-model:value="userId" placeholder="用户 ID" style="width: 160px" @press-enter="load" />
      <Button @click="load">查询</Button>
      <Select
        v-model:value="kind"
        :options="[
          { label: '月卡 · 30 天', value: 'month' },
          { label: '年卡 · 365 天', value: 'year' },
        ]"
        style="width: 160px"
      />
      <Input v-model:value="note" placeholder="开通原因" style="width: 240px" />
      <Button type="primary" :disabled="!canWrite" @click="submit">开通或延长</Button>
    </div>
    <p v-if="view.user_id" class="mb-4">用户 {{ userLabel(view.user_id) }}</p>
    <p class="mb-4 text-sm opacity-70">{{ view.permission_note }}</p>
    <Table
      :columns="[
        { title: '用户', dataIndex: 'user_id' },
        { title: '档位', dataIndex: 'sku_kind' },
        { title: '状态', dataIndex: 'status' },
        { title: '来源', dataIndex: 'platform' },
        { title: '开始', dataIndex: 'starts_at' },
        { title: '到期', dataIndex: 'expires_at' },
        { title: '有效', dataIndex: 'active' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'user_id'">
          {{ userLabel(record.user_id) }}
        </template>
        <template v-else-if="column.dataIndex === 'sku_kind'">
          {{ kindLabel(record.sku_kind) }}
        </template>
        <template v-else-if="column.dataIndex === 'platform'">
          {{ platformLabel(record.platform) }}
        </template>
        <template v-else-if="column.dataIndex === 'active'">
          {{ record.active ? '是' : '否' }}
        </template>
      </template>
    </Table>
  </Page>
</template>
