---
title: Quick Start
description: Get up and running with grepai in 5 minutes
---

## 1. Initialize grepai

Navigate to your project directory and run:

```bash
cd /path/to/your/project
grepai init
```

This creates a `.grepai/` directory with a `config.yaml` file.

## 2. Start the Indexing Daemon

```bash
grepai watch
```

This will:
1. Scan your codebase
2. Split files into chunks
3. Generate embeddings for each chunk
4. Store vectors in the local index
5. Watch for file changes and update the index in real-time

You'll see a progress bar during the initial indexing.

## 3. Search Your Code

Open a new terminal and search:

```bash
# Find authentication code
grepai search "user authentication"

# Find error handling
grepai search "how errors are handled"

# Find API endpoints
grepai search "REST API routes"

# Limit results
grepai search "database queries" --limit 10

# JSON output for AI agents
grepai search "authentication" --json
```

## 4. Check Index Status

```bash
grepai status
```

This shows:
- Number of indexed files
- Number of chunks
- Storage backend status
- Last update time

## Example Output

```
$ grepai search "error handling middleware"

Score: 0.89 | middleware/error.go:15-45
────────────────────────────────────────
func ErrorHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()
        if len(c.Errors) > 0 {
            err := c.Errors.Last()
            // ... handle error
        }
    }
}

Score: 0.82 | handlers/api.go:78-95
────────────────────────────────────────
// handleAPIError wraps errors with context
func handleAPIError(w http.ResponseWriter, err error) {
    // ...
}
```

## Tips for Better Searches

- **Be descriptive**: "user login validation" works better than "login"
- **Use natural language**: "where are users saved" instead of "save user"
- **Think about intent**: describe *what* the code does, not *how* it's written

## Next Steps

- [Configuration](/grepai/configuration/) - Customize chunking, embedders, and storage
- [Commands Reference](/grepai/commands/grepai/) - Full CLI documentation
- [AI Agent Setup](/grepai/commands/grepai_agent-setup/) - Integrate with Cursor or Claude Code
