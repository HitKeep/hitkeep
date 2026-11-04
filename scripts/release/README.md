# MCP registry publication

`server.json` identifies the organization registry entry as
`io.github.HitKeep/hitkeep`. GitHub Actions OIDC grants the running repository
owner's namespace, so publication from `HitKeep/hitkeep` can use that identity
without a personal token or additional secret. The workflow checks the selected
release tag's namespace before schema validation and authenticates only in the
publication job. Passing either source check does not prove publication succeeded.

This is a new registry entry, not a transfer or redirect of
`io.github.PascaleBeier/hitkeep`. The legacy entry remains available; this workflow
does not update, deprecate, or delete it. The runtime MCP implementation continues
to report `io.github.PascaleBeier/hitkeep` for compatibility. Existing `/mcp`
connection URLs, host substitution, and authorization requirements are unchanged.
Clients connecting directly need no configuration change. Registry-based discovery
and saved registry references can use the organization entry after its first
successful publication.

Maintainers must review the public identity change before merging and publishing.
Publish a new stable release containing this metadata through the existing
workflow, then read back the exact version under the organization name in the
official registry. Do not move existing tags or retry old tags expecting their
metadata to change: validation and publication both read the selected tag's source.
Update public registry/discovery documentation after verifying the new entry;
do not describe the legacy entry as redirected or migrated automatically.

The [official authentication rules](https://modelcontextprotocol.io/registry/authentication)
and [GitHub Actions publishing guide](https://modelcontextprotocol.io/registry/github-actions)
define namespace ownership. Registry metadata and protocol implementation names
serve separate purposes; this delivery change does not rename the runtime server.
