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

## Optional reviews: explicit capability opt-in

On 2026-10-09 repository secret names showed no `CLAUDE_CODE_OAUTH_TOKEN`, and
PR run 37897375250 skipped its actual Claude action. Dependency run 37897375202
reported SBOM HTTP 404; a separate authenticated SBOM lookup also returned 404.
These previously green no-op jobs did not provide substantive reviews.

Their unchanged check names now report **skipped** without allocating runners
when the following repository Actions variables are unset or not `true`:

| Variable | Enable only after |
| --- | --- |
| `CLAUDE_REVIEW_ENABLED=true` | Configuring the `CLAUDE_CODE_OAUTH_TOKEN` secret and verifying review works. |
| `DEPENDENCY_REVIEW_ENABLED=true` | Verifying repository dependency graph/review capability; authenticated SBOM returns 200. |

Set these through repository Settings → Secrets and variables → Actions →
Variables when the capability becomes available. This PR does not edit any
secrets, variables, plans, visibility or workflow enablement. Revisit the flags
when credentials or dependency-review availability change; they do not
automatically detect a newly added capability. A skipped review is not an
independent review or vulnerability scan.

PR issue comments mentioning `@claude` remain independent of the automatic
Claude flag, with the existing secret-presence guard; absent credentials yield
an explicit warning, not a review. Automatic fork reviews still do not access
secrets. Enabled dependency review retains the runtime availability probe,
high-severity failure threshold and GPL-3.0/AGPL-3.0 policy.

Branch protection and ruleset inspection returned HTTP 403 due to this private
repository's plan. No required-check setting was changed; all existing check
names remain. If plan/protection changes later, verify optional skipped checks
are acceptable and enable capabilities before relying on them as review gates.

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
billing records. Optional no-op gates could avoid approximately 196 of 1,851
minutes over an equivalent Oct 1–9 workload while capabilities remain absent.
Cache and cancellation benefits are workload-dependent and not yet measured;
rounding and cache overhead can erase small duration gains.
