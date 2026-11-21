<script lang="ts" setup>
import type { IAPQuota, IAPOrder, IAPSku } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Input, Table } from 'ant-design-vue';

import { getIAPQuota, listIAPOrders, listIAPSkus } from '#/api/tata';

import { useTataLookups } from '../use-tata-lookups';

const userId = ref('');
const orders = ref<IAPOrder[]>([]);
const skus = ref<IAPSku[]>([]);
const quota = ref<IAPQuota | null>(null);
const ledger = ref<Array<Record<string, unknown>>>([]);

const { loadLookups, userLabel } = useTataLookups();
const filterUserLabel = ref('');

async function load() {
  await loadLookups();
  const id = userId.value.trim();
  orders.value = await listIAPOrders(id);
  if (id) {
    quota.value = await getIAPQuota(id);
    ledger.value = quota.value.ledger || [];
    filterUserLabel.value = userLabel(Number(id));
  } else {
    quota.value = null;
    ledger.value = [];
    filterUserLabel.value = '';
  }
}

onMounted(async () => {
  skus.value = await listIAPSkus();
  await load();
});
</script>

<template>
  <Page
    description="按用户查看验单状态、有效期和额度入账。价格与发放数量未配置时为空。补充包仅会员可买，入账统一额度。"
    title="IAP 订单"
  >
    <div class="mb-4 flex flex-wrap gap-2">
      <Input v-model:value="userId" placeholder="用户 ID" style="max-width: 240px" @press-enter="load" />
      <Button type="primary" @click="load">查询</Button>
    </div>
    <p v-if="quota" class="mb-4">
      {{ filterUserLabel }} · 额度 {{ quota.quota_remaining ?? quota.balance ?? 0 }}
    </p>
    <Table
      :columns="[
        { title: '订单', dataIndex: 'id' },
        { title: '用户', dataIndex: 'user_id' },
        { title: '商品', dataIndex: 'sku_code' },
        { title: '类型', dataIndex: 'sku_kind' },
        { title: '平台', dataIndex: 'platform' },
        { title: '验单', dataIndex: 'verify_status' },
        { title: '入账', dataIndex: 'credit_status' },
        { title: '入账额度', dataIndex: 'credited_quota' },
        { title: '有效期', dataIndex: 'period_end' },
        { title: '订阅态', dataIndex: 'subscription_status' },
      ]"
      :data-source="orders"
      row-key="id"
      :pagination="{ pageSize: 20 }"
      :locale="{ emptyText: '暂无订单' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'user_id'">
          {{ userLabel(record.user_id) }}
        </template>
      </template>
    </Table>
    <Table
      v-if="ledger.length"
      class="mt-4"
      :columns="[
        { title: '分账', dataIndex: 'bucket' },
        { title: '变动', dataIndex: 'delta' },
        { title: '余额', dataIndex: 'balance_after' },
        { title: '原因', dataIndex: 'reason' },
        { title: '交易号', dataIndex: 'store_transaction_id' },
      ]"
      :data-source="ledger"
      row-key="id"
      :pagination="{ pageSize: 20 }"
    />
    <Table
      class="mt-4"
      :columns="[
        { title: '商品位', dataIndex: 'display_name' },
        { title: '类型', dataIndex: 'kind' },
        { title: 'App Store', dataIndex: 'app_store_product_id' },
        { title: 'Play', dataIndex: 'play_product_id' },
        { title: '价格', dataIndex: 'price_amount_minor' },
        { title: '开通赠送额度', dataIndex: 'quota_grant' },
      ]"
      :data-source="skus"
      row-key="id"
      :pagination="false"
      :locale="{ emptyText: '暂无 SKU 配置' }"
    />
  </Page>
</template>
