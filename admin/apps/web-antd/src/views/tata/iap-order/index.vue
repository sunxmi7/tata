<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Input, Table } from 'ant-design-vue';

import { getIAPDiamonds, listIAPOrders, listIAPSkus } from '#/api/tata';

const userId = ref('');
const orders = ref<Record<string, unknown>[]>([]);
const skus = ref<Record<string, unknown>[]>([]);
const diamonds = ref<Record<string, unknown> | null>(null);
const ledger = ref<Record<string, unknown>[]>([]);

async function load() {
  const id = userId.value.trim();
  orders.value = await listIAPOrders(id);
  if (id) {
    diamonds.value = await getIAPDiamonds(id);
    ledger.value = (diamonds.value.ledger as Record<string, unknown>[]) || [];
  } else {
    diamonds.value = null;
    ledger.value = [];
  }
}

onMounted(async () => {
  skus.value = await listIAPSkus();
  await load();
});
</script>

<template>
  <Page
    description="按用户查看验单状态、有效期和钻石入账。价格与发放数量未配置时为空。订阅钻石和次卡钻石分账。"
    title="IAP 订单"
  >
    <div class="mb-4 flex gap-2">
      <Input v-model:value="userId" placeholder="用户 ID" style="max-width: 240px" @press-enter="load" />
      <Button type="primary" @click="load">查询</Button>
    </div>
    <p v-if="diamonds" class="mb-4">
      订阅钻石 {{ diamonds.subscription_diamonds }} / 次卡钻石 {{ diamonds.pack_diamonds }}
    </p>
    <Table
      :columns="[
        { title: '订单', dataIndex: 'id' },
        { title: '用户', dataIndex: 'user_id' },
        { title: '商品', dataIndex: 'sku_code' },
        { title: '分账', dataIndex: 'ledger_bucket' },
        { title: '平台', dataIndex: 'platform' },
        { title: '验单', dataIndex: 'verify_status' },
        { title: '入账', dataIndex: 'credit_status' },
        { title: '钻石', dataIndex: 'credited_diamonds' },
        { title: '有效期', dataIndex: 'period_end' },
        { title: '订阅态', dataIndex: 'subscription_status' },
      ]"
      :data-source="orders"
      row-key="id"
      :pagination="{ pageSize: 20 }"
    />
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
        { title: '分账', dataIndex: 'ledger_bucket' },
        { title: 'App Store', dataIndex: 'app_store_product_id' },
        { title: 'Play', dataIndex: 'play_product_id' },
        { title: '价格', dataIndex: 'price_amount_minor' },
        { title: '钻石发放', dataIndex: 'diamond_grant' },
      ]"
      :data-source="skus"
      row-key="id"
      :pagination="false"
    />
  </Page>
</template>
