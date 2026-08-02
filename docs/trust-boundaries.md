# Trust boundaries

- Setup must run as root because it manages Docker and protected runtime paths.
- ChatGPT credentials remain under `/var/lib/unapid/state` and use mode `0600`.
- Credential directories are excluded from every Docker build context.
- The OAuth translator is not connected to n8n or any host-facing network; its
  separate project-owned network supplies outbound internet access.
- The API service is connected to n8n but has no published host port.
- The generated API key is removed before requests reach the translator.
- Docker is executed directly with structured arguments; no user-controlled
  value is evaluated by a shell.
- Existing n8n images, Compose files, containers, volumes, and workflows are not
  rebuilt, stopped, restarted, or recreated.
- A managed network can be attached to a bridge-only n8n container without a
  restart. New attachments are rolled back if deployment fails.
- Unmarked runtime directories and reserved Docker networks are refused.

The ChatGPT credential and generated API key are secrets. Never commit, log,
paste, or expose the files that hold them.
