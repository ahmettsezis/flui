# Security Policy

## Supported versions

Only the latest minor release receives security fixes.

## Reporting a vulnerability

**Do not open a public issue.**

Report privately via GitHub: **Security → Report a vulnerability** on
<https://github.com/ahmettsezis/flui/security/advisories/new>.

Please include:

- affected version and component (`flui-server`, `flui-agent`, UI),
- steps to reproduce or a proof of concept,
- impact as you understand it.

## What to expect

| Step | Target |
|---|---|
| Acknowledgement | within 3 business days |
| Initial assessment | within 10 business days |
| Fix and advisory | depends on severity; critical issues are prioritised |

We will credit you in the advisory unless you prefer to stay anonymous.

## Verifying releases

Release binaries come with `checksums.txt`. Container images and checksums are signed with
[cosign](https://github.com/sigstore/cosign) (keyless) and ship with an SBOM:

```sh
cosign verify ghcr.io/ahmettsezis/flui:<version> \
  --certificate-identity-regexp 'https://github.com/ahmettsezis/flui/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

See [docs/security.md](docs/security.md) for the security model.
