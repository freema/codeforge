# Security Policy

## Reporting a vulnerability

Please report security issues privately, through
[GitHub Security Advisories](https://github.com/freema/codeforge/security/advisories/new).
Do not open a public issue for anything exploitable.

CodeForge is maintained by one person, so please set expectations accordingly:

- Acknowledgement within 7 days
- An assessment, and a fix or a plan, within 30 days for anything confirmed
- Credit in the advisory and release notes, unless you prefer otherwise

If you would rather not use GitHub, contact details are at
[tomasgrasl.cz](https://tomasgrasl.cz).

## Supported versions

Only the latest minor release receives security fixes. Fixes are not
backported to earlier versions — upgrade to the current release.

| Version | Supported |
|---------|-----------|
| 1.1.x   | Yes       |
| < 1.1   | No        |

## Threat model

Running a session means executing a repository's code and letting an AI CLI
decide what else to run, with approvals disabled. Repository content is
attacker-controlled input, and prompt injection from within a repository is a
realistic path — not a theoretical one.

**Current isolation:** the AI CLI runs in the same container as the server,
dropped to a non-root user. That is a privilege boundary, not isolation. There
is no per-session container, VM, or network policy yet.

What the server does contain:

- The CLI subprocess receives an allowlisted environment rather than the
  server's, so the encryption key protecting the credential registry, the
  operator token, webhook secrets and the Redis URL never enter a session
  (`internal/tool/runner/env.go`).
- Git credentials are supplied through short-lived `GIT_ASKPASS` scripts that
  are removed before the CLI starts. Tokens never reach the URL or
  `.git/config`.
- Webhook-triggered work from authors without write access is refused by
  default (`code_review.allow_untrusted_authors`).
- Opening a pull request always requires an explicit action. This is a
  hard-coded invariant, not a configuration flag.

What it does not do yet: isolate sessions from each other, from the SQLite
database on the shared filesystem, or from the network. Per-session
sandboxing, a credential-injecting model proxy, default-deny egress and a
read-only tool tier for untrusted authors are planned.

## Deploying safely

Until per-session isolation lands:

- Point CodeForge only at repositories where everyone with push or PR access is
  trusted, and treat the host as part of that trust domain.
- Leave `code_review.allow_untrusted_authors` at its default of `false`.
- Give the server its own host or VM. Do not run it alongside unrelated
  workloads, and never expose the Docker socket to it.
- Pin a released image tag rather than tracking `:latest`, so upgrades are
  deliberate.
- Scope provider tokens to the repositories that actually need them, and rotate
  them if a session ever behaves unexpectedly.

See [Deployment](docs/deployment.md#isolation-and-untrusted-code) and
[Configuration](docs/configuration.md#who-can-trigger-a-webhook-review) for the
details.

## Scope

In scope: anything letting an untrusted PR author or repository reach the
server's credentials, other sessions, or the host. Authentication and tenant
isolation bypasses. Credential leakage through logs, API responses, or the
session environment.

Out of scope: the absence of per-session sandboxing, which is documented above
and on the roadmap; issues requiring operator-level access you already hold;
and vulnerabilities in the AI CLIs themselves, which belong with their vendors.
