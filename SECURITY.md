# Security Policy

## Supported Versions

| Version  | Supported |
| -------- | --------- |
| latest   | ✅        |
| < latest | ❌        |

## Reporting a Vulnerability

**Do not open a public issue for security problems.**

Report privately via [GitHub Security Advisories](https://github.com/fabiocicerchia/aws-killswitch/security/advisories/new).

Please include a description, reproduction steps, and impact. We aim to
acknowledge within 48 hours and to ship a fix or mitigation as soon as
practical, keeping you updated along the way.

If you get no acknowledgement within 5 working days, escalate by opening a
public issue that says only "unacknowledged security report" — no details.

## Response process

1. **Triage** (within 48h) — confirm, reproduce, and rate severity.
1. **Fix** — developed on a private fork, with a regression test.
1. **Disclose** — advisory published with a patched release and a CVE where
   one applies. Reporters are credited unless they ask not to be.

Targets are 7 days to a fix for critical issues (remote impact on an AWS
account) and 30 days for everything else.

## Verifying a release

Every release ships a CycloneDX SBOM and a keyless Sigstore provenance
attestation built by the `release` workflow:

```bash
gh attestation verify aws-killswitch-linux-amd64 --repo fabiocicerchia/aws-killswitch
sha256sum -c checksums.txt
```
