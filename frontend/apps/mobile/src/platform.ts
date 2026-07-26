export type AppTarget = 'mobile' | 'tv'

export const appTarget: AppTarget = import.meta.env.VITE_APP_TARGET === 'tv' ? 'tv' : 'mobile'
export const isTVApp = appTarget === 'tv'
