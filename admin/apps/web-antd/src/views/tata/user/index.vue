<script lang="ts" setup>
import type {
  AppUser,
  InviteRelation,
  Membership,
  QuotaLot,
  RegistrationGift,
  YearlyQualification,
} from '#/api/tata';

import { ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Drawer, Image, Input, Select, Space, Table, Tag, message } from 'ant-design-vue';

import {
  downloadAdminCSV,
  getUser,
  listInvites,
  listMemberships,
  listQuotaLots,
  listRegistrationGifts,
  listUsers,
  listYearlyQualifications,
} from '#/api/tata';

const rows = ref<AppUser[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const q = ref('');
const cancelled = ref<string | undefined>(undefined);
const merged = ref<string | undefined>(undefined);
const open = ref(false);
const loading = ref(false);
const current = ref<AppUser | null>(null);
const memberships = ref<Membership[]>([]);
const gifts = ref<RegistrationGift[]>([]);
const invites = ref<InviteRelation[]>([]);
const lots = ref<QuotaLot[]>([]);
const years = ref<YearlyQualification[]>([]);
const recentReadings = ref<Array<Record<string, unknown>>>([]);
const recentOrders = ref<Array<Record<string, unknown>>>([]);

function prefs(row: AppUser) {
  return row.explore_preferences ?? [];
}

function lotSource(source: string) {
  if (source === 'subscription') return '套餐赠送';
  if (source === 'pack') return '补充包';
  if (source === 'admin') return '人工补发';
  return source;
}

function skuKind(kind: string) {
  if (kind === 'year') return '年卡';
  if (kind === 'month') return '月卡';
  return kind || '—';
}

function membershipSource(platform: string) {
  if (platform === 'admin') return '后台开通';
  if (!platform) return '—';
  return platform;
}

function yearSource(source: string) {
  if (source === 'purchase') return '单买';
  if (source === 'year_subscription') return '年卡';
  if (source === 'membership') return '会员';
  return source || '未记录';
}

function queryParams() {
  return {
    q: q.value.trim() || undefined,
    cancelled: cancelled.value,
    merged: merged.value,
    limit: pageSize.value,
    offset: (page.value - 1) * pageSize.value,
  };
}

async function load() {
  const data = await listUsers(queryParams());
  rows.value = data.items || [];
  total.value = data.total || 0;
}

async function exportCSV() {
  const sp = new URLSearchParams();
  const params = queryParams();
  Object.entries(params).forEach(([k, v]) => {
    if (v === undefined || v === null || v === '') return;
    if (k === 'limit' || k === 'offset') return;
    sp.set(k, String(v));
  });
  const qs = sp.toString();
  await downloadAdminCSV(`/users/export${qs ? `?${qs}` : ''}`, 'users.csv');
  message.success('已导出');
}

async function showEntitlements(row: AppUser) {
  current.value = row;
  open.value = true;
  loading.value = true;
  memberships.value = [];
  gifts.value = [];
  invites.value = [];
  lots.value = [];
  years.value = [];
  recentReadings.value = [];
  recentOrders.value = [];
  try {
    const [member, gift, invite, lot, year, detail] = await Promise.all([
      listMemberships(row.id),
      listRegistrationGifts(row.id),
      listInvites(row.id),
      listQuotaLots(row.id),
      listYearlyQualifications(row.id),
      getUser(row.id),
    ]);
    memberships.value = member.items || [];
    gifts.value = gift.items || [];
    invites.value = invite.items || [];
    lots.value = lot.items || [];
    years.value = year.items || [];
    recentReadings.value = detail.recent_readings || [];
    recentOrders.value = detail.recent_orders || [];
  } finally {
    loading.value = false;
  }
}

load();
</script>

<template>
  <Page
    description="搜索、分页、详情聚合与导出。功能使用仅含业务单据（占卜/订单），不是点击轨迹。"
    title="全部用户"
  >
    <div class="mb-4 flex flex-wrap gap-2">
      <Input
        v-model:value="q"
        allow-clear
        placeholder="ID / 昵称 / 邮箱 / 手机"
        style="max-width: 260px"
        @press-enter="
          () => {
            page = 1;
            load();
          }
        "
      />
      <Select
        v-model:value="cancelled"
        allow-clear
        placeholder="注销状态"
        style="width: 140px"
        :options="[
          { value: 'false', label: '正常' },
          { value: 'true', label: '已注销' },
        ]"
      />
      <Select
        v-model:value="merged"
        allow-clear
        placeholder="合并状态"
        style="width: 140px"
        :options="[
          { value: 'false', label: '未合并' },
          { value: 'true', label: '已合并' },
        ]"
      />
      <Space>
        <Button
          type="primary"
          @click="
            () => {
              page = 1;
              load();
            }
          "
        >
          查询
        </Button>
        <Button @click="exportCSV">导出 CSV</Button>
      </Space>
    </div>
    <Table
      :columns="[
        { title: 'ID', dataIndex: 'id', width: 72 },
        { title: '头像', dataIndex: 'avatar_url', width: 72 },
        { title: '昵称', dataIndex: 'nickname', width: 120 },
        { title: '性别', dataIndex: 'gender', width: 96 },
        { title: '地区', dataIndex: 'region_province', width: 120 },
        { title: '邮箱', dataIndex: 'email' },
        { title: '注册时间', dataIndex: 'created_at', width: 180 },
        { title: '状态', dataIndex: 'cancelled_at', width: 160 },
        { title: '详情', dataIndex: 'action', width: 80, fixed: 'right' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{
        current: page,
        pageSize,
        total,
        showSizeChanger: true,
        onChange: (p: number, ps: number) => {
          page = p;
          pageSize = ps;
          load();
        },
      }"
      :scroll="{ x: 1280 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'avatar_url'">
          <Image v-if="record.avatar_url" :src="record.avatar_url" :width="40" :height="40" />
          <span v-else>—</span>
        </template>
        <template v-else-if="column.dataIndex === 'nickname'">
          {{ record.nickname || record.name || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'gender'">
          {{ record.gender || '未填写' }}
        </template>
        <template v-else-if="column.dataIndex === 'region_province'">
          {{
            record.region_province
              ? `${record.region_province}${record.region_city ? ` · ${record.region_city}` : ''}`
              : '未知'
          }}
        </template>
        <template v-else-if="column.dataIndex === 'cancelled_at'">
          <Tag v-if="record.merged_into" color="orange">已合并 → {{ record.merged_into }}</Tag>
          <Tag v-else-if="record.cancelled_at" color="red">已注销</Tag>
          <Tag v-else color="green">正常</Tag>
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button type="link" @click="showEntitlements(record as AppUser)">详情</Button>
        </template>
      </template>
    </Table>
    <Drawer v-model:open="open" :title="`账号 ${current?.id ?? ''}`" width="820">
      <h3 class="mb-2 font-medium">最近占卜</h3>
      <Table
        :columns="[
          { title: 'ID', dataIndex: 'id', width: 80 },
          { title: '类型', dataIndex: 'type', width: 90 },
          { title: '状态', dataIndex: 'stage_label', width: 120 },
          { title: '问题', dataIndex: 'question' },
        ]"
        :data-source="recentReadings"
        :loading="loading"
        :locale="{ emptyText: '没有占卜记录' }"
        :pagination="false"
        row-key="id"
        size="small"
      />
      <h3 class="mb-2 mt-4 font-medium">最近订单</h3>
      <Table
        :columns="[
          { title: '订单', dataIndex: 'id', width: 80 },
          { title: '商品', dataIndex: 'sku_code' },
          { title: '验单', dataIndex: 'verify_status', width: 100 },
          { title: '金额(分)', dataIndex: 'price_amount_minor', width: 100 },
        ]"
        :data-source="recentOrders"
        :loading="loading"
        :locale="{ emptyText: '没有订单' }"
        :pagination="false"
        row-key="id"
        size="small"
      />
      <h3 class="mb-2 mt-4 font-medium">会员</h3>
      <Table
        :columns="[
          { title: '档位', dataIndex: 'sku_kind', width: 90 },
          { title: '状态', dataIndex: 'status', width: 90 },
          { title: '来源', dataIndex: 'platform', width: 110 },
          { title: '开始', dataIndex: 'starts_at' },
          { title: '到期', dataIndex: 'expires_at' },
          { title: '有效', dataIndex: 'active', width: 70 },
        ]"
        :data-source="memberships"
        :loading="loading"
        :locale="{ emptyText: '没有会员记录' }"
        :pagination="false"
        row-key="id"
        size="small"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'sku_kind'">
            {{ skuKind(record.sku_kind) }}
          </template>
          <template v-else-if="column.dataIndex === 'platform'">
            {{ membershipSource(record.platform) }}
          </template>
          <template v-else-if="column.dataIndex === 'active'">
            {{ record.active ? '是' : '否' }}
          </template>
        </template>
      </Table>
      <h3 class="mb-2 mt-4 font-medium">注册赠送</h3>
      <Table
        :columns="[
          { title: '发放', dataIndex: 'granted_at' },
          { title: '可用', dataIndex: 'usable_at' },
          { title: '到期', dataIndex: 'expires_at' },
          { title: '剩余', dataIndex: 'remaining', width: 80 },
          { title: '使用', dataIndex: 'used_at' },
        ]"
        :data-source="gifts"
        :loading="loading"
        :locale="{ emptyText: '没有注册赠送' }"
        :pagination="false"
        row-key="user_id"
        size="small"
      />
      <h3 class="mb-2 mt-4 font-medium">邀请</h3>
      <Table
        :columns="[
          { title: '邀请人', dataIndex: 'inviter_id', width: 90 },
          { title: '被邀请人', dataIndex: 'invitee_id', width: 100 },
          { title: '邀请人剩余', dataIndex: 'inviter_remaining', width: 110 },
          { title: '被邀请人剩余', dataIndex: 'invitee_remaining', width: 120 },
        ]"
        :data-source="invites"
        :loading="loading"
        :locale="{ emptyText: '没有邀请关系' }"
        :pagination="false"
        row-key="id"
        size="small"
      />
      <h3 class="mb-2 mt-4 font-medium">额度批次</h3>
      <Table
        :columns="[
          { title: '来源', dataIndex: 'source', width: 110 },
          { title: '剩余', dataIndex: 'remaining', width: 80 },
          { title: '总数', dataIndex: 'total', width: 80 },
          { title: '到期', dataIndex: 'expires_at' },
        ]"
        :data-source="lots"
        :loading="loading"
        :locale="{ emptyText: '没有额度批次' }"
        :pagination="false"
        row-key="id"
        size="small"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'source'">
            {{ lotSource(record.source) }}
          </template>
        </template>
      </Table>
      <h3 class="mb-2 mt-4 font-medium">年运资格</h3>
      <Table
        :columns="[
          { title: '年份', dataIndex: 'year', width: 80 },
          { title: '已解锁', dataIndex: 'unlocked', width: 90 },
          { title: '来源', dataIndex: 'unlock_source', width: 90 },
          { title: '报告状态', dataIndex: 'report_status' },
        ]"
        :data-source="years"
        :loading="loading"
        :locale="{ emptyText: '没有年运资格' }"
        :pagination="false"
        row-key="year"
        size="small"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'unlocked'">
            {{ record.unlocked ? '是' : '否' }}
          </template>
          <template v-else-if="column.dataIndex === 'unlock_source'">
            {{ yearSource(record.unlock_source) }}
          </template>
        </template>
      </Table>
    </Drawer>
  </Page>
</template>
