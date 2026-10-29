## Summary

When you pull this repo, immediately run:

```bash
make install-hooks   # installs pre-commit, commit-msg, pre-push
```

After that, every commit is automatically checked.

## What Gets Checked Automatically

### Pre-commit (every `git commit`)
| Check | What it catches |
|---|---|
| Float in monetary code | `float32`/`float64` in `internal/ledger/`, `internal/settlement/`, `internal/recon/` |
| SQL string formatting | `fmt.Sprintf` with SQL fragments (injection risk) |
| Secret patterns | Passwords, API keys, tokens in staged files |
| Clock discipline | Direct `time.Now()` in settlement/ledger code |
| Stubs in production | `// TODO:` and `panic("not implemented")` without `// RAMP-ONLY` tag |
| go fmt | All staged `.go` files must be formatted |
| go vet | No vet issues on staged code |
| Synthetic data | No real IFSC/account number patterns in `tests/` or `spec/` |
| Migration format | `-- goose Up` and `-- goose Down` present in every migration |
| BRD citation | Scenario test files must have `BRD §` or `Spec version:` comment |

### Commit-msg (every `git commit`)
- Must follow `type(scope): description` format
- Security commits need ≥50 char description
- Migration commits show a Tier-2 warning

### Pre-push (pushes to `main` or `develop`)
- Runs `go test -race` on `./internal/...`
- Push fails if any test fails

## Bypassing (Emergency Only)

```bash
git commit --no-verify -m "security: emergency hotfix for CVE-XXXX — bypassing hooks, will fix lint in follow-up"
```

**The commit message must explain why you bypassed and reference the follow-up.** The CI gate still runs and will catch the issue. `--no-verify` buys you time, not an excuse.
