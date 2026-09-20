import { test, expect } from '@playwright/test'

test.describe('App Creation', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/login')
    await page.waitForSelector('input[name="email"]', { state: 'attached', timeout: 30000 })
    await page.fill('input[name="email"]', 'admin')
    await page.fill('input[name="password"]', 'admin')
    await page.click('button[type="submit"]')
    await expect(page).toHaveURL('/dashboard')
  })

  test('should create a new app', async ({ page }) => {
    await page.click('button:has-text("New App")')

    await expect(page.locator('h2')).toContainText('Create New App')

    await page.fill('input[placeholder="My Portfolio"]', 'Test Portfolio')
    await page.fill('textarea[placeholder*="photographer"]', 'A portfolio site for a photographer with gallery and contact form')

    await page.click('button:has-text("Create App")')

    await expect(page).toHaveURL(/\/dashboard\/[a-f0-9-]+/)
    await expect(page.locator('h1')).toContainText('Test Portfolio')
  })

  test('should show validation errors for empty fields', async ({ page }) => {
    await page.click('button:has-text("New App")')
    await page.click('button:has-text("Create App")')

    await expect(page.locator('text=App Name is required')).toBeVisible()
    await expect(page.locator('text=Prompt is required')).toBeVisible()
  })

  test('should create PWA app', async ({ page }) => {
    await page.click('button:has-text("New App")')

    await page.fill('input[placeholder="My Portfolio"]', 'My PWA')
    await page.selectOption('select', 'pwa')
    await page.fill('textarea[placeholder*="photographer"]', 'An offline-capable PWA')

    await page.click('button:has-text("Create App")')

    await expect(page).toHaveURL(/\/dashboard\/[a-f0-9-]+/)
    await expect(page.locator('text=PWA')).toBeVisible()
  })
})