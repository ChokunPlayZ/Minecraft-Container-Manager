# Repository instructions

## Code changes

- Keep changes focused on the task and preserve unrelated work already in progress.
- Follow the repository’s conventions and reuse existing helpers and APIs when they fit. Prefer clear names and straightforward control flow. Use comments to explain decisions the code cannot express on its own.
- Preserve existing behavior and compatibility with saved data unless the task explicitly changes them. Validate input at system boundaries and handle errors with enough context to diagnose them.
- Consider partial failures, retries, and concurrent calls when changing persistent state or external side effects. Keep credentials out of source control and logs.
- Update affected documentation and configuration examples when changing user-facing behavior, setup, or defaults.

## Verification

- Run checks relevant to the change. Use `make lint` and `make test` for Go, `make test-web` for frontend changes, and `make test-deploy` for deployment changes. Use `make verify` for the full suite.
- Add or update focused tests when changing behavior or fixing a regression that existing tests miss. Report any relevant check that could not be completed.

## Commits

- Always create a Git commit after completing and verifying a task that changes repository files, before reporting the task as done. Review the diff and include only changes that belong to the task.
- Use this commit subject format, including the parentheses and the space after the colon.

  ```text
  (type): changes made
  ```

- Choose a lowercase type such as `feat`, `fix`, `docs`, `refactor`, `test`, or `chore`. Describe the specific change in a short imperative phrase, for example `(fix): handle missing server configuration`.
