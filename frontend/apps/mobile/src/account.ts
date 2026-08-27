import type { ViewerUser } from './api'

export function formatAccountDate(value: number | null) {
  if (!value || !Number.isFinite(value)) return '未知'
  const date = new Date(value * 1000)
  if (!Number.isFinite(date.getTime())) return '未知'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false,
  }).format(date)
}

export function registrationMethod(user: ViewerUser) {
  if (user.registrationSource === 'system_invite') return '邀请码注册（系统管理员邀请）'
  if (user.registrationSource === 'user_invite') return `邀请码注册（用户 ${user.invitedByUsername || '未知用户'} 邀请）`
  return '开放注册'
}
