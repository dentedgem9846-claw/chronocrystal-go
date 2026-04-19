# Skills Guide

Skills inject specialized knowledge into ChronoCrystal's system prompt when trigger keywords match the user's message. This guide covers the skill format, discovery, matching, and creation.

## What Are Skills?

A skill is a markdown file with YAML frontmatter. When a user's message contains words that match a skill's trigger keywords, the skill's content is appended to the system prompt under a "Relevant Knowledge" section.

This gives the LLM domain-specific instructions without permanently inflating the context window. Skills are matched per-message — only relevant knowledge is injected for each request.

Example skill file (`tools/git.md`) — placed alongside tool subdirectories:

```markdown
---
name: git
description: Git version control operations
trigger_keywords: [git, commit, branch, merge, rebase, stash, diff, log]
---

## Git Commands

When working with git repositories:

1. Always check `git status` before making changes
2. Use `git diff` to review changes before committing
3. Prefer `git stash` over `git commit` for temporary saves
4. Never force-push to shared branches

Common workflows:
- Create a feature branch: `git checkout -b feature/name`
- Stage all changes: `git add -A`
- Commit with message: `git commit -m "descriptive message"`
```

## How Skills Are Discovered

On startup, ChronoCrystal scans the skills directory for `.md` files. The path is configured by `tools.dir` in `config.toml` (default `./tools`). Skill markdown files are discovered alongside tool subdirectories:

```go
skillReg := skills.NewRegistry(cfg.Tools.Dir)
```

The `Discover` method iterates over entries in that directory, skipping subdirectories (which contain tool programs) and non-`.md` files. Each markdown file is parsed for YAML frontmatter. Files without valid frontmatter are skipped silently.

To add a skill, place a `.md` file with valid frontmatter in the configured directory and restart ChronoCrystal.

## How Skills Are Matched

When a user message arrives, the context builder calls `skills.Registry.InstructionsFor(message)`:

1. `Match(message)` checks each skill's trigger keywords against the lowercased message
2. If any keyword appears as a substring of the message, the skill is matched
3. `InstructionsFor` formats all matched skills as:

```
## Relevant Knowledge

## Skill: git
Git version control operations
[skill content here]

## Skill: docker
...
```

This formatted string is appended to the system prompt after the base identity prompt.

### Matching Rules

- Matching is case-insensitive
- Keyword matching uses `strings.Contains` — a keyword matches if it appears anywhere in the message
- A single message can match multiple skills; all matched skills are injected
- If no skills match, no additional content is injected

## How to Create a New Skill

### Step 1: Create a Markdown File

Create a `.md` file in the skills directory:

```bash
touch tools/docker.md
```

### Step 2: Add Frontmatter and Content

```markdown
---
name: docker
description: Docker container management and operations
trigger_keywords: [docker, container, image, compose, volume, network]
---

## Docker Operations

When working with Docker:

- Use `docker compose` for multi-container applications
- Check running containers with `docker ps`
- View logs with `docker logs <container>`
- Never use `docker system prune -a` without confirmation

### Common Commands

- Build: `docker build -t name .`
- Run: `docker run -d -p 8080:80 name`
- Stop: `docker stop <container>`
```

### Step 3: Verify

Restart ChronoCrystal and send a message containing a trigger keyword:

```
You: Help me manage my docker containers
```

The LLM will receive the docker skill content in its system prompt.

## Frontmatter Format

The YAML frontmatter is parsed as simple `key: value` pairs. No complex YAML nesting is supported — the parser uses a basic key-value split.

```yaml
---
name: string              # Required — skill identifier
description: string        # Required — one-line description
trigger_keywords: [kw1, kw2, kw3]  # Required — list of trigger words
---
```

- `name` — used in the injected header (`## Skill: <name>`)
- `description` — included below the header
- `trigger_keywords` — bracket-comma list of strings; any match injects the skill

## Skill Injection in the System Prompt

The base system prompt defines ChronoCrystal's identity as a time dragon. Skill content is appended after a "Relevant Knowledge" section:

```
You are ChronoCrystal, an ancient time dragon...

## Relevant Knowledge

## Skill: git
Git version control operations
[content]

## Skill: docker
Docker container management
[content]
```

This means:
- Skills don't replace the dragon identity — they augment it
- Multiple skills can be active simultaneously
- The LLM sees skill content as knowledge, not commands
- Skill content counts toward the token budget, so keep it concise

## Tips for Effective Skills

- **Keep them short**: Skill content is injected into every relevant conversation turn. Long skills consume the token budget.
- **Use specific keywords**: `kubernetes` is better than `kube` to avoid false matches.
- **Write instructions, not descriptions**: "Always check git status before committing" is more useful than "Git is a version control system."
- **One topic per skill**: A "docker" skill should not contain git instructions. Split them.
- **Test matching**: Send messages with and without your trigger keywords to verify injection.