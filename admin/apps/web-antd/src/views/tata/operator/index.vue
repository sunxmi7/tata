<script lang="ts" setup>
import type { Operator, OperatorModule } from '#/api/tata';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useUserStore } from '@vben/stores';

import { Button, Form, Input, Modal, Radio, Switch, Table, Tag, message } from 'ant-design-vue';

import { createOperator, listOperators, updateOperator } from '#/api/tata';

import { TATA_MODULES } from '../access';

type ModuleLevel = 'none' | 'read' | 'write';

const userStore = useUserStore();
const isAdmin = computed(() => (userStore.userRoles ?? []).includes('admin'));
const rows = ref<Operator[]>([]);
const open = ref(false);
const editing = ref<null | Operator>(null);
const saving = ref(false);
const form = reactive({
  username: '',
  password: '',
  role: 'readonly' as 'admin' | 'readonly',
  disabled: false,
  levels: emptyLevels(),
});

function emptyLevels(): Record<string, ModuleLevel> {
  return Object.fromEntries(TATA_MODULES.map((item) => [item.id, 'none']));
}

function levelsFrom(modules: OperatorModule[] | undefined) {
  const next = emptyLevels();
  for (const item of modules ?? []) {
    next[item.module] = item.can_write ? 'write' : 'read';
  }
  return next;
}

function modulesFromLevels(): OperatorModule[] {
  return TATA_MODULES.flatMap((item) => {
    const level = form.levels[item.id];
    if (level === 'read') return [{ module: item.id, can_write: false }];
    if (level === 'write') return [{ module: item.id, can_write: true }];
    return [];
  });
}

function roleLabel(role: string) {
  return role === 'admin' ? '管理员' : '运营';
}

function moduleText(record: Operator) {
  if (record.role === 'admin') return '全部可写';
  const mods = record.modules ?? [];
  if (!mods.length) return '无';
  const byId = new Map(mods.map((item) => [item.module, item]));
  return TATA_MODULES.flatMap((item) => {
    const found = byId.get(item.id);
    if (!found) return [];
    return [`${item.label}${found.can_write ? '·写' : '·读'}`];
  }).join('、');
}

async function load() {
  rows.value = await listOperators();
}

function showCreate() {
  editing.value = null;
  form.username = '';
  form.password = '';
  form.role = 'readonly';
  form.disabled = false;
  form.levels = emptyLevels();
  open.value = true;
}

function showEdit(record: Operator) {
  editing.value = record;
  form.username = record.username;
  form.password = '';
  form.role = record.role === 'admin' ? 'admin' : 'readonly';
  form.disabled = record.disabled;
  form.levels = levelsFrom(record.modules);
  open.value = true;
}

async function save() {
  saving.value = true;
  try {
    if (editing.value) {
      await updateOperator(editing.value.id, {
        disabled: form.disabled,
        modules: form.role === 'admin' ? undefined : modulesFromLevels(),
      });
    } else {
      if (!form.username.trim() || !form.password) {
        message.error('用户名和密码必填');
        return;
      }
      await createOperator({
        username: form.username.trim(),
        password: form.password,
        role: form.role,
        modules: form.role === 'admin' ? [] : modulesFromLevels(),
      });
    }
    message.success('已保存');
    open.value = false;
    await load();
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <Page description="只有管理员能新建账号、停用账号，并按内容 / 模型 / 记录 / 账号 / 付费授权。管理员默认全部可写。" title="操作员">
    <template #extra>
      <Button v-if="isAdmin" type="primary" @click="showCreate">新建</Button>
    </template>
    <Table
      :columns="[
        { title: '用户名', dataIndex: 'username' },
        { title: '角色', dataIndex: 'role' },
        { title: '模块', dataIndex: 'modules' },
        { title: '状态', dataIndex: 'disabled' },
        { title: '操作', dataIndex: 'action' },
      ]"
      :data-source="rows"
      row-key="id"
      :pagination="false"
    >
      <template #bodyCell="{ column, record }">
        <span v-if="column.dataIndex === 'role'">{{ roleLabel(record.role) }}</span>
        <span v-else-if="column.dataIndex === 'modules'">{{ moduleText(record) }}</span>
        <Tag v-else-if="column.dataIndex === 'disabled'" :color="record.disabled ? 'red' : 'green'">
          {{ record.disabled ? '停用' : '正常' }}
        </Tag>
        <Button v-else-if="column.dataIndex === 'action' && isAdmin" type="link" @click="showEdit(record)">
          编辑
        </Button>
      </template>
    </Table>
    <Modal
      v-model:open="open"
      :confirm-loading="saving"
      :title="editing ? '编辑操作员' : '新建操作员'"
      @ok="save"
    >
      <Form layout="vertical">
        <Form.Item label="用户名">
          <Input v-model:value="form.username" :disabled="!!editing" />
        </Form.Item>
        <Form.Item v-if="!editing" label="密码">
          <Input.Password v-model:value="form.password" />
        </Form.Item>
        <Form.Item v-if="!editing" label="角色">
          <Radio.Group v-model:value="form.role">
            <Radio value="admin">管理员（全部模块）</Radio>
            <Radio value="readonly">运营（按模块授权）</Radio>
          </Radio.Group>
        </Form.Item>
        <Form.Item v-if="editing" label="停用">
          <Switch v-model:checked="form.disabled" />
        </Form.Item>
        <template v-if="form.role !== 'admin'">
          <Form.Item v-for="mod in TATA_MODULES" :key="mod.id" :label="mod.label">
            <Radio.Group v-model:value="form.levels[mod.id]">
              <Radio value="none">无权限</Radio>
              <Radio value="read">只读</Radio>
              <Radio value="write">可写</Radio>
            </Radio.Group>
          </Form.Item>
        </template>
        <p v-else class="opacity-70">管理员不按模块限制，可进入全部页面并给别人授权。</p>
      </Form>
    </Modal>
  </Page>
</template>
