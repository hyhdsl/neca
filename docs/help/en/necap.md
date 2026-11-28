# necap authentication component (v1.7 development)

> This page applies to the v1.7 necap component. It is still a development build; no production release or deployment has happened.

necap is an optional, separate authentication service for passwords, TOTP, Passkeys, recovery codes and second-factor policy. The Web panel is only the UI/session layer. Authentication requests go through the helper over a Unix socket. necap has no TCP listener and receives no CA key, private keys or docker.sock.

## Supported flows

- Password only
- Password + TOTP
- Password + Passkey
- Password + either enrolled second factor
- TOTP-only and Passkey-only passwordless login are not supported in v1.7.

## Prerequisites

- A working Web panel and web-helper.
- An existing bcrypt admin password hash, by default /opt/ipsec/web-secrets/password.hash.
- RP ID and Origin, for example neca.example.com and https://neca.example.com.
- /opt/ipsec/necap and /run/necap owned by UID/GID 10002.

~~~bash
install -d -m 0700 -o 10002 -g 10002 /opt/ipsec/necap
install -d -m 0700 -o 10002 -g 10002 /run/necap
~~~

Generate a password hash if needed:

~~~bash
neca web hash-password > /opt/ipsec/web-secrets/password.hash
chmod 600 /opt/ipsec/web-secrets/password.hash
~~~

## Initialize

Run a one-off initialization as root; the resulting files are chowned to 10002:

~~~bash
docker compose -f /opt/ipsec/docker-compose.neca.yml --profile necap run --rm   --user 0:0 --entrypoint /usr/local/bin/necap necap init   --config /opt/ipsec/necap/necap.json   --rp-id neca.example.com   --rp-name "neca Admin"   --origins https://neca.example.com   --uid 10002 --gid 10002
~~~

This creates necap.json, necap.db and master.key with mode 0600. Keep master.key separately backed up; losing it makes stored TOTP secrets unreadable.

## Enable

Add to /opt/ipsec/neca.env:

~~~text
NECA_NECAP_ENABLED=1
NECA_NECAP_USERNAME=admin
~~~

Start the services:

~~~bash
docker compose -f /opt/ipsec/docker-compose.neca.yml   --profile web --profile web-write --profile necap   up -d web web-helper necap
~~~

Login uses the password, then TOTP, Passkey or a recovery code. A full Web session is issued only after the second factor succeeds.

In Settings → Security you can re-authenticate, enroll/remove TOTP, register/list/remove Passkeys, regenerate recovery codes and change the second-factor policy. Settings re-authentication uses password + TOTP/recovery code; Passkey users can use a recovery code.

## CLI

After root/helper access and re-authentication:

~~~bash
neca pass status
neca pass totp add
neca pass totp remove
neca pass passkey list
neca pass passkey remove <ID>
neca pass recovery regenerate
neca pass policy <password|totp|passkey|any>
~~~

Passkey enrollment is Web-only. The CLI can list and remove credentials. CLI re-authentication requires the password and an existing TOTP/recovery code; Passkey assertion is not available in the CLI.

## Rollback

1. Remove NECA_NECAP_ENABLED from /opt/ipsec/neca.env.
2. Stop the necap container.
3. Restart web and web-helper.
4. Keep or back up /opt/ipsec/necap. No neca.json or password-hash changes are required.

Rollback returns the Web panel to the legacy password hash. necap data is not deleted automatically.

## Boundaries

- necap has no TCP listener, CA key, private keys or docker.sock.
- necap.db and master.key must be 0600; the directory must be 0700.
- Recovery codes are stored only as hashes and shown once.
- Removing the only factor required by the active policy is refused.
- v1.7 has no passwordless login and no multi-user Web UI.
- Verified: a real necap+helper+Web password-to-TOTP HTTP E2E, plus synthetic-authenticator Passkey registration/assertion tests.
- Real-browser Passkey flow and pre-release security review remain.
