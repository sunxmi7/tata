<script lang="ts" setup>
import type { EchartsUIType } from '@vben/plugins/echarts';
import type { IAPRevenue } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { EchartsUI, useEcharts } from '@vben/plugins/echarts';

import { Card, Select, Table } from 'ant-design-vue';

import { getIAPRevenue } from '#/api/tata';

const grain = ref<'day' | 'week' | 'month'>('day');
const data = ref<IAPRevenue | null>(null);
const chartRef = ref<EchartsUIType>();
const { renderEcharts } = useEcharts(chartRef);

function yuan(minor?: number) {
  if (minor == null) return '—';
  return (minor / 100).toFixed(2);
}

async function load() {
  data.value = await getIAPRevenue({ grain: grain.value });
  const series = data.value.series || [];
  await renderEcharts({
    grid: { left: 48, right: 20, top: 40, bottom: 40 },
    legend: { data: ['收入(元)', '付费人数'] },
    series: [
      {
        data: series.map((s) => Number((s.revenue_minor / 100).toFixed(2))),
        name: '收入(元)',
        type: 'line' as const,
        smooth: true,
      },
      {
        data: series.map((s) => s.paying_users),
        name: '付费人数',
        type: 'bar' as const,
        yAxisIndex: 1,
      },
    ],
    tooltip: { trigger: 'axis' as const },
    xAxis: { data: series.map((s) => s.bucket), type: 'category' as const },
    yAxis: [
      { name: '元', type: 'value' as const },
      { name: '人', minInterval: 1, type: 'value' as const },
    ],
  });
}

onMounted(load);
</script>

<template>
  <Page description="验单成功订单为财务真相。友盟付费事件仅作对照。" title="付费概览">
    <div class="mb-4 flex flex-wrap gap-2">
      <Select
        v-model:value="grain"
        style="width: 140px"
        :options="[
          { value: 'day', label: '按日' },
          { value: 'week', label: '按周' },
          { value: 'month', label: '按月' },
        ]"
        @change="load"
      />
    </div>
    <div v-if="data" class="mb-4 grid grid-cols-2 gap-4 md:grid-cols-4">
      <Card>
        <div class="text-sm text-gray-500">收入（元）</div>
        <div class="text-2xl font-medium">{{ yuan(data.revenue_minor) }}</div>
      </Card>
      <Card>
        <div class="text-sm text-gray-500">付费人数</div>
        <div class="text-2xl font-medium">{{ data.paying_users }}</div>
      </Card>
      <Card>
        <div class="text-sm text-gray-500">同期新增</div>
        <div class="text-2xl font-medium">{{ data.new_users }}</div>
      </Card>
      <Card>
        <div class="text-sm text-gray-500">付费率</div>
        <div class="text-2xl font-medium">{{ data.pay_rate }}%</div>
      </Card>
    </div>
    <Card class="mb-4" title="趋势">
      <EchartsUI ref="chartRef" height="320px" />
    </Card>
    <Card title="按 SKU">
      <Table
        :columns="[
          { title: 'SKU', dataIndex: 'sku_code' },
          { title: '类型', dataIndex: 'sku_kind', width: 140 },
          { title: '订单数', dataIndex: 'orders', width: 100 },
          { title: '付费人数', dataIndex: 'paying_users', width: 100 },
          { title: '收入(元)', dataIndex: 'revenue_minor', width: 120 },
        ]"
        :data-source="data?.by_sku || []"
        :pagination="false"
        row-key="sku_code"
        :locale="{ emptyText: '暂无验单成功订单' }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'revenue_minor'">
            {{ yuan(record.revenue_minor) }}
          </template>
        </template>
      </Table>
    </Card>
    <p v-if="data?.notes?.length" class="mt-4 text-sm text-gray-500">
      口径：{{ data.notes.join('；') }}
    </p>
  </Page>
</template>
