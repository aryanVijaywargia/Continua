<!--
  Source: claude-code-skill-factory
  Original Path: .claude/commands/security-scan.md
  Adaptations: Go security tools (gosec, nancy), removed Python-specific tools
-->
---
description: Perform local security scanning for Go projects
---

## Local Scan

1. **Install tooling** (first run):
   ```bash
   # Install gitleaks for secrets detection
   brew install gitleaks
   # Or: go install github.com/zricethezav/gitleaks/v8@latest

   # Install gosec for Go security analysis
   go install github.com/securego/gosec/v2/cmd/gosec@latest

   # Install nancy for dependency vulnerability scanning
   go install github.com/sonatype-nexus-community/nancy@latest
   ```

2. **Detect secrets**:
   ```bash
   gitleaks detect --verbose --redact
   ```

3. **Go security analysis**:
   ```bash
   gosec ./...
   ```

4. **Dependency vulnerability scan**:
   ```bash
   go list -json -deps ./... | nancy sleuth
   ```

5. **Document results** in commit message or PR description.

## Quick Security Check

Run all checks in sequence:
```bash
echo "==> Checking for secrets..." && gitleaks detect --verbose --redact && \
echo "==> Running Go security analysis..." && gosec ./... && \
echo "==> Scanning dependencies..." && go list -json -deps ./... | nancy sleuth && \
echo "Security scan complete."
```

## Completion
- Ensure all scans are clean before pushing or merging.
- Address any HIGH or CRITICAL findings immediately.
- Document accepted risks for MEDIUM findings.

## Common Go Security Issues to Watch For
- SQL injection in raw queries (use parameterized queries or sqlc)
- Hardcoded credentials or API keys
- Insecure TLS/SSL configurations
- Command injection in exec calls
- Path traversal vulnerabilities
- Unvalidated redirects
