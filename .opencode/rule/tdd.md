# TDD Rule

**No code is committed without tests.**

## Requirements
- Every new function, method, or feature must have accompanying tests
- Tests must be written before or alongside the implementation (TDD)
- Minimum coverage: 80% for new code
- Run tests before every commit: `go test ./...` for Go, `pnpm test` for Next.js
- CI will fail if tests don't pass

## Test Structure
- Go: `*_test.go` files in same package
- Next.js: `*.test.tsx` / `*.spec.tsx` alongside components
- Integration tests in `*_integration_test.go` or `__tests__/`

## Enforcement
- Pre-commit hook runs tests
- Code review rejects untested changes
- Coverage reported in PR checks