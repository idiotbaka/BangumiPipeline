import { tauriGlobal } from './tauri'

export interface ImageViewerSource {
  src: string
  downloadUrl: string
  headers?: Record<string, string>
  suggestedName: string
  alt: string
}

export interface PreparedImage {
  cacheKey: string
  byteSize: number
  mimeType: string
  suggestedName: string
}

export interface SavedImage {
  displayName: string
  path: string
}

interface PrepareImageInput extends Record<string, unknown> {
  url: string
  headers: Record<string, string>
  suggestedName: string
}

const browserImages = new Map<string, Blob>()

export async function prepareImage(source: ImageViewerSource): Promise<PreparedImage> {
  const input: PrepareImageInput = {
    url: source.downloadUrl,
    headers: source.headers ?? {},
    suggestedName: source.suggestedName,
  }
  const core = tauriGlobal()?.core
  if (core) {
    return core.invoke<PreparedImage>('plugin:image-saver|prepareImage', input)
  }

  const response = await fetch(input.url, { headers: input.headers, credentials: 'omit' })
  if (!response.ok) throw new Error(`读取原图失败（${response.status}）`)
  const blob = await response.blob()
  if (!blob.size) throw new Error('原图内容为空')
  const cacheKey = `browser-${crypto.randomUUID()}`
  browserImages.set(cacheKey, blob)
  return {
    cacheKey,
    byteSize: blob.size,
    mimeType: blob.type || 'application/octet-stream',
    suggestedName: input.suggestedName,
  }
}

export async function savePreparedImage(image: PreparedImage): Promise<SavedImage> {
  const core = tauriGlobal()?.core
  if (core) {
    return core.invoke<SavedImage>('plugin:image-saver|saveImage', {
      cacheKey: image.cacheKey,
      suggestedName: image.suggestedName,
    })
  }

  const blob = browserImages.get(image.cacheKey)
  if (!blob) throw new Error('原图缓存已失效，请重新打开图片')
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = image.suggestedName || 'bakavip2-image'
  anchor.click()
  window.setTimeout(() => URL.revokeObjectURL(url), 1_000)
  return { displayName: anchor.download, path: '浏览器默认下载文件夹' }
}

export async function discardPreparedImage(image: PreparedImage) {
  const core = tauriGlobal()?.core
  if (core) {
    await core.invoke('plugin:image-saver|discardImage', { cacheKey: image.cacheKey })
    return
  }
  browserImages.delete(image.cacheKey)
}
