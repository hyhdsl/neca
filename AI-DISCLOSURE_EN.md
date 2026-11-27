# AI Participation Disclosure

[简体中文](AI-DISCLOSURE.md) · **English (current)**

> Scope: **neca v1.2.0 (2026-11-27) and the current v1.3 preview development**. This document explains how AI is used in this project, what the human maintainer is responsible for, and how users should interpret testing and acceptance results.

## Summary

**Code authoring, debugging, testing, CI/build scripts, deployment scripts, documentation and help-site content are primarily produced by an AI coding assistant. The human maintainer defines requirements, checks whether key behavior matches expectations, reviews changes and makes release decisions.** Human functional acceptance is not a line-by-line code review or a security audit.

## Division of work

| Work | Primarily performed by |
|---|---|
| Go code, CLI/Web/helper implementation | AI |
| Test code, test fixtures, regression/shadow tests | AI |
| Debugging, fault isolation, implementation fixes | AI |
| CI/CD, Dockerfile, Compose and release scripts | AI |
| Deployment, design, help documentation and English/Traditional Chinese pages | AI |
| Requirements, scope and release decisions | Human maintainer |
| Functional acceptance: running the software and checking expected behavior | Human maintainer |
| Server operations, credential/password/private-key custody | Human maintainer |
| Final merge, release and rollback decisions | Human maintainer |

Additional notes:

- The human maintainer may ask AI to add, remove or modify implementation and documentation, and checks key behavior.
- The human maintainer does not write or review every line; this project must not be represented as having completed a full human security audit.
- Real deployment, permissions, backups and incident response remain human responsibilities. Public documentation does not record environment-specific details.

## AI tooling

Development currently uses **DeepSeek Harness / DeepSeek models** as the AI coding assistant, together with local Docker, Git, GitHub CLI and similar tools for build, test and release operations. Model and tool versions may change during development.

## What this means for users

1. **Acceptance is limited**: passing tests means the defined tests and human acceptance checks passed; it does not cover every boundary, attack surface or deployment environment.
2. **AI output can be wrong**: including incorrect implementations, omissions, stale documentation, outdated examples or unsafe defaults. Review, test and use at your own risk.
3. **This is not a security guarantee**: the project handles CA private keys, VPN certificates and a Web management surface. Security testing is incomplete; commercial or high-risk production use is not recommended. Perform an independent security review, permission check and backup exercise before use.
4. **The license still applies**: AI participation does not change the **AGPL-3.0-only** license, source-disclosure or attribution obligations.
5. **This disclosure may change**: it will be revised as development practices change; the current repository version is authoritative.

## Verifiable information

- Source and commit history: <https://github.com/hyhdsl/neca>
- CI records: <https://github.com/hyhdsl/neca/actions>
- Releases: <https://github.com/hyhdsl/neca/releases>
- License: <https://github.com/hyhdsl/neca/blob/main/LICENSE>
- Copyright and AI disclosure summary: [NOTICE](NOTICE)
- Simplified Chinese full version: [AI-DISCLOSURE.md](AI-DISCLOSURE.md)
- Secondary development and AGPL obligations: [docs/SECONDARY-DEVELOPMENT.md](docs/SECONDARY-DEVELOPMENT.md)