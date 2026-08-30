/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your
option) any later version.
*/
import { createServer } from 'node:http'
import { createReadStream } from 'node:fs'
import { existsSync, mkdirSync } from 'node:fs'
import { extname, join, normalize, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = normalize(join(fileURLToPath(new URL('../../website/', import.meta.url))))
const contentTypes = {
  '.css': 'text/css; charset=utf-8',
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.png': 'image/png',
}

const server = createServer((request, response) => {
  let relative
  try {
    relative = decodeURIComponent((request.url ?? '/').split('?')[0])
  } catch {
    response.writeHead(400)
    response.end('bad request')
    return
  }
  const requested = resolve(root, `.${relative === '/' ? '/index.html' : relative}`)
  const insideRoot = requested === root || requested.startsWith(`${root}${sep}`)
  if (!insideRoot || !existsSync(requested)) {
    response.writeHead(404)
    response.end('not found')
    return
  }
  response.writeHead(200, { 'content-type': contentTypes[extname(requested)] ?? 'application/octet-stream' })
  createReadStream(requested).pipe(response)
})

const { chromium } = await import('playwright')
const browser = await chromium.launch({ headless: true })
try {
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('website smoke server did not bind')
  const page = await browser.newPage({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 1 })
  await page.goto(`http://127.0.0.1:${address.port}/`, { waitUntil: 'networkidle' })
  if ((await page.title()) !== 'MyAPI — Your AI gateway, in your control') {
    throw new Error('unexpected website title')
  }
  if (!(await page.getByText('MyAPI', { exact: true }).first().isVisible())) {
    throw new Error('MyAPI brand is not visible')
  }
  const menu = page.locator('[data-mobile-menu]')
  if (await menu.isVisible()) throw new Error('mobile menu should start closed')
  await page.getByRole('button', { name: 'Menu' }).click()
  if (!(await menu.isVisible())) throw new Error('mobile menu did not open')
  await menu.getByRole('link', { name: 'Platform' }).click()
  if (await menu.isVisible()) throw new Error('mobile menu did not close after navigation')
  // The accessible label intentionally changes with the active theme. Use
  // the stable data contract to click, then assert both user-visible state
  // signals so localization or copy changes do not make this smoke flaky.
  const themeToggle = page.locator('[data-theme-toggle]')
  const startedLight = (await page.locator('html.light-preview').count()) > 0
  await themeToggle.click()
  // Chromium's system preference varies by runner. Ensure the assertion is
  // deterministic while still exercising a real state transition.
  if (startedLight) await themeToggle.click()
  if (!(await page.locator('html.light-preview').count())) throw new Error('light theme did not apply')
  if ((await themeToggle.getAttribute('aria-pressed')) !== 'true') throw new Error('theme toggle state did not update')
  mkdirSync('artifacts', { recursive: true })
  await page.screenshot({ path: 'artifacts/myapi-website-mobile.png', fullPage: true })
  console.log('website browser smoke passed (mobile menu, anchor navigation, theme toggle)')
} finally {
  await browser.close()
  server.close()
}
