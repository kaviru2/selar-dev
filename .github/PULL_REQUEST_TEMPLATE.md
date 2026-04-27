## Summary

Brief description of what this PR does.

Closes #(issue number)

## Changes

- Change 1
- Change 2
- Change 3

## Service(s) affected

- [ ] `selar-console` (frontend)
- [ ] `selar-api` (Go backend)
- [ ] `selar-worker` (Python ingestion)
- [ ] Database / migrations
- [ ] Documentation
- [ ] CI / build

## Type of change

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to change)
- [ ] Documentation update
- [ ] Refactor (no functional changes)

## Checklist

- [ ] My code follows the project's code style guidelines
- [ ] I have tested my changes locally (all three services start without errors)
- [ ] I have run the relevant build commands:
  - [ ] `go build ./...` (if API changed)
  - [ ] `pnpm build` (if console changed)
  - [ ] `python3 -m py_compile main.py` (if worker changed)
- [ ] I have updated documentation if necessary
- [ ] My commits follow the [Conventional Commits](https://www.conventionalcommits.org/) convention

## Screenshots

If applicable, add screenshots showing the visual impact of your changes.

## Additional Notes

Any context the reviewer should know (design decisions, trade-offs, follow-up work needed).
