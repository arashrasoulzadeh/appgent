import { test, expect } from '@playwright/test'

test.describe('Authentication', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/login')
    await page.waitForSelector('input[name="email"]', { state: 'attached', timeout: 30000 })
  })

  test('should display login page', async ({ page }) => {
    await expect(page.locator('h1')).toContainText('Sign in to Appgent')
    await expect(page.locator('input[name="email"]')).toBeVisible()
    await expect(page.locator('input[name="password"]')).toBeVisible()
    await expect(page.locator('button[type="submit"]')).toBeVisible()
  })

  test('should login with valid credentials', async ({ page }) => {
    await page.fill('input[name="email"]', 'admin')
    await page.fill('input[name="password"]', 'admin')
    await page.click('button[type="submit"]')

    await expect(page).toHaveURL('/dashboard')
    await expect(page.locator('h1')).toContainText('Your Apps')
  })

  test('should show error with invalid credentials', async ({ page }) => {
    await page.fill('input[name="email"]', 'admin')
    await page.fill('input[name="password"]', 'wrong')
    await page.click('button[type="submit"]')

    await expect(page.locator('text=Login failed')).toBeVisible()
  })

  test('should logout', async ({ page }) => {
    await page.fill('input[name="email"]', 'admin')
    await page.fill('input[name="password"]', 'admin')
    await page.click('button[type="submit"]')
    await expect(page).toHaveURL('/dashboard')

    await page.click('text=Log out')
    await expect(page).toHaveURL('/login')
  })
})