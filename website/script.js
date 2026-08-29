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

const savedTheme = window.localStorage.getItem('myapi-site-theme')
if (savedTheme === 'light') document.documentElement.classList.add('light-preview')

themeToggle?.addEventListener('click', () => {
  const light = document.documentElement.classList.toggle('light-preview')
  window.localStorage.setItem('myapi-site-theme', light ? 'light' : 'dark')
  themeToggle.setAttribute('aria-pressed', String(light))
})
