import type { OrderType, OrderStatus } from "@/features/credit-reference/lib/services"
/* 类型标签配置 */
export const typeConfig: Record<OrderType, { label: string; color: string }> = {
  receive: { label: '积分收益', color: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300' },
  payment: { label: '积分消耗', color: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-300' },
  community: { label: '社区划转', color: 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-300' },
  online: { label: '在线活动', color: 'bg-teal-100 text-teal-800 dark:bg-teal-900 dark:text-teal-300' },
  distribute: { label: '商户分发', color: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-300' },
  test: { label: '应用测试', color: 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-300 font-bold' },
  red_envelope_send: { label: '红包支出', color: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-300' },
  red_envelope_receive: { label: '红包收入', color: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300' },
  red_envelope_refund: { label: '红包退款', color: 'bg-muted/50 text-gray-800 dark:bg-gray-900 dark:text-gray-300' }
}

/* 状态标签配置 */
export const statusConfig: Record<OrderStatus, { label: string; color: string }> = {
  success: { label: '成功', color: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300' },
  pending: { label: '处理中', color: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-300' },
  failed: { label: '失败', color: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-300' },
  expired: { label: '已过期', color: 'bg-muted/50 text-gray-800 dark:bg-gray-900 dark:text-gray-300' },
  disputing: { label: '争议中', color: 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-300' },
  refund: { label: '已退回', color: 'bg-muted/50 text-gray-800 dark:bg-gray-900 dark:text-gray-300' },
  refused: { label: '已拒绝', color: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-300' }
}

