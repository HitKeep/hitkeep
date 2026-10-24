# Official HitKeep Analytics Skills

This directory is the canonical product analytics skill pack. Its five skills are published for external assistants, and their transport-neutral procedure references are compiled into HitKeep Ask AI.

The pack contains:

- `hitkeep-analytics`: broad analytics routing, scope, evidence, and privacy judgment.
- `hitkeep-traffic-diagnosis`: traffic drops, spikes, source shifts, and measurement checks.
- `hitkeep-ai-visibility-analyst`: crawler fetches, AI referrals, correlation, and failure analysis.
- `hitkeep-ecommerce-analyst`: revenue, products, acquisition quality, and conversion movement.
- `hitkeep-tracking-verifier`: installation and automatic-event verification.

Each `SKILL.md` is a thin external-agent adapter for production HitKeep MCP. Each `references/procedure.md` contains the transport-neutral reasoning shared with Ask AI. Ask AI uses its internal analytics tool bridge, not MCP, and never embeds contributor skills.

## Connect an assistant

Open **Integration → Assistants** (`/integration/assistants`) in HitKeep to copy the canonical MCP endpoint and find setup links. Approve only the MCP resource, `sites:read analytics:read`, and the sites you intend to share. Review or revoke your OAuth grant under **Settings → Connected applications**. The configuration display does not verify a working client connection; existing scoped API-client tokens remain available for compatible clients.

Skill installation supplies instructions; a custom MCP connection supplies access. Optional plugins can package the same five skills, but Claude and OpenAI use different manifest and connection formats. Offline fixture metadata and the prepared review cases live in [`tests/agent-plugins`](../tests/agent-plugins/README.md); they are not complete installable packages or a public listing. Structural tests stage transient regular-file bundles from this canonical pack, README, and LICENSE without maintaining a second copy of the skills. Real account installation and assistant behavior remain unverified.

## Install

List only the analytics pack:

```bash
npx skills add https://github.com/HitKeep/hitkeep/tree/main/skills --list
```

Install the complete analytics pack for Codex:

```bash
npx skills add https://github.com/HitKeep/hitkeep/tree/main/skills --skill '*' --agent codex --copy -y
```

Install one specialist by replacing the skill name:

```bash
npx skills add https://github.com/HitKeep/hitkeep/tree/main/skills --skill hitkeep-traffic-diagnosis --agent codex --copy -y
```

For live analytics, pair the installed skill with HitKeep's scoped, read-only production MCP endpoint. Skills contain no credentials or customer data. OAuth-capable clients can use an explicit MCP-resource grant with `sites:read analytics:read` and selected sites; existing API-client bearer tokens remain supported. Authentication does not broaden the analytics procedure or privacy boundary.

Contributor skills for changing HitKeep itself are canonical under `.agents/skills` and documented in `CONTRIBUTING.md`. Do not install the repository root with `--skill '*'` when only one audience is intended.
