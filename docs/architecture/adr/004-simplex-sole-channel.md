# ADR-004: SimpleX Chat as Sole Channel

**Status:** Accepted  
**Date:** 2026-04-19  
**Context:** ChronoCrystal needs a messaging channel for the operator to communicate with the agent. TEMM1E supports 5 channels (Telegram, Discord, WhatsApp, Slack, CLI), but ChronoCrystal targets privacy-focused single-user deployment.

**Decision:** SimpleX Chat is the only messaging channel. ChronoCrystal manages the `simplex-chat` CLI subprocess, communicating via its JSON bot protocol over stdin/stdout.

SimpleX Chat provides: end-to-end encryption, no phone number or email required, no central server, and a well-documented bot API with event-driven architecture.

**Alternatives Considered:**

1. **Multi-channel abstraction** — The `Channel` interface could support multiple implementations. Adds complexity, testing surface, and the operator only uses one channel anyway.
2. **Telegram** — Most mature bot API, but requires phone number and central server. Conflicts with privacy focus.
3. **Discord** — Requires server creation, not designed for 1:1 agent communication.
4. **CLI only** — No encryption, no mobile access, no persistence across sessions. SimpleX gives us a proper chat UX for free.

**Consequences:**

- **Positive:** E2E encryption by default. No external account required. The bot protocol gives us structured events (NewChatItems, ContactConnected) and commands (/_send, /_address). ChronoCrystal manages the subprocess lifecycle entirely — the operator just runs `chronocrystal start`.
- **Negative:** Tied to SimpleX Chat's protocol. If they change the bot API, we must adapt. The subprocess model means we're responsible for lifecycle management (start, restart, shutdown). SimpleX Chat binary must be installed separately.
- **Future:** Adding channels means implementing the `Channel` interface. The abstraction is already there — it's a thin boundary.