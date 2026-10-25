import type { RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  {
    meta: {
      icon: 'lucide:sparkles',
      order: -10,
      title: '塔塔运营',
    },
    name: 'Tata',
    path: '/tata',
    children: [
      {
        name: 'TataSpread',
        path: 'spread',
        component: () => import('#/views/tata/spread/index.vue'),
        meta: { icon: 'lucide:layers', title: '牌阵' },
      },
      {
        name: 'TataTarotist',
        path: 'tarotist',
        component: () => import('#/views/tata/tarotist/index.vue'),
        meta: { icon: 'lucide:user-round', title: '塔罗师' },
      },
      {
        name: 'TataVendor',
        path: 'vendor',
        component: () => import('#/views/tata/vendor/index.vue'),
        meta: { icon: 'lucide:bot', title: '模型供应商' },
      },
      {
        name: 'TataWorkflow',
        path: 'workflow',
        component: () => import('#/views/tata/workflow/index.vue'),
        meta: { icon: 'lucide:workflow', title: '调用测试' },
      },
      {
        name: 'TataQuota',
        path: 'quota',
        component: () => import('#/views/tata/quota/index.vue'),
        meta: { icon: 'lucide:calendar-clock', title: '额度参数' },
      },
      {
        name: 'TataBanner',
        path: 'banner',
        component: () => import('#/views/tata/banner/index.vue'),
        meta: { icon: 'lucide:megaphone', title: '首页提示窗' },
      },
      {
        name: 'TataReading',
        path: 'reading',
        component: () => import('#/views/tata/reading/index.vue'),
        meta: { icon: 'lucide:scroll-text', title: '占卜记录' },
      },
      {
        name: 'TataLedger',
        path: 'ledger',
        component: () => import('#/views/tata/ledger/index.vue'),
        meta: { icon: 'lucide:receipt', title: '额度流水' },
      },
      {
        name: 'TataOperator',
        path: 'operator',
        component: () => import('#/views/tata/operator/index.vue'),
        meta: { icon: 'lucide:shield', title: '操作员' },
      },
      {
        name: 'TataUsers',
        path: 'users',
        component: () => import('#/views/tata/user/index.vue'),
        meta: { icon: 'lucide:users', title: '用户账号' },
      },
      {
        name: 'TataCalendar',
        path: 'calendar',
        component: () => import('#/views/tata/calendar/index.vue'),
        meta: { icon: 'lucide:calendar', title: '日历' },
      },
      {
        name: 'TataMessages',
        path: 'messages',
        component: () => import('#/views/tata/message/index.vue'),
        meta: { icon: 'lucide:messages-square', title: '追问消息' },
      },
      {
        name: 'TataAudit',
        path: 'audit',
        component: () => import('#/views/tata/audit/index.vue'),
        meta: { icon: 'lucide:scroll', title: '审计日志' },
      },
      {
        name: 'TataIAPOrders',
        path: 'iap-orders',
        component: () => import('#/views/tata/iap-order/index.vue'),
        meta: { icon: 'lucide:receipt-text', title: 'IAP 订单' },
      },
    ],
  },
];

export default routes;
