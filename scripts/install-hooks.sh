#!/usr/bin/env bash
# install-hooks — installs the pre-commit hook and any other git hooks
# Run once after cloning: make install-hooks

set -euo pipefail

HOOKS_DIR=".git/hooks"
SCRIPTS_DIR="scripts"

if [ ! -d "$HOOKS_DIR" ]; then
  echo "ERROR: .git/hooks not found. Run this from the repo root after git init."
  exit 1
fi

echo "Installing git hooks..."

cp "$SCRIPTS_DIR/pre-commit" "$HOOKS_DIR/pre-commit"
chmod +x "$HOOKS_DIR/pre-commit"
echo "  ✓ pre-commit installed"

# commit-msg hook: enforce conventional commits
cat > "$HOOKS_DIR/commit-msg" << 'EOF'
#!/usr/bin/env bash
# Enforce conventional commit format:
# type(scope): description
# Types: feat|fix|test|docs|chore|refactor|perf|ci|security
# Example: feat(ledger): add idempotency insert-first

COMMIT_MSG=$(cat "$1")
PATTERN='^(feat|fix|test|docs|chore|refactor|perf|ci|security|wip)(\([a-z0-9-]+\))?: .{1,80}$'

if [[ "$COMMIT_MSG" =~ ^Merge ]]; then
  exit 0  # Allow merge commits
fi

if ! echo "$COMMIT_MSG" | grep -qE "$PATTERN"; then
  echo "ERROR: Commit message does not follow conventional commit format."
  echo "  Format:  type(scope): description"
  echo "  Types:   feat|fix|test|docs|chore|refactor|perf|ci|security|wip"
  echo "  Example: feat(ledger): add idempotency insert-first"
  echo ""
  echo "  Your message: $COMMIT_MSG"
  exit 1
fi

# Security commits require justification
if [[ "$COMMIT_MSG" =~ ^security ]]; then
  if [ ${#COMMIT_MSG} -lt 50 ]; then
    echo "ERROR: Security commits require a description of at least 50 characters."
    exit 1
  fi
fi

# Migration commits are Tier-2 — warn
if [[ "$COMMIT_MSG" =~ db/migrations|migration ]]; then
  echo "WARNING: This commit touches migrations (Tier-2 change)."
  echo "  Ensure you have:"
  echo "  1. Tested 'goose up' and 'goose down' locally"
  echo "  2. Added rollback in '-- goose Down' section"
  echo "  3. Checked for table locks (avoid NOT NULL without DEFAULT on large tables)"
fi
EOF
chmod +x "$HOOKS_DIR/commit-msg"
echo "  ✓ commit-msg installed"

# pre-push hook: run tests before pushing to main
cat > "$HOOKS_DIR/pre-push" << 'EOF'
#!/usr/bin/env bash
# Run fast tests before pushing to main or develop

BRANCH=$(git rev-parse --abbrev-ref HEAD)

if [[ "$BRANCH" == "main" || "$BRANCH" == "develop" ]]; then
  echo "Running tests before push to $BRANCH..."
  go test -race -count=1 -timeout=60s ./internal/... || {
    echo "FAIL: Tests failed. Fix before pushing to $BRANCH."
    exit 1
  }
  echo "✓ Tests passed"
fi
EOF
chmod +x "$HOOKS_DIR/pre-push"
echo "  ✓ pre-push installed"

echo ""
echo "All hooks installed. Happy building."
