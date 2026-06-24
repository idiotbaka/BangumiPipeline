import { reactive } from 'vue'
import type { User } from './api'

export const session = reactive<{ user: User | null }>({ user: null })
