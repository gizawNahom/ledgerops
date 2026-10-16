# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub:
[**Report a vulnerability**](https://github.com/gizawNahom/ledgerops/security/advisories/new)
(Security tab → "Report a vulnerability"). Don't open a public issue.

Helpful details:

- what an attacker can do, and what they need first (a tenant key, the
  operator key, database access)
- steps to reproduce, ideally against the `make up` stack
- the version or commit you tested

ledgerops is maintained by one person, so responses are best-effort. You'll
get an acknowledgement, and the fix will be credited to you unless you'd
rather it weren't.

## Supported versions

ledgerops is pre-1.0 and not yet production-ready. Fixes land on `main` and
in the next release; older releases don't get backports.

## Out of scope

- The credentials in `docker-compose.yml` (`demo-operator-key`,
  `demo-tenant-key`, the database passwords). They exist only for the local
  demo stack and are public on purpose.
- The lack of TLS in the service itself. It's a documented limitation: run
  it behind a TLS-terminating proxy.
