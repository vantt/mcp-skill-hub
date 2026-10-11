# Skill Hub documentation

Find the page you need, grouped by what you want to do.

## Start here

- [Get started](getting-started.md): build your own skill library and see the hub recommend a skill in about 10 minutes.
- [Installation](installation.md): install `skillhub` on Linux, macOS or Windows and verify its checksum.

## Guides

- [Using Skill Hub](user-guide.md): use Skill Hub day to day after setup.
- [Curating skills](curating-skills.md): add, create, review, edit and change the state of the skills in your collection.
- [Sources and learning](sources-and-learning.md): keep copied skills up to date and learn from what others publish.
- [Web UI](web-ui.md): look after your skills by clicking instead of typing, in your own browser.
- [Troubleshooting](troubleshooting.md): find the symptom you see and run the fix below it.

## Reference

- [Command reference](command-reference.md): find the command for a task; `skillhub help <command>` has every flag.
- [Web API](contracts/web-api.md): the HTTP API behind the web UI.
- [Error codes](contracts/error-codes.md): the stable codes behind every error and how they are shown.
- [Result envelope](contracts/result-envelope.md): the result shape shared by commands and agent tools.
- [UX fixtures](contracts/ux-fixtures.md): fixed acceptance inputs for the curation experience and its CLI fallback.
- [Compatibility matrix](mcp-compatibility-matrix.json): which MCP clients and protocol revisions were checked, and which are not yet verified.
- [Resolve request schema](../schemas/skill-resolve-request-v1.schema.json): the JSON Schema for a skill resolve request. Other schemas live in [`schemas/`](../schemas/).

## For maintainers

Most design records below (design, use cases, plans) are written in Vietnamese.

- [Contributing](../CONTRIBUTING.md): build, test and release Skill Hub.
- [Release runbook](release-runbook.md): the release process, supported platforms and recovery paths.
- [Product requirements](PRD.md): the original product requirements document.
- [Design](design/): system architecture, the agent protocol, the resolver, curation and storage.
- [Use cases](use-cases/): the agent runtime protocol, core curation use cases and web UI flows.
- [Distillery](distillery/): lessons from other projects, scored against the Skill Hub goal and grouped by domain.
- [Brainstorm](brainstorm/): early architecture review reports.
- [Plans](plans/): open maintainer plans and lead handoffs.
- [References](references/): skills copied from other projects, with notes on what was learned from them.
