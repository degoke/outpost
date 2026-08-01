---
title: ai
slug: commands/ai
section: commands
order: 3
---

# outpost ai

Run an AI coding agent in the project's managed development environment. Your local terminal drives the session; the agent runs remotely with your synced project files.

## Usage

```bash
outpost ai
outpost ai claude
outpost ai --command "opencode --model anthropic/claude-sonnet-4-6"
outpost ai --no-pull
```

## What happens

1. Syncs the repository to the remote host (if changed)
2. Ensures the managed development container is running
3. Starts an interactive AI agent (`opencode`, `claude`, or `codex` by default)
4. Keeps your local edits synchronized to the remote host while the session is open
5. Pulls agent-made file changes back to your machine when you exit

## Configuration

Set a default agent in `.outpost/project.yaml`:

```yaml
ai:
  command: claude
```

## Notes

- **Owner only** on shared hosts.
- Requires a managed development environment (`environment.enabled: true`, the default).
- Install your preferred agent inside the remote environment image, or set `ai.command` to a custom command.
- Use `--no-pull` to skip downloading remote file changes after the session.

## Related

- [shell](shell) · [run](run) · [projects](../projects)
