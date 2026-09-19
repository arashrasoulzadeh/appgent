# Git Commit & Push Skill

## Commit After Each Logical Unit of Work

### When to Commit
- After completing a todo item
- After fixing a bug
- After adding a feature with tests
- After refactoring

### Commit Message Format
```
<type>(<scope>): <subject>

<body>

<footer>
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `style`

Examples:
```
feat(api): add POST /apps endpoint with validation

test(api): add integration tests for app creation

fix(db): handle migration rollback on failure
```

### Workflow
1. Stage changes: `git add -A`
2. Commit: `git commit -m "message"`
3. Push: `git push origin main`

### Pre-commit Checks (run before commit)
```bash
# Go
go fmt ./...
go vet ./...
go test ./...

# Next.js
pnpm lint
pnpm typecheck
pnpm test
```

### Branch Strategy
- Main branch: `main`
- Feature branches: `feat/<short-description>`
- PR required for merge (even for solo work)