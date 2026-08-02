<!-- Modified by UnAPI'd contributors from the original Relmio project file. -->

# UnAPI'd development rules

## Product boundary

UnAPI'd is a terminal-only, server-local Go tool. Do not add a browser UI, SSH
transport, desktop launcher, telemetry, or hosted control plane.

## Runtime boundary

- Never edit, rebuild, recreate, stop, or restart n8n.
- Never publish the API runtime port on the host.
- Manage only the Compose project and files owned by UnAPI'd.
- Use Docker argument arrays. Never pass discovered values through `sh -c`.
- Refuse unmarked runtime directories and reserved networks.
- Keep authentication files and generated keys out of build contexts and logs.
- Roll back staged runtime and newly added network state after any failed check.
- Preserve accurate license and provenance notices.

## Verification

Run `make check` after source changes and `make release` before publishing.
Exercise migration and bridge-network behavior with isolated Docker fixtures.
Confirm that n8n's start timestamp is unchanged after live verification.
