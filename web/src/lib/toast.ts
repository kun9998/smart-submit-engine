import { toast } from 'vue-sonner'

const TOAST_POSITION = 'top-right' as const
const ERROR_DURATION = 5000
const SUCCESS_DURATION = 3000

/** 右上角错误提示，数秒后自动消失 */
export function notifyError(message: string) {
  toast.error(message, { duration: ERROR_DURATION, position: TOAST_POSITION })
}

/** 右上角成功提示 */
export function notifySuccess(message: string) {
  toast.success(message, { duration: SUCCESS_DURATION, position: TOAST_POSITION })
}
