# Channel Capability Registry

This table is the single source of truth for built-in channel capability behavior.

Runtime API:

- `adapter.CapabilitiesFor(channelType)`
- `adapter.RegisterCapabilities(channelType, caps)` for extensions/new connectors

| Channel | Threading | Attachments | DM | Group | Mentions | Voice | Reactions | Edits | Max Message Length |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| telegram | yes | yes | yes | yes | yes | no | yes | yes | 4096 |
| discord | yes | yes | yes | yes | yes | yes | yes | yes | 2000 |
| slack | yes | yes | yes | yes | yes | no | yes | yes | 40000 |
| whatsapp | no | yes | yes | yes | yes | yes | yes | no | 4000 |

## Usage notes

- Send paths should check capabilities before optional operations (e.g. thread replies).
- Current outbound reliability layer degrades gracefully when threading is unsupported.
- Keep this file updated when adding a new built-in channel or changing behavior.
