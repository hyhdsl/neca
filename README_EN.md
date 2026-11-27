# Neko's Efficient Certificate Administrator

**README language / 语言：** [简体中文（authoritative）](README.md) · [繁體中文](README_zh-TW.md) · [English (current)](README_EN.md)

> [!IMPORTANT]
> This English README is a convenience translation. The [Chinese README](README.md) is the **authoritative version**; this translation may lag behind and is not guaranteed to be current. Other language versions will be updated separately on a scheduled basis. Normally, only the Chinese README is kept up to date.
>
> Translation status: synchronized with the v1.2.0 release on 2026-11-27; the Chinese README remains authoritative.

> [!WARNING]
> **AI disclosure (read before use)**: Code, debugging, tests, CI/build/deploy scripts, documentation and help-site content are primarily produced by an AI coding assistant; the human maintainer is responsible for requirements, review, functional acceptance and production operations. See [AI-DISCLOSURE_EN.md](AI-DISCLOSURE_EN.md) for the full scope.
>
> **Limitation of liability**: The software is provided “as is”, without express or implied warranties. To the extent permitted by applicable law, the authors and AI-tool providers are not liable for losses caused by use or misuse.
>
> **Security boundary**: security testing is incomplete; commercial or high-risk production use is not recommended. If you use it, perform an independent security review, backups and rollback validation.
>
> **Contributions**: open an Issue using the Bug / Feature / Question template, or [choose a template](https://github.com/hyhdsl/neca/issues/new/choose); read the [development policy](docs/SECONDARY-DEVELOPMENT.md) before submitting a PR. Derivative work must comply with AGPL.
>
> Related files: [LICENSE](LICENSE) · [NOTICE](NOTICE) · [AI-DISCLOSURE_EN.md](AI-DISCLOSURE_EN.md) · [docs/SECONDARY-DEVELOPMENT.md](docs/SECONDARY-DEVELOPMENT.md)

**Neko's Efficient Certificate Administrator (neca, also written NECA)** is a lightweight certificate manager for strongSwan IKEv2. One Go CLI, an optional web panel that is read-only by default, and a controlled root-helper write boundary.

🌐 [Home](https://neca.nekobiglazyer.work) · 🇨🇳 [Chinese docs](https://neca.nekobiglazyer.work/zh/) · 🇬🇧 [English](https://neca.nekobiglazyer.work/en/) · 📦 [v1.2.0](https://github.com/hyhdsl/neca/releases/tag/v1.2.0)

## ✨ Features

- 🔐 **Secure by default**: the web panel is read-only; the web container is non-root and has no <code>ca.key</code> or <code>docker.sock</code>.
- 🧾 **Full lifecycle**: issue / reissue / revoke / unrevoke / delete, with automatic CRL maintenance.
- 📦 **Platform bundles**: Windows / Linux / macOS / Android / iOS; optional installers, p12 profiles and zip downloads.
- 🐳 **Simple deployment**: a single static binary; separate lite and neca-web images for amd64 / arm64.
- ⚙️ **Operations-ready**: JSON config, <code>--json</code> / <code>--dry-run</code>, audit logs, SIGHUP hot reload and a persistent requestId idempotency cache.
- 🧩 **Extensible**: <code>internal/core/ports</code> provides index-store and config-source interfaces.

## 🚀 Quick start

Pin the version, dry-run first, then issue:

~~~bash
docker pull ghcr.io/hyhdsl/neca:v1.2.0
neca version
neca cert list
neca cert issue alice --platforms windows,android --zip --dry-run
neca cert issue alice --platforms windows,android --zip
~~~

When deploying, mount <code>/opt/ipsec</code> into the container and run it through the wrapper or compose. See the [install, upgrade and rollback guide](https://neca.nekobiglazyer.work/en/#/install) for the full procedure.

## 🖥️ Web panel

Read-only by default. To enable writes, explicitly start the root helper and turn read-only off:

~~~bash
docker compose -f /opt/ipsec/docker-compose.neca.yml --profile web up -d web
docker compose -f /opt/ipsec/docker-compose.neca.yml --profile web --profile web-write up -d web web-helper
neca web readonly off
~~~

Deployment config lives at <code>/opt/ipsec/config/neca.json</code>; the web container sees SIGHUP reloads through the config-directory mount. OpenSSH local forwarding remains the recommended default; do not expose the panel publicly without an access design.

## 📚 Documentation

| Entry | Contents |
|---|---|
| 🌐 [Home](https://neca.nekobiglazyer.work) | Project overview and quick links |
| 🇨🇳 [Chinese help](https://neca.nekobiglazyer.work/zh/) | Installation, certificates, web, API, security, FAQ |
| 🇬🇧 [English](https://neca.nekobiglazyer.work/en/) | English core pages |
| ⌨️ [Command cheat sheet](https://neca.nekobiglazyer.work/en/#/cheatsheet) | Copy-ready common commands |
| ⌨️ [Command reference](https://neca.nekobiglazyer.work/en/#/commands) | Global options, every subcommand, arguments, confirmation and exit codes |
| 🧪 [Web API](https://neca.nekobiglazyer.work/zh/#/api) | Experimental API reference |
| 🐳 [Install, upgrade and rollback](https://neca.nekobiglazyer.work/en/#/install) | Docker/binary install, upgrade, rollback |
| 🛠️ [Development, license and feedback (Chinese)](https://neca.nekobiglazyer.work/zh/#/development) | Local build, architecture, release flow, contribution (Chinese-only) |
| 🤖 [AI disclosure](AI-DISCLOSURE_EN.md) | AI scope and acceptance boundaries |

## 🛠️ Development

~~~bash
docker compose run -T --rm dev make all      # format + vet + docs + tests + cross-compile
docker compose run -T --rm dev make shadow   # old-vs-new shadow comparison
docker compose run -T --rm dev make test     # go test -race ./...
~~~

Common checks: <code>make check-docs</code>, <code>make check-locales</code>, <code>make check-ascii</code>. See the [development page](https://neca.nekobiglazyer.work/en/#/development) for architecture and release details.

## 📌 Status

- **Current stable**: v1.2.0 (released 2026-11-27)
- **Completed**: M0–M12; v1.2.0 (CLI shell completion, certificate-chain verification and documentation consistency)
- **Next**: v1.3.0 certificate-import preview; v1.4.0 cert+key import; v1.5.0 config-schema and Passkey evaluation
- **Boundaries**: Passkey has configuration and reserved interfaces only (no login routes yet); the Web API is experimental

## ⚖️ License and acknowledgements

**AGPL-3.0-only**: commercial use is allowed, but distribution or network services must publish the corresponding source under AGPL; keep the original author, repository link and license. See [LICENSE](LICENSE) and the [secondary-development policy](docs/SECONDARY-DEVELOPMENT.md).

This project builds on substantial upstream work. Special thanks to:

- **Go** standard library and toolchain, plus the `golang.org/x` dependencies;
- **strongSwan** and the IKEv2 community for certificate, CRL and VPN practice;
- **OpenSSL** for certificate, key and PKCS#12 operations;
- **Docker / Buildx / QEMU** for multi-architecture images and reproducible development;
- **GitHub Actions** for CI, release and image pipelines;
- **Docsify** and help-site tooling, and everyone who filed issues, tested or suggested improvements.

Upstream licenses and copyrights remain with their respective authors; this project retains only the necessary notices.
