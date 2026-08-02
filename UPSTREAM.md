# Upstream and modification record

UnAPI'd originated as a server-local derivative of
[Relmio](https://github.com/Demonbane18/relmio), Copyright 2026 Relmio
contributors. Relmio is licensed under the Apache License, Version 2.0.

## Current modification status

UnAPI'd 2.1 is a substantial rewrite for a different execution model:

- Relmio's Node.js browser wizard, web UI, SSH/SFTP transport, desktop launchers,
  npm packaging, and JavaScript domain/service layers are not present.
- `cmd/unapid/main.go` and every package under `internal/` are new Go source files
  written for UnAPI'd's server-local runtime.
- `packaging/install.sh.in` replaces the upstream workstation bootstrap with a
  checksum-pinned static-binary installer and carries a modification notice.
- `README.md`, `.gitignore`, `AGENTS.md`, `NOTICE`, and the Apache license
  application notice were changed by UnAPI'd contributors and carry or refer to
  prominent modification notices.
- The former documentation was replaced by `docs/runtime-topology.md` and
  `docs/trust-boundaries.md`.

The product lineage, Relmio copyright, and complete attribution text from
Relmio's `NOTICE` remain visible in this repository. The Apache-2.0 license text
is distributed in `LICENSE`.

## Additional upstream component

The runtime installs
[openai-oauth](https://github.com/EvanZhouDev/openai-oauth), developed by Evan
Zhou and OpenAI OAuth contributors and licensed under Apache-2.0. Its attribution
is retained in `NOTICE`.
