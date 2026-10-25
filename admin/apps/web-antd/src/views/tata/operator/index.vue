<script lang="ts" setup>
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Table, Tag } from 'ant-design-vue';

import { listOperators } from '#/api/tata';

const rows = ref<Record<string, unknown>[]>([]);
onMounted(async () => {
  rows.value = await listOperators();
});
</script>

<template>
  <Page description="后台账号，不走 App 的微信或邮箱。" title="操作员">
    <Table
      :columns="[
        { title: '用户名', dataIndex: 'username' },
        { title: '角色', dataIndex: 'role' },
        { title: '状态', dataIndex: 'disabled' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="false"
    >
      <template #bodyCell="{ column, record }">
        <Tag v-if="column.dataIndex === 'disabled'" :color="record.disabled ? 'red' : 'green'">
          {{ record.disabled ? '停用' : '正常' }}
        </Tag>
      </template>
    </Table>
  </Page>
</template>
