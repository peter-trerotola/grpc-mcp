# Security Policy

## Reporting Vulnerabilities

If you discover a security vulnerability in grpc-mcp, please report it responsibly:

**GitHub Security Advisory** (preferred): [Create a private security advisory](https://github.com/peter-trerotola/grpc-mcp/security/advisories/new)

**Do not open a public issue for security vulnerabilities.**

Please include:
- Description of the vulnerability
- Steps to reproduce
- Impact assessment

## Security Model

| Layer | Mechanism | What it protects |
|-------|-----------|------------------|
| Transport auth | Bearer token, API key, mTLS | Authenticates to upstream gRPC services |
| TLS | TLS / mTLS connections | Encrypts traffic to gRPC endpoints |
| Credential handling | Environment variable expansion (`${VAR}`) | Keeps secrets out of config files |
| Config file protection | `.gitignore` patterns for `grpc-mcp.yaml` | Prevents accidental secret commits |

## Supported Versions

Security fixes are applied to the latest release only. We recommend always running the latest version.
