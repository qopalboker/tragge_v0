// Tiny in-house navigation progress bar.
//
// nprogress is unusable here because the admin :8081 CSP enforces
// `require-trusted-types-for 'script'` (matching production), and
// nprogress writes innerHTML internally without a Trusted Types
// policy. This implementation uses pure DOM APIs — createElement +
// classList + style mutations — so every assignment is allowed under
// strict TT.
//
// No innerHTML, outerHTML, insertAdjacentHTML, or document.write.

let bar: HTMLDivElement | null = null
let trickleId: ReturnType<typeof setInterval> | null = null
let hideTimeoutId: ReturnType<typeof setTimeout> | null = null
let percent = 0

function ensureBar(): HTMLDivElement {
  if (bar) return bar
  const el = document.createElement('div')
  el.className = 'nav-progress'
  document.body.appendChild(el)
  bar = el
  return el
}

function setPercent(p: number): void {
  const el = ensureBar()
  percent = p
  el.style.transform = `translateX(${-100 + p}%)`
  el.style.opacity = p > 0 && p < 100 ? '1' : '0'
}

export const navProgress = {
  start(): void {
    if (hideTimeoutId) {
      clearTimeout(hideTimeoutId)
      hideTimeoutId = null
    }
    const el = ensureBar()
    el.style.transition = 'none'
    setPercent(0)
    // Force reflow so the next transition runs from 0
    void el.offsetWidth
    el.style.transition = 'transform 200ms ease-out, opacity 200ms ease'
    setPercent(15)
    if (trickleId) clearInterval(trickleId)
    trickleId = setInterval(() => {
      const remaining = 90 - percent
      if (remaining > 0) setPercent(percent + remaining * 0.1)
    }, 200)
  },
  done(): void {
    if (trickleId) {
      clearInterval(trickleId)
      trickleId = null
    }
    setPercent(100)
    hideTimeoutId = setTimeout(() => {
      setPercent(0)
      hideTimeoutId = null
    }, 250)
  },
}
