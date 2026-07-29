# Open-source release checklist

The working tree has been sanitized, but the repository must not be made
public until every item below is complete.

## Required before publishing

- Revoke or rotate every credential that has ever appeared in the repository.
  Treat deleted and replaced values as compromised.
- Rewrite Git history to remove the deleted private directories and old
  versions of sanitized configuration files.
- Perform the rewrite on a fresh mirror clone using `git filter-repo`, review
  the result, then force-push only after coordinating with all collaborators.
- Run a full-history scanner such as Gitleaks or TruffleHog against every ref.
- Review issues, pull requests, release assets, CI logs, container images, and
  package registries; rewriting Git does not clean those systems.
- Choose and add an OSI-approved license.
- Configure a private security-reporting contact in `SECURITY.md`.
- Have the relevant legal/security owner review the release.

## Local verification

```bash
bash scripts/check-secrets.sh
go test ./...
git diff --check
```

After a history rewrite, all existing clones should be discarded and cloned
again so removed objects are not accidentally pushed back.
