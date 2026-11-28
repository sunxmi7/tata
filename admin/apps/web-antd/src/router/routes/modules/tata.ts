import type { RouteRecordRaw } from 'vue-router';

function can(...mods: string[]) {
  return ['admin', ...mods];
}

const routes: RouteRecordRaw[] = [
  {
    meta: {
      icon: 'lucide:sparkles',
      order: -10,
      title: '塔塔运营',
      authority: can('content', 'ai', 'records', 'accounts', 'pay'),
    },
    name: 'Tata',
    path: '/tata',
    children: [
      {
        meta: { icon: 'lucide:library', title: '内容', authority: can('content') },
        name: 'TataContent',
        path: '/tata/content',
        redirect: '/tata/spread',
        children: [
          {
            name: 'TataSpread',
            path: '/tata/spread',
            component: () => import('#/views/tata/spread/index.vue'),
            meta: { icon: 'lucide:layers', title: '牌阵', authority: can('content') },
          },
          {
            name: 'TataCard',
            path: '/tata/cards',
            component: () => import('#/views/tata/cards/index.vue'),
            meta: { icon: 'lucide:layout-grid', title: '牌目录', authority: can('content') },
          },
          {
            name: 'TataDeck',
            path: '/tata/deck',
            component: () => import('#/views/tata/deck/index.vue'),
            meta: { icon: 'lucide:layers-2', title: '牌组', authority: can('content') },
          },
          {
            name: 'TataTarotist',
            path: '/tata/tarotist',
            component: () => import('#/views/tata/tarotist/index.vue'),
            meta: { icon: 'lucide:user-round', title: '塔罗师', authority: can('content') },
          },
          {
            name: 'TataCopy',
            path: '/tata/copy',
            component: () => import('#/views/tata/copy/index.vue'),
            meta: { icon: 'lucide:file-text', title: '配置文案', authority: can('content') },
          },
          {
            name: 'TataBanner',
            path: '/tata/banner',
            component: () => import('#/views/tata/banner/index.vue'),
            meta: { icon: 'lucide:megaphone', title: '首页提示窗', authority: can('content') },
          },
        ],
      },
      {
        meta: { icon: 'lucide:bot', title: '模型', authority: can('ai') },
        name: 'TataAI',
        path: '/tata/ai',
        redirect: '/tata/vendor',
        children: [
          {
            name: 'TataVendor',
            path: '/tata/vendor',
            component: () => import('#/views/tata/vendor/index.vue'),
            meta: { icon: 'lucide:cpu', title: '模型供应商', authority: can('ai') },
          },
          {
            name: 'TataWorkflow',
            path: '/tata/workflow',
            component: () => import('#/views/tata/workflow/index.vue'),
            meta: { icon: 'lucide:workflow', title: '调用测试', authority: can('ai') },
          },
          {
            name: 'TataQuota',
            path: '/tata/quota',
            component: () => import('#/views/tata/quota/index.vue'),
            meta: { icon: 'lucide:calendar-clock', title: '额度参数', authority: can('ai') },
          },
        ],
      },
      {
        meta: { icon: 'lucide:scroll-text', title: '记录', authority: can('records') },
        name: 'TataRecords',
        path: '/tata/records',
        redirect: '/tata/reading',
        children: [
          {
            name: 'TataReading',
            path: '/tata/reading',
            component: () => import('#/views/tata/reading/index.vue'),
            meta: { icon: 'lucide:scroll-text', title: '占卜记录', authority: can('records') },
          },
          {
            name: 'TataCalendar',
            path: '/tata/calendar',
            component: () => import('#/views/tata/calendar/index.vue'),
            meta: { icon: 'lucide:calendar', title: '日历', authority: can('records') },
          },
          {
            name: 'TataMessages',
            path: '/tata/messages',
            component: () => import('#/views/tata/message/index.vue'),
            meta: { icon: 'lucide:messages-square', title: '追问消息', authority: can('records') },
          },
          {
            name: 'TataWishlists',
            path: '/tata/wishlists',
            component: () => import('#/views/tata/wishlist/index.vue'),
            meta: { icon: 'lucide:heart', title: '功能心愿单', authority: can('records') },
          },
          {
            name: 'TataFeedback',
            path: '/tata/feedback',
            component: () => import('#/views/tata/feedback/index.vue'),
            meta: { icon: 'lucide:message-circle-warning', title: '反馈意见', authority: can('records') },
          },
          {
            name: 'TataLedger',
            path: '/tata/ledger',
            component: () => import('#/views/tata/ledger/index.vue'),
            meta: { icon: 'lucide:receipt', title: '额度流水', authority: can('records') },
          },
        ],
      },
      {
        meta: { icon: 'lucide:users', title: '账号', authority: can('accounts') },
        name: 'TataAccounts',
        path: '/tata/accounts',
        redirect: '/tata/users',
        children: [
          {
            name: 'TataUsers',
            path: '/tata/users',
            component: () => import('#/views/tata/user/index.vue'),
            meta: { icon: 'lucide:users', title: '全部用户', authority: can('accounts') },
          },
          {
            name: 'TataOnboarding',
            path: '/tata/onboarding',
            component: () => import('#/views/tata/onboarding/index.vue'),
            meta: { icon: 'lucide:smartphone', title: '新人引导', authority: can('accounts') },
          },
          {
            name: 'TataUserStats',
            path: '/tata/user-stats',
            component: () => import('#/views/tata/user-stats/index.vue'),
            meta: { icon: 'lucide:pie-chart', title: '用户概览', authority: can('accounts') },
          },
          {
            name: 'TataOperator',
            path: '/tata/operator',
            component: () => import('#/views/tata/operator/index.vue'),
            meta: { icon: 'lucide:shield', title: '操作员', authority: ['admin'] },
          },
          {
            name: 'TataAudit',
            path: '/tata/audit',
            component: () => import('#/views/tata/audit/index.vue'),
            meta: { icon: 'lucide:scroll', title: '审计日志', authority: can('accounts') },
          },
        ],
      },
      {
        meta: { icon: 'lucide:wallet', title: '付费', authority: can('pay') },
        name: 'TataPay',
        path: '/tata/pay',
        redirect: '/tata/pay-overview',
        children: [
          {
            name: 'TataPayOverview',
            path: '/tata/pay-overview',
            component: () => import('#/views/tata/pay-overview/index.vue'),
            meta: { icon: 'lucide:chart-column', title: '付费概览', authority: can('pay') },
          },
          {
            name: 'TataIAPSkus',
            path: '/tata/iap-skus',
            component: () => import('#/views/tata/iap-sku/index.vue'),
            meta: { icon: 'lucide:tag', title: '价位', authority: can('pay') },
          },
          {
            name: 'TataIAPOrders',
            path: '/tata/iap-orders',
            component: () => import('#/views/tata/iap-order/index.vue'),
            meta: { icon: 'lucide:receipt-text', title: 'IAP 订单', authority: can('pay') },
          },
          {
            name: 'TataGrant',
            path: '/tata/grant',
            component: () => import('#/views/tata/grant/index.vue'),
            meta: { icon: 'lucide:badge-plus', title: '人工补发', authority: can('pay') },
          },
          {
            name: 'TataMembership',
            path: '/tata/membership',
            component: () => import('#/views/tata/membership/index.vue'),
            meta: { icon: 'lucide:badge-check', title: '会员', authority: can('pay') },
          },
          {
            name: 'TataRedeemCodes',
            path: '/tata/redeem-codes',
            component: () => import('#/views/tata/redeem/index.vue'),
            meta: { icon: 'lucide:ticket', title: '兑换码', authority: can('pay') },
          },
          {
            name: 'TataInvites',
            path: '/tata/invites',
            component: () => import('#/views/tata/invite/index.vue'),
            meta: { icon: 'lucide:users', title: '邀请关系', authority: can('pay') },
          },
          {
            name: 'TataRegistrationGifts',
            path: '/tata/registration-gifts',
            component: () => import('#/views/tata/registration-gift/index.vue'),
            meta: { icon: 'lucide:gift', title: '注册赠送', authority: can('pay') },
          },
          {
            name: 'TataQuotaLots',
            path: '/tata/quota-lots',
            component: () => import('#/views/tata/quota-lot/index.vue'),
            meta: { icon: 'lucide:layers', title: '额度批次', authority: can('pay') },
          },
          {
            name: 'TataYearlyQualifications',
            path: '/tata/yearly-qualifications',
            component: () => import('#/views/tata/yearly-qualification/index.vue'),
            meta: { icon: 'lucide:calendar-range', title: '年运资格', authority: can('pay') },
          },
        ],
      },
    ],
  },
];

export default routes;
