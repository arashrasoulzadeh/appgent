# Token Optimization Skill

## Principles
- **Read only what you need** - Use `grep`, `glob`, `read` with `offset`/`limit` instead of reading entire files
- **Batch independent operations** - Run multiple tools in parallel when possible
- **Avoid redundant reads** - Cache file contents in memory during a session
- **Use targeted searches** - `grep` with `include` patterns instead of broad searches

## Tool Usage Patterns

### Good (Low Tokens)
```bash
# Read specific lines
read(filePath, offset=50, limit=30)

# Search for pattern in specific files
grep(pattern="function.*Test", include="*_test.go")

# Find files by pattern
glob(pattern="**/services/**/*.go")
```

### Avoid (High Tokens)
```bash
# Reading entire large files
read(filePath)  # without offset/limit

# Broad searches without filters
grep(pattern="error")  # no include, searches everything

# Multiple sequential reads of same file
```

## Response Style
- **Concise** - Answer in 1-3 sentences unless detail requested
- **Direct** - No preamble/postamble ("Here is...", "Based on...")
- **Action-oriented** - Show code/tool output, not explanations

## Context Management
- Clear context between unrelated tasks
- Use `task` tool for complex sub-operations (isolated context)
- Don't accumulate unnecessary file reads in conversation history