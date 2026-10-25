<script lang="ts" setup>
import type { Tarotist } from '#/api/tata';

import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import { Button, Form, Input, Modal, Switch, Table, Tag, message } from 'ant-design-vue';

import { createTarotist, listTarotists, updateTarotist } from '#/api/tata';

const access = useAccessStore();
const rows = ref<Tarotist[]>([]);
const open = ref(false);
const editing = ref<null | Tarotist>(null);
const form = ref({ name: '', persona: '', display_copy: '', voice: '', banned: '', ask_style: '', enabled: true });
const columns = [
  { title: '名称', dataIndex: 'name' },
  { title: '展示文案', dataIndex: 'display_copy' },
  { title: '启用', dataIndex: 'enabled' },
  { title: '操作', dataIndex: 'action' },
];

async function load() {
  rows.value = await listTarotists();
}
function canWrite() {
  return access.accessCodes.includes('write');
}
function showCreate() {
  editing.value = null;
  form.value = { name: '', persona: '', display_copy: '', voice: '', banned: '', ask_style: '', enabled: true };
  open.value = true;
}
function showEdit(row: Tarotist) {
  editing.value = row;
  form.value = { ...row };
  open.value = true;
}
async function save() {
  if (editing.value) {
    await updateTarotist(editing.value.id, form.value);
  } else {
    await createTarotist(form.value);
  }
  message.success('已保存');
  open.value = false;
  await load();
}
onMounted(load);
</script>

<template>
  <Page description="人设只影响之后开始的占卜。" title="塔罗师">
    <template #extra>
      <Button v-if="canWrite()" type="primary" @click="showCreate">新建</Button>
    </template>
    <Table :columns="columns" :data-source="rows" row-key="id" :pagination="false">
      <template #bodyCell="{ column, record }">
        <Tag v-if="column.dataIndex === 'enabled'" :color="record.enabled ? 'green' : 'default'">
          {{ record.enabled ? '启用' : '停用' }}
        </Tag>
        <Button v-else-if="column.dataIndex === 'action' && canWrite()" type="link" @click="showEdit(record)">
          编辑
        </Button>
      </template>
    </Table>
    <Modal v-model:open="open" title="塔罗师" @ok="save">
      <Form layout="vertical">
        <Form.Item label="名称"><Input v-model:value="form.name" /></Form.Item>
        <Form.Item label="人设"><Input.TextArea v-model:value="form.persona" :rows="5" /></Form.Item>
        <Form.Item label="展示文案"><Input v-model:value="form.display_copy" /></Form.Item>
        <Form.Item label="语音"><Input v-model:value="form.voice" /></Form.Item>
        <Form.Item label="禁止表达"><Input v-model:value="form.banned" /></Form.Item>
        <Form.Item label="追问方式"><Input v-model:value="form.ask_style" /></Form.Item>
        <Form.Item label="启用"><Switch v-model:checked="form.enabled" /></Form.Item>
      </Form>
    </Modal>
  </Page>
</template>
