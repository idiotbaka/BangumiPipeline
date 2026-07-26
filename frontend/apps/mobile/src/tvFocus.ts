import { isTVApp } from './platform'

export type TVFocusDirection = 'up' | 'down' | 'left' | 'right'

const focusableSelector = [
  'button:not(:disabled)',
  'input:not(:disabled)',
  'select:not(:disabled)',
  'textarea:not(:disabled)',
  'a[href]',
  '[role="button"]:not([aria-disabled="true"])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

let mutationObserver: MutationObserver | null = null
let modalFocusOrigin: HTMLElement | null = null
let observedModal: HTMLElement | null = null

export function installTVFocusNavigation() {
  if (!isTVApp) {
    return () => undefined
  }

  document.addEventListener('keydown', handleDocumentKeydown, true)
  window.addEventListener('bp-tv-key', handleNativeTVKey as EventListener)
  mutationObserver = new MutationObserver(handleDOMMutation)
  mutationObserver.observe(document.body, { childList: true, subtree: true })
  window.requestAnimationFrame(() => focusTVInitial())

  return () => {
    document.removeEventListener('keydown', handleDocumentKeydown, true)
    window.removeEventListener('bp-tv-key', handleNativeTVKey as EventListener)
    mutationObserver?.disconnect()
    mutationObserver = null
    observedModal = null
    modalFocusOrigin = null
  }
}

export function focusTVInitial(root: ParentNode = activeFocusRoot()) {
  if (!isTVApp) return false
  const focusables = visibleFocusables(root)
  const preferred = focusables.find((element) => element.dataset.tvAutofocus === 'true')
  const target = preferred ?? focusables[0]
  if (!target) return false
  focusElement(target)
  return true
}

export function moveTVFocus(direction: TVFocusDirection, root: ParentNode = activeFocusRoot()) {
  if (!isTVApp) return false
  const candidates = visibleFocusables(root)
  if (candidates.length === 0) return false

  const active = document.activeElement instanceof HTMLElement ? document.activeElement : null
  if (!active || !candidates.includes(active)) {
    return focusTVInitial(root)
  }

  const currentRect = active.getBoundingClientRect()
  const currentCenter = rectCenter(currentRect)
  let best: { element: HTMLElement; score: number } | null = null

  for (const candidate of candidates) {
    if (candidate === active) continue
    const rect = candidate.getBoundingClientRect()
    const center = rectCenter(rect)
    const primaryDelta =
      direction === 'left'
        ? currentCenter.x - center.x
        : direction === 'right'
          ? center.x - currentCenter.x
          : direction === 'up'
            ? currentCenter.y - center.y
            : center.y - currentCenter.y
    if (primaryDelta <= 2) continue

    const secondaryDelta =
      direction === 'left' || direction === 'right'
        ? Math.abs(center.y - currentCenter.y)
        : Math.abs(center.x - currentCenter.x)
    const overlaps =
      direction === 'left' || direction === 'right'
        ? rangesOverlap(currentRect.top, currentRect.bottom, rect.top, rect.bottom)
        : rangesOverlap(currentRect.left, currentRect.right, rect.left, rect.right)
    const score = primaryDelta + secondaryDelta * (overlaps ? 0.22 : 1.8) + (overlaps ? 0 : 240)
    if (!best || score < best.score) {
      best = { element: candidate, score }
    }
  }

  if (!best) return false
  focusElement(best.element)
  return true
}

function handleNativeTVKey(event: CustomEvent<{ key?: string }>) {
  if (event.detail?.key !== 'select') return
  const active = document.activeElement instanceof HTMLElement ? document.activeElement : null
  if (!active) {
    focusTVInitial()
    return
  }
  if (active.matches('button, a[href], [role="button"], input')) {
    active.click()
  }
}

function handleDocumentKeydown(event: KeyboardEvent) {
  if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey) return
  if (event.target instanceof Element && event.target.closest('[data-tv-key-scope="player"]')) return

  const direction = keyDirection(event.key)
  if (!direction) return

  const target = event.target
  if (
    target instanceof HTMLInputElement &&
    (direction === 'left' || direction === 'right') &&
    ['text', 'search', 'url', 'password'].includes(target.type)
  ) {
    return
  }

  if (moveTVFocus(direction)) {
    event.preventDefault()
    event.stopPropagation()
  }
}

function handleDOMMutation() {
  const modal = document.querySelector<HTMLElement>('[aria-modal="true"]')
  if (modal && modal !== observedModal) {
    modalFocusOrigin = document.activeElement instanceof HTMLElement ? document.activeElement : null
    observedModal = modal
    window.requestAnimationFrame(() => focusTVInitial(modal))
    return
  }
  if (!modal && observedModal) {
    observedModal = null
    const origin = modalFocusOrigin
    modalFocusOrigin = null
    if (origin?.isConnected) {
      window.requestAnimationFrame(() => focusElement(origin))
      return
    }
  }

  const active = document.activeElement
  if (!(active instanceof HTMLElement) || active === document.body || !active.isConnected) {
    window.requestAnimationFrame(() => focusTVInitial())
  }
}

function activeFocusRoot(): ParentNode {
  return document.querySelector<HTMLElement>('[aria-modal="true"]') ?? document
}

function visibleFocusables(root: ParentNode) {
  return Array.from(root.querySelectorAll<HTMLElement>(focusableSelector)).filter((element) => {
    if (element.closest('[aria-hidden="true"], [inert]')) return false
    const style = window.getComputedStyle(element)
    if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') return false
    const rect = element.getBoundingClientRect()
    return rect.width > 1 && rect.height > 1
  })
}

function focusElement(element: HTMLElement) {
  element.focus({ preventScroll: true })
  element.scrollIntoView({ behavior: 'smooth', block: 'center', inline: 'center' })
}

function keyDirection(key: string): TVFocusDirection | null {
  if (key === 'ArrowUp') return 'up'
  if (key === 'ArrowDown') return 'down'
  if (key === 'ArrowLeft') return 'left'
  if (key === 'ArrowRight') return 'right'
  return null
}

function rectCenter(rect: DOMRect) {
  return {
    x: rect.left + rect.width / 2,
    y: rect.top + rect.height / 2,
  }
}

function rangesOverlap(aStart: number, aEnd: number, bStart: number, bEnd: number) {
  return Math.max(aStart, bStart) <= Math.min(aEnd, bEnd)
}
