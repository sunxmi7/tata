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

import { Button, Drawer, Image, Table, Tag } from 'ant-design-vue';

import {
  listInvites,
  listMemberships,
  listQuotaLots,
  listRegistrationGifts,
  listUsers,
  listYearlyQualifications,
} from '#/api/tata';

const rows = ref<AppUser[]>([]);
const open = ref(false);
const loading = ref(false);
const current = ref<AppUser | null>(null);
const memberships = ref<Membership[]>([]);
const gifts = ref<RegistrationGift[]>([]);
const invites = ref<InviteRelation[]>([]);
const lots = ref<QuotaLot[]>([]);
const years = ref<YearlyQualification[]>([]);

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

async function load() {
  rows.value = await listUsers();
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
  try {
    const [member, gift, invite, lot, year] = await Promise.all([
      listMemberships(row.id),
      listRegistrationGifts(row.id),
      listInvites(row.id),
      listQuotaLots(row.id),
      listYearlyQualifications(row.id),
    ]);
    memberships.value = member.items || [];
    gifts.value = gift.items || [];
    invites.value = invite.items || [];
    lots.value = lot.items || [];
    years.value = year.items || [];
  } finally {
    loading.value = false;
  }
}

load();
</script>

<template>
  <Page
    description="头像、昵称、性别、探索偏好与注销状态。点「权益」查看这个账号的会员、注册赠送、邀请、额度批次和年运资格。注册奖励不在这里扣。"
    title="用户账号"
  >
    <Table
      :columns="[
        { title: 'ID', dataIndex: 'id', width: 72 },
        { title: '头像', dataIndex: 'avatar_url', width: 72 },
        { title: '昵称', dataIndex: 'nickname', width: 120 },
        { title: '性别', dataIndex: 'gender', width: 96 },
        { title: '生日', dataIndex: 'birth_date', width: 120 },
        { title: '探索偏好', dataIndex: 'explore_preferences' },
        { title: '邮箱', dataIndex: 'email' },
        { title: '微信', dataIndex: 'wechat_openid', ellipsis: true },
        { title: '苹果', dataIndex: 'apple_sub', ellipsis: true },
        { title: '时区', dataIndex: 'timezone', width: 140 },
        { title: '状态', dataIndex: 'cancelled_at', width: 180 },
        { title: '权益', dataIndex: 'action', width: 80, fixed: 'right' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
      :scroll="{ x: 1560 }"
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
        <template v-else-if="column.dataIndex === 'birth_date'">
          {{ record.birth_date || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'explore_preferences'">
          <template v-if="prefs(record as AppUser).length">
            <Tag v-for="item in prefs(record as AppUser)" :key="item" class="mb-1 mr-1">{{ item }}</Tag>
          </template>
          <span v-else>未选择</span>
        </template>
        <template v-else-if="column.dataIndex === 'cancelled_at'">
          <Tag v-if="record.cancelled_at" color="red">已注销 {{ record.cancelled_at }}</Tag>
          <Tag v-else color="green">正常</Tag>
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button type="link" @click="showEntitlements(record as AppUser)">权益</Button>
        </template>
      </template>
    </Table>
    <Drawer v-model:open="open" :title="`账号 ${current?.id ?? ''} 的权益`" width="760">
      <h3 class="mb-2 font-medium">会员</h3>
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
          { title: '邀请人到期', dataIndex: 'inviter_expires_at' },
          { title: '被邀请人剩余', dataIndex: 'invitee_remaining', width: 120 },
          { title: '被邀请人到期', dataIndex: 'invitee_expires_at' },
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
          { title: '已到期', dataIndex: 'expired', width: 80 },
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
          <template v-else-if="column.dataIndex === 'expired'">
            {{ record.expired ? '是' : '否' }}
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
