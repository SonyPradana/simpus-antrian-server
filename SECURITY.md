# Security Policy

## Supported versions

Only the latest release on `main` receives security fixes. Older tags are not patched.

## Reporting a vulnerability

Do not open a public issue for a security problem.

1. Open the **Security** tab of this repository.
2. Click **Report a vulnerability** to start a private advisory.

Please include the affected version or commit, the steps to reproduce, and the impact you expect. If the report is valid, a fix is prepared in a private fork and published together with the advisory.

## What to expect

- Acknowledgement as soon as possible.
- A status update once the report is triaged.
- Credit in the advisory unless you ask to stay anonymous.

## Scope

This policy covers the code in this repository only. Out of scope:

- The SIMPUS database, the BPJS/PCare service, and any other third party system.
- Deployments configured by third parties, including their network setup, TLS, and API key handling.
- Findings that require a pre-shared secret or physical access.

## Deployment checklist

This service talks to a production SIMPUS database containing patient data. Before exposing it beyond a trusted network:

1. Set `APM_API_KEY` to a long random value and send it only over TLS.
2. Never commit `.env`, `config.json`, or `koneksi.jjk`. They are ignored by git for a reason.
3. Set `CORS_ALLOWED_ORIGIN` only to the exact origin of a browser client, never `*`.
4. Restrict database and PCare credentials to the minimum permissions the service needs.
5. Keep the binary up to date; releases are built from tagged commits by CI.

## Secrets in this repository

No secret is committed. `.env`, `config.json`, and `koneksi.jjk` are ignored, and only `.env.example` and `config.example.json` with dummy values are tracked. If you believe a real credential was committed, report it privately using the steps above.
