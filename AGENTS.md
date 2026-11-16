# Repository instructions

This repository is the ClickHouse MCP server. User setup is in `README.md` and `README.zh.md`. Keep those two files aligned.

`scripts/` and `templates/` are retained template assets. Do not add this repository's module path, npm scope, image name, or tokens to those files. `./scripts/init-template.sh` initializes a new repository created from the template. This repository is already initialized. Do not run that script as a build step.

## Conventions

- Keep code, workflow identifiers, and comments in English.
- When a user-facing document exists in English and Chinese, update both.
- Keep generated binary names, npm package names, and supported platforms in `.github/ci/release-platforms.json`. The README states which packages and images are actually published.

## Verification

For Go and workflow changes, run `make ci`. That target runs `lint`, `test`, and `build`. It does not run `go test -race` or `make test-integration`. The GitHub Actions build workflow runs the race tests and `make test-template-init`. When `actionlint` is installed, run it on a workflow you changed.

Do not commit or publish unless the user asks.
