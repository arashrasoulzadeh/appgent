import { test, expect } from '@playwright/test'

test.describe('Run Details', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/login')
    await page.waitForSelector('input[name="email"]', { state: 'attached', timeout: 30000 })
    await page.fill('input[name="email"]', 'admin')
    await page.fill('input[name="password"]', 'admin')
    await page.click('button[type="submit"]')
    await expect(page).toHaveURL('/dashboard')

    await page.click('button:has-text("New App")')
    await page.fill('input[placeholder="My Portfolio"]', 'Test App')
    await page.fill('textarea[placeholder*="photographer"]', 'A simple test app')
    await page.click('button:has-text("Create App")')
    await expect(page).toHaveURL(/\/dashboard\/[a-f0-9-]+/)
  })

  test('should display run details with agent steps', async ({ page }) => {
    await expect(page.locator('h1')).toContainText('Test App')
    await expect(page.locator('text=Run v1')).toBeVisible()

    await expect(page.locator('text=Plan')).toBeVisible()
    await expect(page.locator('text=Design')).toBeVisible()
    await expect(page.locator('text=Code')).toBeVisible()
    await expect(page.locator('text=QA')).toBeVisible()
  })

  test('should expand agent step accordion', async ({ page }) => {
    await page.click('text=Plan')
    await expect(page.locator('text=Attempt 1')).toBeVisible()
    await expect(page.locator('text=nvidia/nemotron-3-ultra:free')).toBeVisible()
  })

  test('should navigate between tabs', async ({ page }) => {
    await page.click('text=Deployments')
    await expect(page.locator('text=No deployments yet')).toBeVisible()

    await page.click('text=History')
    await expect(page.locator('text=Run History')).toBeVisible()
  })

  test('should regenerate app', async ({ page }) => {
    await page.click('button:has-text("Regenerate")')

    await expect(page.locator('h2')).toContainText('Regenerate App')

    await page.fill('textarea[placeholder*="instructions"]', 'Make it dark mode')
    await page.click('button:has-text("Regenerate")')

    await expect(page.locator('text=Regeneration started')).toBeVisible()
  })
})