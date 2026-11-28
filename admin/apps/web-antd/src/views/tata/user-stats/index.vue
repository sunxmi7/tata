<script lang="ts" setup>
import type { EchartsUIType } from '@vben/plugins/echarts';
import type { UserStatGroup } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { EchartsUI, useEcharts } from '@vben/plugins/echarts';

import { Card } from 'ant-design-vue';

import { getUserStats } from '#/api/tata';

const total = ref(0);
const notes = ref<string[]>([]);
const genderRef = ref<EchartsUIType>();
const ageRef = ref<EchartsUIType>();
const exploreRef = ref<EchartsUIType>();
const regionRef = ref<EchartsUIType>();
const dailyRef = ref<EchartsUIType>();
const { renderEcharts: renderGender } = useEcharts(genderRef);
const { renderEcharts: renderAge } = useEcharts(ageRef);
const { renderEcharts: renderExplore } = useEcharts(exploreRef);
const { renderEcharts: renderRegion } = useEcharts(regionRef);
const { renderEcharts: renderDaily } = useEcharts(dailyRef);

const colors = [
  '#5ab1ef',
  '#b6a2de',
  '#67e0e3',
  '#2ec7c9',
  '#ffb980',
  '#d87a80',
  '#8d98b3',
  '#97b552',
  '#95706d',
  '#dc69aa',
];

function pieOption(title: string, group: UserStatGroup | undefined) {
  const items = group?.items ?? [];
  return {
    legend: {
      bottom: 0,
      left: 'center',
      type: 'scroll' as const,
    },
    series: [
      {
        avoidLabelOverlap: true,
        color: colors,
        data: items.map((item) => ({ name: `${item.label} ${item.percent}%`, value: item.count })),
        emphasis: {
          label: { fontWeight: 'bold' as const, show: true },
        },
        itemStyle: { borderColor: '#fff', borderRadius: 8, borderWidth: 2 },
        label: { formatter: '{b}\n{c}', show: true },
        name: title,
        radius: ['36%', '62%'],
        type: 'pie' as const,
      },
    ],
    title: { left: 'center', text: title, textStyle: { fontSize: 14 } },
    tooltip: {
      formatter: '{b}<br/>人数/次数：{c}',
      trigger: 'item' as const,
    },
  };
}

onMounted(async () => {
  const stats = await getUserStats(30);
  total.value = stats.total_users ?? 0;
  notes.value = stats.notes || [];
  await Promise.all([
    renderGender(pieOption('性别', stats.gender)),
    renderAge(pieOption('年龄', stats.age)),
    renderExplore(pieOption('探索偏好', stats.explore)),
    renderRegion(pieOption('注册地区', stats.region)),
    renderDaily({
      grid: { left: 40, right: 20, top: 40, bottom: 40 },
      series: [
        {
          data: (stats.daily_new || []).map((d) => d.count),
          name: '日新增',
          type: 'line' as const,
          smooth: true,
        },
      ],
      title: { left: 'center', text: '近 30 日新增', textStyle: { fontSize: 14 } },
      tooltip: { trigger: 'axis' as const },
      xAxis: {
        data: (stats.daily_new || []).map((d) => d.date.slice(5)),
        type: 'category' as const,
      },
      yAxis: { minInterval: 1, type: 'value' as const },
    }),
  ]);
});
</script>

<template>
  <Page
    description="自建：日新增、画像、注册地区。日活/留存/渠道见友盟。"
    :title="`用户概览（${total} 人）`"
  >
    <Card class="mb-4" title="日新增趋势">
      <EchartsUI ref="dailyRef" height="300px" />
    </Card>
    <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
      <Card title="性别占比">
        <EchartsUI ref="genderRef" height="320px" />
      </Card>
      <Card title="年龄占比">
        <EchartsUI ref="ageRef" height="320px" />
      </Card>
      <Card title="探索类型占比">
        <EchartsUI ref="exploreRef" height="320px" />
      </Card>
      <Card title="注册地区">
        <EchartsUI ref="regionRef" height="320px" />
      </Card>
    </div>
    <p v-if="notes.length" class="mt-4 text-sm text-gray-500">
      口径：{{ notes.join('；') }}
    </p>
  </Page>
</template>
