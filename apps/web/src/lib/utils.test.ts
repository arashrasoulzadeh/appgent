import { describe, it, expect } from 'vitest'
import {
  cn,
  formatDate,
  formatDateTime,
  truncate,
  slugify,
  generateSlug,
  getInitials,
} from './utils'

describe('utils', () => {
  describe('cn', () => {
    it('merges class names correctly', () => {
      expect(cn('foo', 'bar')).toBe('foo bar')
    })

    it('handles conditional classes', () => {
      expect(cn('foo', true && 'bar', false && 'baz')).toBe('foo bar')
    })

    it('merges tailwind classes with twMerge', () => {
      expect(cn('p-2', 'p-4')).toBe('p-4')
    })
  })

  describe('formatDate', () => {
    it('formats date correctly', () => {
      const date = new Date('2024-01-15T10:30:00Z')
      const result = formatDate(date)
      expect(result).toContain('Jan')
      expect(result).toContain('15')
      expect(result).toContain('2024')
    })

    it('handles string input', () => {
      const result = formatDate('2024-01-15T10:30:00Z')
      expect(result).toContain('Jan')
    })
  })

  describe('formatDateTime', () => {
    it('formats date and time correctly', () => {
      const date = new Date('2024-01-15T10:30:00Z')
      const result = formatDateTime(date)
      expect(result).toContain('Jan')
      expect(result).toContain('15')
      expect(result).toContain('2024')
      expect(result).toMatch(/\d{2}:\d{2}/)
    })
  })

  describe('truncate', () => {
    it('truncates long strings', () => {
      expect(truncate('hello world', 8)).toBe('hello...')
    })

    it('returns original string if shorter than length', () => {
      expect(truncate('hi', 10)).toBe('hi')
    })

    it('handles exact length', () => {
      expect(truncate('hello', 5)).toBe('hello')
    })
  })

  describe('slugify', () => {
    it('converts to lowercase and replaces spaces', () => {
      expect(slugify('Hello World')).toBe('hello-world')
    })

    it('removes special characters', () => {
      expect(slugify('Hello@World!')).toBe('helloworld')
    })

    it('handles multiple spaces', () => {
      expect(slugify('Hello   World')).toBe('hello-world')
    })

    it('trims leading/trailing hyphens', () => {
      expect(slugify('-hello-')).toBe('hello')
    })
  })

  describe('generateSlug', () => {
    it('generates slug with random suffix', () => {
      const slug = generateSlug('My App')
      expect(slug).toMatch(/^my-app-[a-z0-9]{6}$/)
    })

    it('generates different suffixes', () => {
      const slug1 = generateSlug('Test')
      const slug2 = generateSlug('Test')
      expect(slug1).not.toBe(slug2)
    })
  })

  describe('getInitials', () => {
    it('gets initials from two words', () => {
      expect(getInitials('John Doe')).toBe('JD')
    })

    it('handles single word', () => {
      expect(getInitials('John')).toBe('J')
    })

    it('handles more than two words', () => {
      expect(getInitials('John Michael Doe')).toBe('JM')
    })

    it('uppercases result', () => {
      expect(getInitials('john doe')).toBe('JD')
    })
  })
})