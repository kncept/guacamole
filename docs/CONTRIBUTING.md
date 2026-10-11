# Contributing

Thanks for wanting to contribute to Guacamole! This project is developed on
GitHub at [kncept/guacamole](https://github.com/kncept/guacamole). The standard
workflow for any change is **fork → branch → pull request**.

## Before you start

- Read [the docs](.) — particularly [ARCHITECTURE](ARCHITECTURE.md),
  [GUI](GUI.md), [TOOLS](TOOLS.md) and [PROVIDERS](PROVIDERS.md) — and the
  [README](../README.md).
- Check the [issues](https://github.com/kncept/guacamole/issues) and open PRs
  first. For a larger change, open an issue to agree on the direction before
  writing code.

## Fork → branch → PR

1. **Clone your fork** and branch from an up-to-date `main`:

   ```sh
   git clone git@github.com:<you>/guacamole.git && cd guacamole
   git remote add upstream git@github.com:kncept/guacamole.git
   git fetch upstream
   git switch -c <your-branch> upstream/main
   ```

   One logical change per branch, with a short name like
   `fix-permission-prompt`.

2. **Make the change**, matching the surrounding code and adding tests where
   sensible, then run:

   ```sh
   go build ./... && go vet ./... && go test ./...
   ```

3. **Commit, push, and open a PR** against `main`:

   ```sh
   git add -A
   git commit -m "Describe what this change does and why"
   git push -u origin <your-branch>
   ```

   Say what changed, why, and how you tested it; reference any issue it
   closes (e.g. `Closes #123`).

## After opening the PR

- If the PR lives a while, keep it rebased on `main`:

  ```sh
  git fetch upstream
  git rebase upstream/main
  git push --force-with-lease
  ```

- Address review feedback in follow-up commits, and ask if a comment is
  unclear.
- Keep the PR scoped. A second, independent change belongs in its own PR.

## Guidelines

- **Style:** match the surrounding code; keep Go changes `gofmt`-clean.
- **Tests:** add or update tests for behavior changes.
- **Docs:** update the relevant doc in `docs/` for user-visible or structural
  changes.
- **Commits:** small, logical commits with imperative summaries
  (“Add …”, “Fix …”).
- **Don't** include unrelated formatting churn or out-of-scope config or
  dependency changes.

## Reporting issues and bugs

Found a bug? Open an issue at
[github.com/kncept/guacamole/issues](https://github.com/kncept/guacamole/issues)
with what you did, what you expected, and what happened — including the
relevant commands, errors, and (for provider issues) the provider
configuration involved, keys redacted.