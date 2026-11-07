import type { UploadFile } from 'ant-design-vue';

import { requestClient } from '#/api/request';

interface UploadFileParams {
  file: File;
  folder?: string;
  onError?: (error: Error) => void;
  onProgress?: (progress: { percent: number }) => void;
  onSuccess?: (data: any, file: File) => void;
}

export async function upload_file({
  file,
  folder,
  onError,
  onProgress,
  onSuccess,
}: UploadFileParams) {
  try {
    onProgress?.({ percent: 0 });

    const query = folder ? `?folder=${encodeURIComponent(folder)}` : '';
    const data = await requestClient.upload(`/uploads${query}`, { file });

    onProgress?.({ percent: 100 });
    onSuccess?.(data, file);
  } catch (error) {
    onError?.(error instanceof Error ? error : new Error(String(error)));
  }
}

export function imageUploadProps(folder: string) {
  return {
    accept: '.png,.jpg,.jpeg,.webp,.gif',
    customRequest: (params: UploadFileParams) => upload_file({ ...params, folder }),
    listType: 'picture-card' as const,
    maxCount: 1,
    maxSize: 5,
    multiple: false,
    showUploadList: true,
  };
}

export function fileListFromURL(url?: null | string): UploadFile[] {
  const value = url?.trim();
  if (!value) {
    return [];
  }
  return [
    {
      name: value.split('/').pop() || 'image',
      status: 'done',
      uid: '-1',
      url: value,
    },
  ];
}

export function fileListToURL(list: unknown): string {
  const files = (Array.isArray(list) ? list : []) as UploadFile[];
  const file = files.find((item) => item.status === 'done') ?? files.at(-1);
  const response = file?.response as { url?: string } | undefined;
  return String(response?.url || file?.url || '');
}
