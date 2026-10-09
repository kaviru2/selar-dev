# CI cost controls

Issue #155 scopes these changes to hosted workflow configuration. Automatic jobs
still use `ubuntu-latest`; no self-hosted runner, canary or visibility change is
part of this work.

## Checks that always run

CI, coverage and synthetic service E2E retain their job IDs, check names and
commands. CI still responds to `merge_group`. There are no new path filters or
job-level conditions on these checks: docs-only PRs continue to exercise them.
Backups, deploy scripts, release and Pages protection are unchanged.

A newer run cancels older runs only within the same workflow and PR. Every
non-PR invocation gets a unique concurrency group (run ID), so main pushes,
merge queues, manual dispatches and manual reviews cannot cancel each other.
CI jobs have 20-minute limits, coverage 15 minutes, and synthetic E2E retains
25 minutes. These are guardrails, not expected runtimes.

Go caching is retained. Python jobs cache pip downloads/wheels by worker
requirements; console jobs cache the pnpm store by its lockfile. pnpm is set up
before setup-node resolves its cache. Every dependency installation still runs;
no environment, `node_modules`, credentials or production data is cached.

## Dependency scanning and optional Claude review

The canonical repository is now public. Dependency Review runs by default on
public-repository PRs, independently of `DEPENDENCY_REVIEW_ENABLED`. It calls
the review action directly, retaining the high-severity failure threshold and
GPL-3.0/AGPL-3.0 policy. API errors fail visibly: an SBOM probe is not allowed to
turn an unavailable scan into a successful no-op. Private copies can explicitly
opt in with `DEPENDENCY_REVIEW_ENABLED=true` after verifying capability.

The earlier private-repository audit on 2026-10-09 observed SBOM HTTP 404 and
no substantive scan in run 37897375202. After publication, a local authenticated
SBOM lookup still returned 404 and a dependency comparison returned 403; those
local-token results do not establish what the workflow token can access.
Exact-head Dependency Review execution, not visibility alone, verifies scanning.

Automatic Claude review remains optional: `CLAUDE_REVIEW_ENABLED=true` requires
configuring `CLAUDE_CODE_OAUTH_TOKEN` and verifying review works. The earlier
secret-name audit found no credential; run 37897375250 skipped its actual
Claude action. With the flag unset, the check explicitly skips without a runner.
PR issue comments mentioning `@claude` remain independent of the automatic
flag and retain the secret-presence guard. Missing credentials yield a warning,
not an independent review. Automatic fork reviews do not access secrets.

The earlier protection/ruleset HTTP 403 observations describe the historical
private-repository audit, not the current public repository's protection state.
No secrets, variables, plans, visibility, required checks or other repository
settings are changed by this PR. A skipped review is never a vulnerability scan.

## Regression verification

```sh
python -m pip install PyYAML
python scripts/test_workflow_contracts.py
# If installed:
actionlint -shellcheck='' -pyflakes='' .github/workflows/*.yml
```

The worker CI job also runs the contract tests. They check concurrency scope,
limits, cache ordering, retained dependency installs, manual review routing,
check names and the merge-group trigger. They do not substitute for the real
service tests, Postgres integration, coverage or browser E2E.

Historical audit estimates sum per-job elapsed minutes rounded upward, not
billing records. The earlier private-repository scenario estimated approximately
196 of 1,851 minutes from suppressing both no-op reviews over an equivalent
Oct 1–9 workload. That scenario is not a current savings claim: public dependency
review now executes by default.
Cache and cancellation benefits are workload-dependent and not yet measured;
rounding and cache overhead can erase small duration gains.
