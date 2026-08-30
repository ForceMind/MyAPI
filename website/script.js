const header = document.querySelector('[data-header]')
const menuToggle = document.querySelector('[data-menu-toggle]')
const mobileMenu = document.querySelector('[data-mobile-menu]')
const themeToggle = document.querySelector('[data-theme-toggle]')

window.addEventListener(
  'scroll',
  () => header?.classList.toggle('scrolled', window.scrollY > 18),
  { passive: true }
)

function setMenu(open) {
  mobileMenu?.classList.toggle('open', open)
  menuToggle?.setAttribute('aria-expanded', String(open))
}

menuToggle?.addEventListener('click', () => {
  setMenu(!mobileMenu?.classList.contains('open'))
})

mobileMenu?.querySelectorAll('a').forEach((link) =>
  link.addEventListener('click', () => setMenu(false))
)

document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape') setMenu(false)
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
