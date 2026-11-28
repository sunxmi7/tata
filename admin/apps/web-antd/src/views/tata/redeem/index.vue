<script lang="ts" setup>
import type { IAPSku, RedeemCode } from '#/api/tata';

import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Button,
  Form,
  Input,
  InputNumber,
  Modal,
  Radio,
  Select,
  Switch,
  Table,
  message,
} from 'ant-design-vue';

import {
  createRedeemCodes,
  getCopy,
  listIAPSkus,
  listRedeemCodes,
  updateRedeemCode,
} from '#/api/tata';

import { hasTataWrite } from '../access';

const redeemSkuKinds = new Set([
  'month_subscription',
  'year_subscription',
  'quota_pack',
  'yearly_unlock',
  'deck_year',
  'deck_bundle',
  'deck_single',
  'deck_addon',
]);

const kindLabel: Record<string, string> = {
  quota: '裸额度',
  free_count: '历史权益',
  month_subscription: '会员 · 月卡',
  year_subscription: '会员 · 年卡',
  quota_pack: '额度补充包',
  yearly_unlock: '年运解锁',
  deck_year: '牌组 · 年度畅享',
  deck_bundle: '牌组 · 打包',
  deck_single: '牌组 · 单套',
  deck_addon: '牌组 · 后续加购',
};

const rows = ref<RedeemCode[]>([]);
const skus = ref<IAPSku[]>([]);
const deckOptions = ref<{ label: string; value: string }[]>([]);
const note = ref('');
const open = ref(false);
const editing = ref<RedeemCode | null>(null);
const form = ref(emptyForm());
const canWrite = computed(() => hasTataWrite('pay'));

const groupLabel: Record<string, string> = {
  month_subscription: '会员',
  year_subscription: '会员',
  quota_pack: '额度包',
  yearly_unlock: '年运',
  deck_year: '牌组',
  deck_bundle: '牌组',
  deck_single: '牌组',
  deck_addon: '牌组',
};

const skuOptions = computed(() => {
  const buckets = new Map<string, { label: string; value: string }[]>();
  const groupOrder = ['会员', '额度包', '年运', '牌组'];
  for (const sku of skus.value) {
    if (!sku.enabled || !redeemSkuKinds.has(sku.kind)) continue;
    const group = groupLabel[sku.kind] || sku.kind;
    const grant =
      sku.kind === 'quota_pack' && sku.quota_grant != null
        ? ` · ${sku.quota_grant} 额度`
        : '';
    const list = buckets.get(group) || [];
    list.push({
      label: `${sku.display_name || sku.code}（${sku.code}${grant}）`,
      value: sku.code,
    });
    buckets.set(group, list);
  }
  return groupOrder
    .filter((label) => (buckets.get(label) || []).length > 0)
    .map((label) => ({ label, options: buckets.get(label)! }));
});

const selectedSku = computed(() =>
  skus.value.find((s) => s.code === form.value.sku_code),
);

const needAddonDeck = computed(
  () => !editing.value && form.value.mode === 'sku' && selectedSku.value?.kind === 'deck_addon',
);

function emptyForm() {
  return {
    mode: 'sku' as 'sku' | 'quota',
    sku_code: undefined as string | undefined,
    addon_deck_code: undefined as string | undefined,
    kind: 'quota',
    amount: null as number | null,
    benefit_name: '',
    code: '',
    count: 1,
    max_redemptions: 1 as number | null,
    expires_at: '',
    enabled: true,
  };
}

function kindText(kind: string) {
  return kindLabel[kind] || kind || '—';
}

async function load() {
  const data = await listRedeemCodes();
  rows.value = data.items || [];
  note.value = data.permission_note || '';
}

async function loadCatalog() {
  const [skuList, decks] = await Promise.all([
    listIAPSkus(),
    getCopy('decks').catch(() => ({ items: [] as unknown[] })),
  ]);
  skus.value = skuList || [];
  deckOptions.value = ((decks.items as { code?: string; name?: string }[]) || [])
    .filter((d) => d.code)
    .map((d) => ({
      label: `${d.name || d.code}（${d.code}）`,
      value: String(d.code),
    }));
}

function showCreate() {
  editing.value = null;
  form.value = emptyForm();
  open.value = true;
}

