const header = document.querySelector('[data-header]')
const menuToggle = document.querySelector('[data-menu-toggle]')
const mobileMenu = document.querySelector('[data-mobile-menu]')
const themeToggle = document.querySelector('[data-theme-toggle]')

// Keep the LAN edition CTA useful even when the page is mirrored without a
// build step; the guide contains the safe, pinned-image setup path.
document.querySelector('.lan-edition .button-light')?.setAttribute(
  'href',
  'https://github.com/ForceMind/MyAPI/blob/main/docs/LAN_LITE.md',
)

window.addEventListener(
  'scroll',
  () => header?.classList.toggle('scrolled', window.scrollY > 18),
  { passive: true }
)

function setMenu(open) {
  const wasOpen = mobileMenu?.classList.contains('open') === true
  mobileMenu?.classList.toggle('open', open)
  menuToggle?.setAttribute('aria-expanded', String(open))
  if (!open && wasOpen && document.activeElement instanceof HTMLElement) {
    menuToggle?.focus({ preventScroll: true })
  }
}

menuToggle?.addEventListener('click', () => {
  setMenu(!mobileMenu?.classList.contains('open'))
})

mobileMenu?.querySelectorAll('a').forEach((link) =>
  link.addEventListener('click', () => setMenu(false))
)

document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape') setMenu(false)
  if (event.key !== 'Tab' || !mobileMenu?.classList.contains('open')) return
  const focusable = [menuToggle, ...mobileMenu.querySelectorAll('a')].filter(
    (element) => element instanceof HTMLElement && !element.hasAttribute('disabled'),
  )
  if (focusable.length === 0) return
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
})

document.addEventListener('pointerdown', (event) => {
  if (mobileMenu?.classList.contains('open') && header && !header.contains(event.target)) {
    setMenu(false)
  }
})

function readSavedTheme() {
  try {
    return window.localStorage.getItem('myapi-site-theme')
  } catch {
    // Private browsing modes may deny storage; the site still works without it.
    return null
  }
}

function saveTheme(theme) {
  try {
    window.localStorage.setItem('myapi-site-theme', theme)
  } catch {
    // Persisting a preference is optional and must never break navigation.
  }
}

function applyTheme(light) {
  document.documentElement.classList.toggle('light-preview', light)
  themeToggle?.setAttribute('aria-pressed', String(light))
  themeToggle?.setAttribute('aria-label', light ? 'Use dark theme' : 'Use light theme')
}

const savedTheme = readSavedTheme()
const useLightTheme = savedTheme
  ? savedTheme === 'light'
  : window.matchMedia?.('(prefers-color-scheme: light)').matches === true
applyTheme(useLightTheme)

themeToggle?.addEventListener('click', () => {
  const light = !document.documentElement.classList.contains('light-preview')
  applyTheme(light)
  saveTheme(light ? 'light' : 'dark')
})
