<script lang="ts" setup>
import type { IAPOrder, IAPQuota, IAPSku } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Input, Select, Space, Table, message } from 'ant-design-vue';

import { downloadAdminCSV, getIAPQuota, listIAPOrders, listIAPSkus } from '#/api/tata';

import { useTataLookups } from '../use-tata-lookups';

const userId = ref('');
const verifyStatus = ref<string | undefined>(undefined);
const platform = ref<string | undefined>(undefined);
const skuCode = ref('');
const page = ref(1);
const pageSize = ref(20);
const total = ref(0);
const orders = ref<IAPOrder[]>([]);
const skus = ref<IAPSku[]>([]);
const quota = ref<IAPQuota | null>(null);
const ledger = ref<Array<Record<string, unknown>>>([]);

const { loadLookups, userLabel } = useTataLookups();
const filterUserLabel = ref('');

function queryParams() {
  return {
    user_id: userId.value.trim() || undefined,
    verify_status: verifyStatus.value,
    platform: platform.value,
    sku_code: skuCode.value.trim() || undefined,
    limit: pageSize.value,
    offset: (page.value - 1) * pageSize.value,
  };
}

async function load() {
  await loadLookups();
  const data = await listIAPOrders(queryParams());
  orders.value = data.items || [];
  total.value = data.total || 0;
  const id = userId.value.trim();
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

async function exportCSV() {
  const sp = new URLSearchParams();
  const params = queryParams();
  Object.entries(params).forEach(([k, v]) => {
    if (v === undefined || v === null || v === '' || k === 'limit' || k === 'offset') return;
    sp.set(k, String(v));
  });
  const qs = sp.toString();
  await downloadAdminCSV(`/iap/orders/export${qs ? `?${qs}` : ''}`, 'iap_orders.csv');
  message.success('已导出');
}

function priceText(row: IAPOrder) {
  if (row.price_amount_minor == null) return '—';
  const yuan = (row.price_amount_minor / 100).toFixed(2);
  return row.price_estimated ? `${yuan}（估）` : yuan;
}

onMounted(async () => {
  skus.value = await listIAPSkus();
  await load();
});
</script>

<template>
  <Page
    description="按用户/验单/平台/SKU 筛选分页导出。金额优先订单快照，否则按当前 SKU 价估算。"
    title="IAP 订单"
  >
    <div class="mb-4 flex flex-wrap gap-2">
      <Input v-model:value="userId" placeholder="用户 ID" style="max-width: 160px" @press-enter="load" />
      <Select
        v-model:value="verifyStatus"
        allow-clear
        placeholder="验单状态"
        style="width: 140px"
        :options="[
          { value: 'pending', label: 'pending' },
          { value: 'verified', label: 'verified' },
          { value: 'failed', label: 'failed' },
        ]"
      />
      <Select
        v-model:value="platform"
        allow-clear
        placeholder="平台"
        style="width: 140px"
        :options="[
          { value: 'app_store', label: 'App Store' },
          { value: 'play', label: 'Play' },
        ]"
      />
      <Input v-model:value="skuCode" placeholder="SKU code" style="max-width: 160px" />
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
    <p v-if="quota" class="mb-4">
      {{ filterUserLabel }} · 额度 {{ quota.quota_remaining ?? quota.balance ?? 0 }}
    </p>
    <Table
      :columns="[
        { title: '订单', dataIndex: 'id', width: 80 },
        { title: '用户', dataIndex: 'user_id', width: 160 },
        { title: '商品', dataIndex: 'sku_code' },
        { title: '类型', dataIndex: 'sku_kind', width: 120 },
        { title: '平台', dataIndex: 'platform', width: 100 },
        { title: '验单', dataIndex: 'verify_status', width: 100 },
        { title: '入账', dataIndex: 'credit_status', width: 110 },
        { title: '金额(元)', dataIndex: 'price_amount_minor', width: 110 },
        { title: '时间', dataIndex: 'created_at', width: 180 },
      ]"
      :data-source="orders"
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
      :locale="{ emptyText: '暂无订单' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'user_id'">
          {{ userLabel(record.user_id) }}
        </template>
        <template v-else-if="column.dataIndex === 'price_amount_minor'">
          {{ priceText(record as IAPOrder) }}
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
