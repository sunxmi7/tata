import { useAccessStore } from '@vben/stores';

export type TataModule = 'content' | 'ai' | 'records' | 'accounts' | 'pay';

export const TATA_MODULES: { id: TataModule; label: string }[] = [
  { id: 'content', label: '内容' },
  { id: 'ai', label: '模型' },
  { id: 'records', label: '记录' },
  { id: 'accounts', label: '账号' },
  { id: 'pay', label: '付费' },
];

export function hasTataWrite(module: TataModule) {
  const codes = useAccessStore().accessCodes;
  return codes.includes('write') || codes.includes(`${module}:write`);
}

export function hasTataModule(module: TataModule) {
  const codes = useAccessStore().accessCodes;
  return codes.includes('write') || codes.includes(module);
}
