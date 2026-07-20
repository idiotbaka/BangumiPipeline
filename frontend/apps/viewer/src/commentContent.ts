export type CommentContentNode =
  | { type: 'text'; value: string }
  | { type: 'strike'; children: CommentContentNode[] }
  | { type: 'mask'; children: CommentContentNode[] }
  | { type: 'image'; url: string; width?: number; height?: number }
  | { type: 'smile'; code: string; url: string }

interface ContentFrame {
  type: 'root' | 'strike' | 'mask'
  nodes: CommentContentNode[]
}

const contentTokenPattern = /\((?:bgm[0-9]{2,3}|musume_[0-9]{2,3}|blake_[0-9]{2,3})\)|\[\/?[a-z][a-z0-9_]*(?:=[^\]\r\n]*)?\]/gi
const maxRenderableCommentLength = 50_000
const maxExternalImageURLLength = 2_048
const maxExternalImageWidth = 360
const maxExternalImageHeight = 280
const imageOpenTagPattern = /^\[img(?:=\s*(\d+)\s*,\s*(\d+)\s*)?\]$/i

export function parseCommentContent(content: string, smiles: Record<string, string>): CommentContentNode[] {
  const source = String(content ?? '').slice(0, maxRenderableCommentLength)
  const lowerSource = source.toLocaleLowerCase()
  const frames: ContentFrame[] = [{ type: 'root', nodes: [] }]
  let cursor = 0
  contentTokenPattern.lastIndex = 0

  for (let match = contentTokenPattern.exec(source); match; match = contentTokenPattern.exec(source)) {
    appendText(currentFrame(frames).nodes, source.slice(cursor, match.index))
    const token = match[0]
    const normalized = token.toLocaleLowerCase()
    cursor = contentTokenPattern.lastIndex

    if (token.startsWith('(')) {
      const smileURL = smiles[token]
      if (smileURL) currentFrame(frames).nodes.push({ type: 'smile', code: token, url: smileURL })
      continue
    }
    const imageTag = imageOpenTagPattern.exec(token)
    if (imageTag) {
      const closingIndex = lowerSource.indexOf('[/img]', cursor)
      if (closingIndex < 0) continue
      const imageURL = safeExternalImageURL(source.slice(cursor, closingIndex))
      if (imageURL) {
        const size = fitExternalImageSize(imageTag[1], imageTag[2])
        currentFrame(frames).nodes.push({ type: 'image', url: imageURL, ...size })
      }
      cursor = closingIndex + '[/img]'.length
      contentTokenPattern.lastIndex = cursor
      continue
    }
    if (normalized === '[s]') {
      frames.push({ type: 'strike', nodes: [] })
      continue
    }
    if (normalized === '[mask]') {
      frames.push({ type: 'mask', nodes: [] })
      continue
    }
    if (normalized === '[/s]') {
      closeFrame(frames, 'strike')
      continue
    }
    if (normalized === '[/mask]') {
      closeFrame(frames, 'mask')
      continue
    }
    // Unknown BBCode tags are deliberately removed while their text remains.
  }
  appendText(currentFrame(frames).nodes, source.slice(cursor))
  while (frames.length > 1) {
    const unclosed = frames.pop()
    if (unclosed) currentFrame(frames).nodes.push(...unclosed.nodes)
  }
  return frames[0]?.nodes ?? []
}

function fitExternalImageSize(widthValue?: string, heightValue?: string) {
  if (!widthValue || !heightValue) return {}
  const width = Number.parseInt(widthValue, 10)
  const height = Number.parseInt(heightValue, 10)
  if (!Number.isSafeInteger(width) || !Number.isSafeInteger(height) || width <= 0 || height <= 0) return {}
  const scale = Math.min(1, maxExternalImageWidth / width, maxExternalImageHeight / height)
  return {
    width: Math.max(1, Math.round(width * scale)),
    height: Math.max(1, Math.round(height * scale)),
  }
}

function currentFrame(frames: ContentFrame[]) {
  return frames[frames.length - 1] as ContentFrame
}

function closeFrame(frames: ContentFrame[], type: 'strike' | 'mask') {
  const frame = currentFrame(frames)
  if (frames.length === 1 || frame.type !== type) return
  frames.pop()
  if (!frame.nodes.length) return
  currentFrame(frames).nodes.push({ type, children: frame.nodes })
}

function appendText(nodes: CommentContentNode[], value: string) {
  if (!value) return
  const previous = nodes[nodes.length - 1]
  if (previous?.type === 'text') {
    previous.value += value
    return
  }
  nodes.push({ type: 'text', value })
}

function safeExternalImageURL(value: string) {
  const candidate = value.trim()
  if (!candidate || candidate.length > maxExternalImageURLLength || /\s/.test(candidate)) return ''
  try {
    const parsed = new URL(candidate)
    if ((parsed.protocol !== 'https:' && parsed.protocol !== 'http:') || !parsed.hostname) return ''
    if (parsed.username || parsed.password) return ''
    return parsed.href
  } catch {
    return ''
  }
}
