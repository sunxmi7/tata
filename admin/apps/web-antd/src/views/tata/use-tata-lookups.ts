import type { AppUser, Operator, ReadingRow } from '#/api/tata';

import { ref, shallowRef } from 'vue';

import { useAccessStore } from '@vben/stores';

import { listOperators, listReadings, listUsers } from '#/api/tata';

function asNumber(value: unknown): number | undefined {
  if (value === null || value === undefined || value === '') return undefined;
  const n = Number(value);
  return Number.isFinite(n) ? n : undefined;
}

export function formatUserRecord(user: AppUser): string {
  const hint =
    (user.email && String(user.email).trim()) ||
    (user.wechat_openid && `微信 …${String(user.wechat_openid).slice(-6)}`) ||
    (user.apple_sub && `Apple …${String(user.apple_sub).slice(-6)}`);
  return hint ? `${hint} (#${user.id})` : `用户 #${user.id}`;
}

export function useTataLookups() {
  const userLabels = shallowRef(new Map<number, string>());
  const operatorLabels = shallowRef(new Map<number, string>());
  const readingLabels = shallowRef(new Map<number, string>());
  const ready = ref(false);

  async function loadLookups() {
    const codes = new Set(useAccessStore().accessCodes);
    const can = (module: string) => codes.has('write') || codes.has(module);
    const [users, operators, readings] = await Promise.all([
      can('accounts') ? listUsers() : Promise.resolve([] as AppUser[]),
      can('accounts') ? listOperators() : Promise.resolve([] as Operator[]),
      can('records') ? listReadings() : Promise.resolve([] as ReadingRow[]),
    ]);
    userLabels.value = new Map(users.map((user) => [user.id, formatUserRecord(user)]));
    operatorLabels.value = new Map(
      operators.map((op: Operator) => [op.id, `${op.username} (#${op.id})`]),
    );
    readingLabels.value = new Map(
      readings.map((row: ReadingRow) => {
        const question = String(row.question ?? '').trim();
        const short = question.length > 24 ? `${question.slice(0, 24)}…` : question;
        return [row.id, short ? `#${row.id} · ${short}` : `#${row.id}`];
      }),
    );
    ready.value = true;
  }

  function userLabel(id: unknown) {
    const n = asNumber(id);
    if (n === undefined) return '—';
    return userLabels.value.get(n) ?? `用户 #${n}`;
  }

  function operatorLabel(id: unknown) {
    const n = asNumber(id);
    if (n === undefined) return '—';
    return operatorLabels.value.get(n) ?? `操作员 #${n}`;
  }

  function readingLabel(id: unknown) {
    const n = asNumber(id);
    if (n === undefined) return '—';
    return readingLabels.value.get(n) ?? `#${n}`;
  }

  return {
    loadLookups,
    operatorLabel,
    readingLabel,
    ready,
    userLabel,
  };
}
