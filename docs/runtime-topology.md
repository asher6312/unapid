# Runtime topology

UnAPI'd 2 is a server-local Go program with two distinct execution roles.

The host CLI discovers n8n, performs the isolated Codex device login, and
reconciles a dedicated Compose project. It invokes Docker with argument arrays;
it does not construct shell commands or edit the n8n deployment.

The managed runtime has two containers:

1. `translator` runs the pinned `openai-oauth` package. It reaches the API over
   the internal `backplane` and uses a project-owned `egress` network for
   outbound ChatGPT traffic; it never joins the n8n network.
2. `api` is the same static UnAPI'd binary running in gateway mode. It joins the
   backplane and n8n networks, validates the generated bearer key using a
   constant-time comparison, strips that key, and streams the request to the
   translator.

Runtime configuration is rendered under `/var/lib/unapid/runtime`; the writable
OAuth cache and API key live separately under `/var/lib/unapid/state`. Updates
therefore reuse the same live token file while configuration is assembled in a
sibling staging directory and activated with a directory rename. On any
build, health, publication, or authenticated model-check failure, UnAPI'd stops
the failed runtime, restores the previous directory, and rolls back a newly
created Docker-network attachment.

No runtime port is published on the server. n8n reaches the API through the
Docker-only address `http://subscription-api-gateway:8317/v1`.
