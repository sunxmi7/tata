<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import { Button, Form, Input, InputNumber, Table, Tabs, message } from 'ant-design-vue';

import { getCopy, updateCopy } from '#/api/tata';

const access = useAccessStore();
const canWrite = access.accessCodes.includes('write');
const year = ref(new Date().getFullYear());
const yearly = ref<Record<string, unknown>>({});
const cards = ref<Record<string, unknown>[]>([]);
const help = ref<Record<string, unknown>[]>([]);
const campaigns = ref<Record<string, unknown>[]>([]);
const saving = ref(false);

async function loadYearly() {
  yearly.value = await getCopy('yearly', year.value);
}

async function loadCards() {
  const data = await getCopy('cards');
  cards.value = (data.items as Record<string, unknown>[]) || [];
}

async function loadHelp() {
  const data = await getCopy('help');
  help.value = (data.items as Record<string, unknown>[]) || [];
}

async function loadCampaigns() {
  const data = await getCopy('campaigns');
  campaigns.value = (data.items as Record<string, unknown>[]) || [];
}

async function saveYearly() {
  saving.value = true;
  try {
    const sections = ((yearly.value.sections as Record<string, unknown>[]) || []).map((row) => ({
      key: row.key,
      body: row.body ?? '',
    }));
    yearly.value = await updateCopy('yearly', { year: year.value, sections });
    message.success('已保存年运正文');
  } finally {
    saving.value = false;
  }
}

async function saveCard(row: Record<string, unknown>) {
  const data = await updateCopy('cards', {
    card_name: row.card_name,
    yi: row.yi ?? '',
  });
  cards.value = (data.items as Record<string, unknown>[]) || [];
  message.success('已保存');
}

async function saveHelp(row: Record<string, unknown>) {
  await updateCopy('help', { article_id: row.article_id, title: row.title, body: row.body });
  message.success('已保存');
}

async function saveCampaign(row: Record<string, unknown>) {
  await updateCopy('campaigns', {
    campaign_id: row.campaign_id,
    title: row.title,
    subtitle: row.subtitle,
    badge: row.badge,
    detail_title: row.detail_title,
    detail_body: row.detail_body,
    status: row.status,
    enabled: row.enabled,
  });
  message.success('已保存');
}

onMounted(async () => {
  await Promise.all([loadYearly(), loadCards(), loadHelp(), loadCampaigns()]);
});
</script>

<template>
  <Page
    description="年运、今日一牌「宜」、帮助、活动。78 张牌图和风格套装在侧栏「牌面」。会员专属活动文案在「价位」年卡。"
    title="运营文案"
  >
    <Tabs>
      <Tabs.TabPane key="yearly" tab="年运正文">
        <div class="mb-4 flex items-center gap-2">
          <InputNumber v-model:value="year" :min="2000" :max="2100" @change="loadYearly" />
          <Button v-if="canWrite" type="primary" :loading="saving" @click="saveYearly">保存</Button>
        </div>
        <Form layout="vertical">
          <Form.Item
            v-for="row in (yearly.sections as Record<string, unknown>[]) || []"
            :key="String(row.key)"
            :label="String(row.title)"
          >
            <Input.TextArea v-model:value="row.body" :rows="4" placeholder="空表示未定稿" />
          </Form.Item>
        </Form>
      </Tabs.TabPane>
      <Tabs.TabPane key="cards" tab="今日一牌宜">
        <Table
          :columns="[
            { title: '牌', dataIndex: 'card_name', width: 140 },
            { title: '宜', dataIndex: 'yi' },
            { title: '操作', dataIndex: 'action', width: 80 },
          ]"
          :data-source="cards"
          row-key="card_name"
          :pagination="false"
          :scroll="{ y: 480 }"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'yi'">
              <Input v-model:value="record.yi" placeholder="空则首页不展示" />
            </template>
            <template v-else-if="column.dataIndex === 'action'">
              <Button v-if="canWrite" type="link" @click="saveCard(record)">保存</Button>
            </template>
          </template>
        </Table>
      </Tabs.TabPane>
      <Tabs.TabPane key="help" tab="帮助">
        <Table
          :columns="[
            { title: '分类', dataIndex: 'category_id', width: 140 },
            { title: '标题', dataIndex: 'title' },
            { title: '正文', dataIndex: 'body' },
            { title: '操作', dataIndex: 'action', width: 80 },
          ]"
          :data-source="help"
          row-key="article_id"
          :pagination="false"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'title'">
              <Input v-model:value="record.title" />
            </template>
            <template v-else-if="column.dataIndex === 'body'">
              <Input.TextArea v-model:value="record.body" :rows="2" />
            </template>
            <template v-else-if="column.dataIndex === 'action'">
              <Button v-if="canWrite" type="link" @click="saveHelp(record)">保存</Button>
            </template>
          </template>
        </Table>
      </Tabs.TabPane>
      <Tabs.TabPane key="campaigns" tab="活动">
        <Table
          :columns="[
            { title: '编码', dataIndex: 'campaign_id', width: 160 },
            { title: '标题', dataIndex: 'title' },
            { title: '详情', dataIndex: 'detail_body' },
            { title: '操作', dataIndex: 'action', width: 80 },
          ]"
          :data-source="campaigns"
          row-key="campaign_id"
          :pagination="false"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'title'">
              <Input v-model:value="record.title" />
            </template>
            <template v-else-if="column.dataIndex === 'detail_body'">
              <Input.TextArea v-model:value="record.detail_body" :rows="2" />
            </template>
            <template v-else-if="column.dataIndex === 'action'">
              <Button v-if="canWrite" type="link" @click="saveCampaign(record)">保存</Button>
            </template>
          </template>
        </Table>
      </Tabs.TabPane>
    </Tabs>
  </Page>
</template>
