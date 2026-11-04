import { ref, shallowRef } from 'vue';

import { listOperators, listReadings, listUsers } from '#/api/tata';

function asNumber(value: unknown): number | undefined {
  if (value === null || value === undefined || value === '') return undefined;
  const n = Number(value);
  return Number.isFinite(n) ? n : undefined;
}

export function formatUserRecord(user: Record<string, unknown>): string {
  const id = user.id as number;
  const email = user.email as string | null | undefined;
  const phone = user.phone as string | null | undefined;
  const wechat = user.wechat_openid as string | null | undefined;
  const apple = user.apple_sub as string | null | undefined;
  const hint =
    (email && String(email).trim()) ||
    (phone && String(phone).trim()) ||
    (wechat && `微信 …${String(wechat).slice(-6)}`) ||
    (apple && `Apple …${String(apple).slice(-6)}`);
  return hint ? `${hint} (#${id})` : `用户 #${id}`;
}

export function useTataLookups() {
  const userLabels = shallowRef(new Map<number, string>());
  const operatorLabels = shallowRef(new Map<number, string>());
  const readingLabels = shallowRef(new Map<number, string>());
  const ready = ref(false);

  async function loadLookups() {
    const [users, operators, readings] = await Promise.all([
      listUsers(),
      listOperators(),
      listReadings(),
    ]);
    userLabels.value = new Map(
      users.map((user) => [user.id as number, formatUserRecord(user)]),
    );
    operatorLabels.value = new Map(
      operators.map((op) => {
        const id = op.id as number;
        const name = String(op.username ?? id);
        return [id, `${name} (#${id})`];
      }),
    );
    readingLabels.value = new Map(
      readings.map((row) => {
        const id = row.id as number;
        const question = String(row.question ?? '').trim();
        const short =
          question.length > 24 ? `${question.slice(0, 24)}…` : question;
        return [id, short ? `#${id} · ${short}` : `#${id}`];
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
