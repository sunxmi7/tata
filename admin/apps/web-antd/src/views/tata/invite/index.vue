<script lang="ts" setup>
import type { InviteRelation } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table } from 'ant-design-vue';

import { listInvites } from '#/api/tata';

const rows = ref<InviteRelation[]>([]);

onMounted(async () => {
  const data = await listInvites();
  rows.value = data.items || [];
});
</script>

<template>
  <Page description="邀请码是邀请人的账号 ID。关系成功后双方各 3 次，从发放时刻起 1 年。这里只查看。" title="邀请关系">
    <Table
      :columns="[
        { title: '邀请人', dataIndex: 'inviter_id', width: 100 },
        { title: '被邀请人', dataIndex: 'invitee_id', width: 110 },
        { title: '邀请码', dataIndex: 'invite_code', width: 120 },
        { title: '来源', dataIndex: 'source_label', width: 90 },
        { title: '邀请人剩余', dataIndex: 'inviter_remaining', width: 110 },
        { title: '邀请人发放', dataIndex: 'inviter_granted_at' },
        { title: '邀请人到期', dataIndex: 'inviter_expires_at' },
        { title: '被邀请人剩余', dataIndex: 'invitee_remaining', width: 120 },
        { title: '被邀请人发放', dataIndex: 'invitee_granted_at' },
        { title: '被邀请人到期', dataIndex: 'invitee_expires_at' },
        { title: '建立时间', dataIndex: 'created_at' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
      :locale="{ emptyText: '还没有邀请关系' }"
    />
  </Page>
</template>