function showEdit(row: RedeemCode) {
  editing.value = row;
  form.value = {
    mode: row.sku_code ? 'sku' : 'quota',
    sku_code: row.sku_code || undefined,
    addon_deck_code: row.addon_deck_code || undefined,
    kind: String(row.kind || 'quota'),
    amount: row.amount,
    benefit_name: String(row.benefit_name || ''),
    code: String(row.code || ''),
    count: 1,
    max_redemptions: row.max_redemptions,
    expires_at: String(row.expires_at || ''),
    enabled: row.enabled !== false,
  };
  open.value = true;
}

async function copyCode(code: string) {
  await navigator.clipboard.writeText(code);
  message.success('已复制');
}

async function save() {
  if (editing.value) {
    await updateRedeemCode(Number(editing.value.id), {
      enabled: form.value.enabled,
      benefit_name: form.value.benefit_name,
      max_redemptions: form.value.max_redemptions ?? 0,
      expires_at: form.value.expires_at.trim(),
    });
    message.success('已保存');
  } else {
    const body: Parameters<typeof createRedeemCodes>[0] = {
      count: form.value.count || 1,
      enabled: form.value.enabled,
    };
    if (form.value.mode === 'sku') {
      if (!form.value.sku_code) {
        message.error('请选择商品');
        return;
      }
      body.sku_code = form.value.sku_code;
      if (needAddonDeck.value) {
        const addon = form.value.addon_deck_code?.trim();
        if (!addon) {
          message.error('牌组加点请填写 addon_deck_code');
          return;
        }
        body.addon_deck_code = addon;
      }
      const benefit = form.value.benefit_name.trim();
      if (benefit) {
        body.benefit_name = benefit;
      }
    } else {
      if (!form.value.amount || form.value.amount <= 0) {
        message.error('请填写额度数量');
        return;
      }
      if (!form.value.benefit_name.trim()) {
        message.error('请填写权益名称');
        return;
      }
      body.kind = 'quota';
      body.amount = form.value.amount;
      body.benefit_name = form.value.benefit_name.trim();
    }
    const custom = form.value.code.trim();
    if (custom) {
      body.code = custom;
    }
    if (form.value.max_redemptions) {
      body.max_redemptions = form.value.max_redemptions;
    }
    const expires = form.value.expires_at.trim();
    if (expires) {
      body.expires_at = expires;
    }
    const created = await createRedeemCodes(body);
    const codes = (created.items || []).map((item) => item.code).join('、');
    message.success(codes ? `已生成：${codes}` : '已生成');
  }
  open.value = false;
  await load();
}

onMounted(async () => {
  await Promise.all([load(), loadCatalog()]);
});
</script>

