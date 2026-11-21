<script lang="ts" setup>
import type { EchartsUIType } from '@vben/plugins/echarts';
import type { UserStatGroup } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { EchartsUI, useEcharts } from '@vben/plugins/echarts';

import { Card } from 'ant-design-vue';

import { getUserStats } from '#/api/tata';

const total = ref(0);
const genderRef = ref<EchartsUIType>();
const ageRef = ref<EchartsUIType>();
const exploreRef = ref<EchartsUIType>();
const { renderEcharts: renderGender } = useEcharts(genderRef);
const { renderEcharts: renderAge } = useEcharts(ageRef);
const { renderEcharts: renderExplore } = useEcharts(exploreRef);

const colors = ['#5ab1ef', '#b6a2de', '#67e0e3', '#2ec7c9', '#ffb980', '#d87a80', '#8d98b3', '#97b552', '#95706d', '#dc69aa'];

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
  const stats = await getUserStats();
  total.value = stats.total_users ?? 0;
  await Promise.all([
    renderGender(pieOption('性别', stats.gender)),
    renderAge(pieOption('年龄', stats.age)),
    renderExplore(pieOption('探索偏好', stats.explore)),
  ]);
});
</script>

<template>
  <Page
    description="未注销、未合并用户。性别与年龄按人数；探索偏好多选按选择次数，未选任何项计入「未选择」。"
    :title="`用户统计（${total} 人）`"
  >
    <div class="grid grid-cols-1 gap-4 xl:grid-cols-3">
      <Card title="性别占比">
        <EchartsUI ref="genderRef" height="360px" />
      </Card>
      <Card title="年龄占比">
        <EchartsUI ref="ageRef" height="360px" />
      </Card>
      <Card title="探索类型占比">
        <EchartsUI ref="exploreRef" height="360px" />
      </Card>
    </div>
  </Page>
</template>
