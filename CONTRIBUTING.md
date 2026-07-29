# Contributing

1. Create a focused branch.
2. Copy `.env.example` to `.env` and keep local values untracked.
3. Run `bash scripts/check-secrets.sh`.
4. Run `go test ./...`.
5. Open a pull request describing the behavior change and tests.

Do not include company-confidential prompts, production endpoints, customer
data, credentials, or personal filesystem paths in code, fixtures, screenshots,
issues, or pull requests.
