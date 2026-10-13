export const scheduleTabs = [
  { value: 1, label: '周一', kana: 'MON' },
  { value: 2, label: '周二', kana: 'TUE' },
  { value: 3, label: '周三', kana: 'WED' },
  { value: 4, label: '周四', kana: 'THU' },
  { value: 5, label: '周五', kana: 'FRI' },
  { value: 6, label: '周六', kana: 'SAT' },
  { value: 7, label: '周日', kana: 'SUN' },
  { value: 8, label: '开播未定', kana: 'TBA' },
  { value: 9, label: '电影/剧场版', kana: 'MOVIE' },
] as const

interface ScheduleAnime {
  animeType: 'series' | 'movie'
  airWeekday: number
}

export function scheduleTabValue(item: ScheduleAnime): number {
  if (item.animeType === 'movie') return 9
  return item.airWeekday >= 1 && item.airWeekday <= 7 ? item.airWeekday : 8
}
