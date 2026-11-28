import { useAccessStore } from '@vben/stores';

import { useAppConfig } from '@vben/hooks';

function resolveApiURL() {
  const { apiURL } = useAppConfig(import.meta.env, import.meta.env.PROD);
  if (import.meta.env.DEV) {
    return apiURL;
  }
  const override =
    typeof window === 'undefined' ? '' : (window as any).__TATA_ADMIN_API_URL__;
  if (typeof override === 'string' && override.trim()) {
    return override.trim().replace(/\/$/, '');
  }
  return apiURL;
}

/** 下载管理端 CSV 导出（绕过 JSON 拦截器）。path 形如 /users/export?... */
export async function downloadAdminCSV(path: string, filename: string) {
  const accessStore = useAccessStore();
  const base = resolveApiURL().replace(/\/$/, '');
  const url = `${base}${path.startsWith('/') ? path : `/${path}`}`;
  const res = await fetch(url, {
    headers: {
      Authorization: accessStore.accessToken ? `Bearer ${accessStore.accessToken}` : '',
    },
  });
  if (!res.ok) {
    throw new Error(`导出失败（${res.status}）`);
  }
  const blob = await res.blob();
  const href = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = href;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(href);
}