<template>
  <Page
    description="优先选 IAP 商品（月/年卡、额度包、年运、牌组）生成兑换码，核销履约与付费一致；亦可发运营裸额度。空码则自动生成 XXXX-XXXX。不填核销上限表示不限人数。付费模块可写才能生成或停用。"
    title="兑换码"
  >
    <template #extra>
      <Button v-if="canWrite" type="primary" @click="showCreate">生成兑换码</Button>
    </template>
    <p v-if="note" class="mb-4 text-sm opacity-70">{{ note }}</p>
    <Table
      :columns="[
        { title: '兑换码', dataIndex: 'code' },
        { title: '类型', dataIndex: 'kind', width: 140 },
        { title: 'SKU', dataIndex: 'sku_code', width: 140 },
        { title: '数量', dataIndex: 'amount', width: 80 },
        { title: '权益名', dataIndex: 'benefit_name' },
        { title: '已核销 / 上限', dataIndex: 'usage', width: 140 },
        { title: '过期', dataIndex: 'expires_at' },
        { title: '启用', dataIndex: 'enabled', width: 80 },
        { title: '创建时间', dataIndex: 'created_at' },
        { title: '操作', dataIndex: 'action', width: 140 },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{ pageSize: 20 }"
      :locale="{ emptyText: '暂无兑换码' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'kind'">
          {{ kindText(String(record.kind || '')) }}
        </template>
        <template v-else-if="column.dataIndex === 'sku_code'">
          <span v-if="record.sku_code">
            {{ record.sku_code }}
            <template v-if="record.addon_deck_code">
              · {{ record.addon_deck_code }}
            </template>
          </span>
          <span v-else>—</span>
        </template>
        <template v-else-if="column.dataIndex === 'usage'">
          {{ record.redeemed_count }}
          /
          {{ record.max_redemptions == null ? '不限' : record.max_redemptions }}
        </template>
        <template v-else-if="column.dataIndex === 'expires_at'">
          {{ record.expires_at || '—' }}
        </template>
        <template v-else-if="column.dataIndex === 'enabled'">
          {{ record.enabled ? '是' : '否' }}
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <Button type="link" @click="copyCode(String(record.code))">复制</Button>
          <Button v-if="canWrite" type="link" @click="showEdit(record as RedeemCode)">编辑</Button>
        </template>
      </template>
    </Table>
    <Modal
      v-model:open="open"
      :title="editing ? '编辑兑换码' : '生成兑换码'"
      @ok="save"
    >
      <Form layout="vertical">
        <template v-if="!editing">
          <Form.Item label="发放方式">
            <Radio.Group v-model:value="form.mode">
              <Radio.Button value="sku">IAP 商品</Radio.Button>
              <Radio.Button value="quota">裸额度</Radio.Button>
            </Radio.Group>
          </Form.Item>
          <Form.Item v-if="form.mode === 'sku'" label="商品（sku_code）" required>
            <Select
              v-model:value="form.sku_code"
              allow-clear
              show-search
              option-filter-prop="label"
              placeholder="月/年卡、额度包、年运、牌组"
              :options="skuOptions"
              style="width: 100%"
            />
          </Form.Item>
          <Form.Item
            v-if="needAddonDeck"
            label="加点牌组（addon_deck_code）"
            required
          >
            <Select
              v-if="deckOptions.length"
              v-model:value="form.addon_deck_code"
              allow-clear
              show-search
              option-filter-prop="label"
              placeholder="选择要解锁的牌组 code"
              :options="deckOptions"
              style="width: 100%"
            />
            <Input
              v-else
              v-model:value="form.addon_deck_code"
              placeholder="例如 star-realm"
            />
          </Form.Item>
          <Form.Item v-if="form.mode === 'quota'" label="额度数量" required>
            <InputNumber
              v-model:value="form.amount"
              :min="1"
              style="width: 100%"
            />
          </Form.Item>
        </template>
        <template v-else>
          <Form.Item label="类型">
            <Input :value="kindText(form.kind)" disabled />
          </Form.Item>
          <Form.Item v-if="form.sku_code" label="SKU">
            <Input :value="form.sku_code" disabled />
          </Form.Item>
          <Form.Item v-if="form.addon_deck_code" label="加点牌组">
            <Input :value="form.addon_deck_code" disabled />
          </Form.Item>
          <Form.Item v-if="!form.sku_code" label="数量">
            <InputNumber v-model:value="form.amount" disabled style="width: 100%" />
          </Form.Item>
        </template>
        <Form.Item
          :label="form.mode === 'quota' && !editing ? '权益名称（必填）' : '权益名称（可空，SKU 默认用商品名）'"
          :required="form.mode === 'quota' && !editing"
        >
          <Input v-model:value="form.benefit_name" placeholder="例如 新春礼" />
        </Form.Item>
        <Form.Item v-if="!editing" label="自定义码（可空，空则自动生成）">
          <Input v-model:value="form.code" placeholder="仅 count=1 时可填" />
        </Form.Item>
        <Form.Item v-if="!editing" label="生成个数">
          <InputNumber v-model:value="form.count" :min="1" :max="100" style="width: 100%" />
        </Form.Item>
        <Form.Item label="核销上限（空或 0 表示不限）">
          <InputNumber v-model:value="form.max_redemptions" :min="0" style="width: 100%" />
        </Form.Item>
        <Form.Item label="过期时间 RFC3339（可空）">
          <Input v-model:value="form.expires_at" placeholder="2026-12-31T16:00:00Z" />
        </Form.Item>
        <Form.Item label="启用">
          <Switch v-model:checked="form.enabled" />
        </Form.Item>
      </Form>
    </Modal>
  </Page>
</template>
