<script lang="ts" setup>
import { computed, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Alert, Button, Card, Form, Input, Steps, Tag, message } from 'ant-design-vue';

interface CardFace {
  position_name: string;
  name: string;
  orientation: string;
}

interface StepView {
  key: string;
  title: string;
  text: string;
  vendor: string;
  model: string;
  error: string;
  cards: CardFace[];
}

const stages = [
  { key: 'detect', title: '问题检测', hint: '判断这个问题适不适合塔罗' },
  { key: 'rewrite', title: '问题改写', hint: '给出三条更清楚的问法' },
  { key: 'recommend', title: '牌阵推荐', hint: '按问题选牌阵' },
  { key: 'draw', title: '抽牌', hint: '固定三张牌和正逆位' },
  { key: 'interpret', title: '解读', hint: '按已固定牌面解读，不重新抽牌' },
];

const question = ref('接下来三个月工作怎么选');
const running = ref(false);
const cursor = ref(-1);
const done = ref<StepView[]>([]);
const heldCards = ref<CardFace[]>([]);

const current = computed(() => stages[cursor.value + 1]);
const finished = computed(() => cursor.value >= stages.length - 1);

function reset() {
  cursor.value = -1;
  done.value = [];
  heldCards.value = [];
}

const mockCards: CardFace[] = [
  { position_name: '过去', name: '力量', orientation: 'upright' },
  { position_name: '现在', name: '隐者', orientation: 'reversed' },
  { position_name: '未来', name: '星星', orientation: 'upright' },
];

function mockStep(key: string, questionText: string): StepView {
  if (key === 'detect') {
    return {
      key,
      title: '问题检测',
      text: `适合塔罗。问题「${questionText}」指向一个需要做选择的处境，不是闲聊，也不是危机求助。`,
      vendor: 'mock',
      model: 'mock-tarot',
      error: '',
      cards: [],
    };
  }
  if (key === 'rewrite') {
    return {
      key,
      title: '问题改写',
      text: '1. 接下来三个月，哪条工作路径更适合我现在的能力？\n2. 如果继续现在的工作，三个月后我会面对什么？\n3. 如果换一条路，我需要先补上哪一块？',
      vendor: 'mock',
      model: 'mock-tarot',
      error: '',
      cards: [],
    };
  }
  if (key === 'recommend') {
    return {
      key,
      title: '牌阵推荐',
      text: '用时间流。三张牌，牌位是过去、现在、未来。这个问题要看一段时间内的变化，不需要更复杂的牌阵。',
      vendor: 'mock',
      model: 'mock-tarot',
      error: '',
      cards: [],
    };
  }
  if (key === 'draw') {
    return {
      key,
      title: '抽牌',
      text: '过去：力量，正位\n现在：隐者，逆位\n未来：星星，正位',
      vendor: '',
      model: '',
      error: '',
      cards: mockCards,
    };
  }
  return {
    key,
    title: '解读',
    text: '过去的力量是正位：你已经有能力扛住压力，选择不是因为你不行。\n现在的隐者是逆位：你在反复想，但信息还不够，继续一个人闷着不会更清楚。\n未来的星星是正位：三个月内有一条能看见方向的路，但要先把现在卡住的那件事说出来。\n这组牌不重新抽取。重试解读时仍用这三张。',
    vendor: 'mock',
    model: 'mock-tarot',
    error: '',
    cards: [],
  };
}

async function next() {
  const stage = current.value;
  if (!stage || !question.value.trim()) {
    message.warning('先写问题');
    return;
  }
  running.value = true;
  try {
    await new Promise((resolve) => setTimeout(resolve, 280));
    const data = mockStep(stage.key, question.value.trim());
    if (stage.key === 'draw') {
      heldCards.value = data.cards;
    }
    done.value.push(data);
    cursor.value += 1;
  } finally {
    running.value = false;
  }
}
</script>

<template>
  <Page description="先用一组模拟结果走完五步。不调用供应商，不写占卜记录。" title="调用测试">
    <Card title="问题">
      <Form layout="vertical">
        <Form.Item label="用户问题">
          <Input.TextArea v-model:value="question" :disabled="cursor >= 0" :rows="3" />
        </Form.Item>
        <Button v-if="!finished" type="primary" :loading="running" @click="next">
          {{ cursor < 0 ? '开始' : `下一步：${current?.title}` }}
        </Button>
        <Button class="ml-2" :disabled="running" @click="reset">重来</Button>
      </Form>
    </Card>

    <Card class="mt-4" title="进度">
      <Steps :current="Math.max(cursor, 0)" size="small">
        <Steps.Step
          v-for="(stage, index) in stages"
          :key="stage.key"
          :description="done[index]?.error ? '失败' : stage.hint"
          :status="done[index]?.error ? 'error' : undefined"
          :title="stage.title"
        />
      </Steps>
    </Card>

    <Card v-for="(step, index) in done" :key="step.key" class="mt-4" :title="`${index + 1}. ${step.title}`">
      <Alert v-if="step.error" class="mb-3" type="error" show-icon message="这一步失败">
        <template #description>
          <pre class="m-0 whitespace-pre-wrap">{{ step.error }}</pre>
        </template>
      </Alert>
      <div v-if="step.cards.length" class="flex flex-wrap gap-2">
        <Tag v-for="card in step.cards" :key="card.position_name" color="purple">
          {{ card.position_name }} · {{ card.name }} · {{ card.orientation === 'reversed' ? '逆位' : '正位' }}
        </Tag>
      </div>
      <p v-else-if="step.text" class="whitespace-pre-wrap">{{ step.text }}</p>
      <p v-if="step.vendor" class="mt-2 text-xs opacity-60">{{ step.vendor }} / {{ step.model }}</p>
    </Card>
  </Page>
</template>
