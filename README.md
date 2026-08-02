# UnAPI'd

UnAPI'd provides a Docker-private, OpenAI-compatible endpoint for an existing
n8n installation using a ChatGPT subscription sign-in. It is a terminal-only
tool for administrators already logged in to the Docker host.

## Install and run

One command line:

```bash
curl -fsSL https://asherflora.com/unapid/install.sh | sudo sh && sudo unapid
```

UnAPI'd asks before using or creating its isolated ChatGPT sign-in. It then
discovers the running official n8n Docker container, selects a compatible
network, deploys the private API runtime, verifies it, and prints the Base URL
and generated API key for n8n.

Requirements: Linux x86_64 or ARM64, Docker Engine with Docker Compose, one
running `n8nio/n8n` container, and Codex CLI for device-code sign-in. Podman
Docker emulation is not supported.

## n8n credential

Use the values printed after setup. The internal Base URL is:

```text
http://subscription-api-gateway:8317/v1
```

The endpoint is available only inside the selected Docker network; port `8317`
is not published on the host. UnAPI'd does not edit or restart n8n.

Check the installation with:

```bash
sudo unapid status
```

## Credits and license

UnAPI'd originated as a server-local derivative of
[Relmio](https://github.com/Demonbane18/relmio), Copyright 2026 Relmio
contributors. UnAPI'd 2.1 substantially rewrites the implementation in Go for a
server-local terminal workflow while preserving Relmio's copyright and complete
upstream notice.

The OAuth translator uses
[openai-oauth](https://github.com/EvanZhouDev/openai-oauth), developed by Evan
Zhou and OpenAI OAuth contributors.

UnAPI'd, Relmio, and openai-oauth are distributed under the Apache License 2.0.
See [LICENSE](LICENSE), [NOTICE](NOTICE), and [UPSTREAM.md](UPSTREAM.md) for the
license, retained notices, and modification record.

UnAPI'd is an unofficial personal-use project. It is not affiliated with or
endorsed by OpenAI or n8n and does not create an OpenAI Platform API key.
