<script lang="ts" setup>
import type { DeckProduct } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page, useVbenModal } from '@vben/common-ui';
import { Plus } from '@vben/icons';

import { Button, Form, Image, Input, InputNumber, Modal, Switch, Table, message } from 'ant-design-vue';

import { getCopy, listDeckProducts, updateDeckProduct } from '#/api/tata';

import { hasTataWrite } from '../access';
import StyleForm from './modules/style-form.vue';

const kindLabel: Record<string, string> = {
  deck_year: '年度畅享',
  deck_bundle: '套餐',
  deck_single: '单套',
  deck_addon: '后续打包',
};

const canWrite = hasTataWrite('content');
const decks = ref<Record<string, unknown>[]>([]);
const products = ref<DeckProduct[]>([]);
const editing = ref<DeckProduct | null>(null);
const productForm = ref({ name: '', price_amount_minor: 0, note: '', enabled: true });

const [StyleModal, styleModalApi] = useVbenModal({
  connectedComponent: StyleForm,
  destroyOnClose: true,
});

async function loadDecks() {
  const data = await getCopy('decks');
  decks.value = (data.items as Record<string, unknown>[]) || [];
  products.value = await listDeckProducts();
}

function yuan(minor: number) {
  return (minor / 100).toFixed(2);
}

function onEditProduct(row: DeckProduct) {
  editing.value = row;
  productForm.value = {
    name: row.name,
    price_amount_minor: row.price_amount_minor,
    note: row.note,
    enabled: row.enabled,
  };
}

async function saveProduct() {
  if (!editing.value) {
    return;
  }
  if (productForm.value.price_amount_minor < 0) {
    message.error('价格不能为负');
    return;
  }
  await updateDeckProduct(editing.value.code, productForm.value);
  message.success('已保存');
  editing.value = null;
  await loadDecks();
}

function onCreateStyle() {
  styleModalApi.setData(null).open();
}

function onEditStyle(row: Record<string, unknown>) {
  styleModalApi.setData(row).open();
}

onMounted(loadDecks);
</script>

<template>
  <Page
    description="上面是牌面风格。下面的售价和价位页是同一条商品，单位为分。已单买再买套餐不抵扣，也不加额度。"
    title="牌组"
  >
    <template #extra>
      <Button v-if="canWrite" type="primary" @click="onCreateStyle">
        <Plus class="size-5" />
        添加风格
      </Button>
    </template>
    <StyleModal @success="loadDecks" />
    <Table
      :columns="[
        { title: '编码', dataIndex: 'code', width: 160 },
        { title: '名称', dataIndex: 'name', width: 160 },
        { title: '预览图', dataIndex: 'preview', width: 88 },
        { title: '启用', dataIndex: 'enabled', width: 80 },
        { title: '操作', dataIndex: 'action', width: 80 },
      ]"
      :data-source="decks"
      row-key="code"
      :pagination="false"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'preview'">
          <Image :src="record.preview" :width="48" />
        </template>
        <template v-else-if="column.dataIndex === 'enabled'">
          <Switch :checked="record.enabled" disabled />
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button v-if="canWrite" type="link" @click="onEditStyle(record)">编辑</Button>
        </template>
      </template>
    </Table>
    <h3 class="mt-6 mb-3 text-base font-medium">牌组商品</h3>
    <Table
      :columns="[
        { title: '编码', dataIndex: 'code', width: 180 },
        { title: '类型', dataIndex: 'kind', width: 120 },
        { title: '名称', dataIndex: 'name', width: 160 },
        { title: '价格（分）', dataIndex: 'price_amount_minor', width: 120 },
        { title: '说明', dataIndex: 'note' },
        { title: '操作', dataIndex: 'action', width: 80 },
      ]"
      :data-source="products"
      row-key="code"
      :pagination="false"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'kind'">
          {{ kindLabel[record.kind] || record.kind }}
        </template>
        <template v-else-if="column.dataIndex === 'price_amount_minor'">
          {{ record.price_amount_minor }}（¥{{ yuan(record.price_amount_minor) }}）
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button v-if="canWrite" type="link" @click="onEditProduct(record as DeckProduct)">编辑</Button>
        </template>
      </template>
    </Table>
    <Modal
      :open="editing !== null"
      title="编辑牌组商品"
      ok-text="保存"
      @cancel="editing = null"
      @ok="saveProduct"
    >
      <Form layout="vertical">
        <Form.Item label="名称">
          <Input v-model:value="productForm.name" />
        </Form.Item>
        <Form.Item label="价格（分）">
          <InputNumber v-model:value="productForm.price_amount_minor" :min="0" class="w-full" />
        </Form.Item>
        <Form.Item label="说明">
          <Input.TextArea v-model:value="productForm.note" :rows="3" />
        </Form.Item>
        <Form.Item label="启用">
          <Switch v-model:checked="productForm.enabled" />
        </Form.Item>
      </Form>
    </Modal>
  </Page>
</template>
