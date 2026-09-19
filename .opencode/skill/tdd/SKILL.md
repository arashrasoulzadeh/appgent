# TDD Skill

## Workflow
1. **Write failing test first** - Define expected behavior
2. **Implement minimal code** - Make test pass
3. **Refactor** - Clean up while keeping tests green
4. **Repeat** - For each new requirement

## Go Testing Patterns
```go
// Table-driven tests for multiple cases
func TestFunction(t *testing.T) {
    tests := []struct{
        name string
        input Input
        want Output
    }{
        {"case 1", input1, output1},
        {"edge case", input2, output2},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got := Function(tc.input)
            assert.Equal(t, tc.want, got)
        })
    }
}

// Mock interfaces for external deps
type MockDB struct {
    QueryFunc func(query string, args ...any) (Rows, error)
}
func (m *MockDB) Query(query string, args ...any) (Rows, error) {
    return m.QueryFunc(query, args...)
}
```

## Next.js Testing Patterns
```tsx
// Component test with RTL
import { render, screen } from '@testing-library/react'
import { Button } from './Button'

test('renders and handles click', () => {
  const handleClick = vi.fn()
  render(<Button onClick={handleClick}>Click me</Button>)
  fireEvent.click(screen.getByRole('button'))
  expect(handleClick).toHaveBeenCalledTimes(1)
})
```

## Commands
- Go: `go test ./... -cover`
- Next.js: `pnpm test --coverage`