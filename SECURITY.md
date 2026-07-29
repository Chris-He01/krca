# Security Policy

Please do not report vulnerabilities in public issues. Contact the maintainers
privately through the security contact configured for this repository.

Never commit API keys, access tokens, private keys, production URLs, customer
data, or internal operational material. Use environment variables and the
checked-in example configuration files.

If a credential is committed, revoke or rotate it immediately. Removing it in
a later commit is not sufficient because it remains in Git history.
