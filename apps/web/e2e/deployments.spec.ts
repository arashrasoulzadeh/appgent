import { test, expect } from '@playwright/test'

test.describe('Deployments', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/login')
    await page.waitForSelector('input[name="email"]', { state: 'attached', timeout: 30000 })
    await page.fill('input[name="email"]', 'admin')
    await page.fill('input[name="password"]', 'admin')
    await page.click('button[type="submit"]')
    await expect(page).toHaveURL('/dashboard')

    await page.click('button:has-text("New App")')
    await page.fill('input[placeholder="My Portfolio"]', 'Deploy Test')
    await page.fill('textarea[placeholder*="photographer"]', 'A simple app for deployment test')
    await page.click('button:has-text("Create App")')
    await expect(page).toHaveURL(/\/dashboard\/[a-f0-9-]+/)

    await page.waitForSelector('text=succeeded', { timeout: 60000 })
  })

  test('should deploy successful run', async ({ page }) => {
    await page.click('text=Deployments')
    await page.click('button:has-text("Deploy Latest Run")')

    await expect(page.locator('text=Deployment started')).toBeVisible()

    await expect(page.locator('text=deploying')).toBeVisible()
  })

  test('should show deployment in history after completion', async ({ page }) => {
    await page.click('text=Deployments')
    await page.click('button:has-text("Deploy Latest Run")')

    await page.waitForSelector('text=live', { timeout: 60000 })

    await expect(page.locator('text=live')).toBeVisible()
    await expect(page.locator('text=https://')).toBeVisible()
  })

  test('should view preview link', async ({ page }) => {
    await expect(page.locator('button:has-text("View Preview")')).toBeVisible()
  })
})