<script lang="ts" setup>
import type { AuditRow } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Input, Space, Table, message } from 'ant-design-vue';

import { downloadAdminCSV, listAudit } from '#/api/tata';

import { useTataLookups } from '../use-tata-lookups';

const rows = ref<AuditRow[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const operatorId = ref('');
const action = ref('');
const objectType = ref('');
const { loadLookups, operatorLabel } = useTataLookups();

function queryParams() {
  return {
    operator_id: operatorId.value.trim() || undefined,
    action: action.value.trim() || undefined,
    object_type: objectType.value.trim() || undefined,
    limit: pageSize.value,
    offset: (page.value - 1) * pageSize.value,
  };
}

async function load() {
  await loadLookups();
  const data = await listAudit(queryParams());
  rows.value = data.items || [];
  total.value = data.total || 0;
}

async function exportCSV() {
  const sp = new URLSearchParams();
  const params = queryParams();
  Object.entries(params).forEach(([k, v]) => {
    if (v === undefined || v === null || v === '' || k === 'limit' || k === 'offset') return;
    sp.set(k, String(v));
  });
  const qs = sp.toString();
  await downloadAdminCSV(`/audit/export${qs ? `?${qs}` : ''}`, 'audit.csv');
  message.success('已导出');
}

onMounted(load);
</script>

<template>
  <Page description="后台写操作。支持按操作员/动作/对象筛选与导出。" title="审计日志">
    <div class="mb-4 flex flex-wrap gap-2">
      <Input v-model:value="operatorId" placeholder="操作员 ID" style="max-width: 140px" />
      <Input v-model:value="action" placeholder="动作" style="max-width: 140px" />
      <Input v-model:value="objectType" placeholder="对象类型" style="max-width: 160px" />
      <Space>
        <Button
          type="primary"
          @click="
            () => {
              page = 1;
              load();
            }
          "
        >
          查询
        </Button>
        <Button @click="exportCSV">导出 CSV</Button>
      </Space>
    </div>
    <Table
      :columns="[
        { title: '时间', dataIndex: 'created_at' },
        { title: '操作员', dataIndex: 'operator_id' },
        { title: '动作', dataIndex: 'action' },
        { title: '对象', dataIndex: 'object_type' },
        { title: '对象 ID', dataIndex: 'object_id' },
        { title: '说明', dataIndex: 'detail' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="{
        current: page,
        pageSize,
        total,
        showSizeChanger: true,
        onChange: (p: number, ps: number) => {
          page = p;
          pageSize = ps;
          load();
        },
      }"
      :locale="{ emptyText: '暂无审计记录' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'operator_id'">
          {{ operatorLabel(record.operator_id) }}
        </template>
      </template>
    </Table>
  </Page>
</template>
