# Discord Channel Setup Guide

This guide helps you set up AssistClaw's Discord integration with adapter reliability, mention-gating, and clear verification steps.

## 1) Create a Discord bot

1. Open the [Discord Developer Portal](https://discord.com/developers/applications).
2. Create an application, then open the **Bot** tab.
3. Create/reset token and copy it.

## 2) Enable required intents

In the **Bot** tab:

- Enable **Message Content Intent** (required for reading message text).
- Keep other intents minimal unless your deployment needs them.

## 3) Invite bot to your server

Generate an OAuth2 URL with scopes:

- `bot`

Permissions (recommended minimum):

- View Channels
- Send Messages
- Read Message History
- Add Reactions

## 4) Configure AssistClaw

In `assistclaw.yaml`:

```yaml
channels:
  discord:
    bot_token: "YOUR_DISCORD_BOT_TOKEN"
    dm_mode: "pairing"         # pairing | allowlist | open | disabled
    require_mention: true      # guild channels require @bot mention (recommended)
    allow_from:
      - "123456789012345678"   # optional for allowlist mode
```

## 5) Understand modes

- `pairing`: onboarding-first behavior for trusted contacts.
- `allowlist`: only user IDs listed in `allow_from`.
- `open`: all users can message.
- `disabled`: Discord channel is disabled.

## 6) Guild behavior (`require_mention`)

With `require_mention: true`, AssistClaw only processes guild messages when:

- the message contains a direct bot mention (`@BotName`), or
- the message replies to a bot message.

DMs are not affected by this setting.

## 7) Start and verify

1. Run `assistclaw start`
2. Run `assistclaw doctor --fix`
3. In Discord DM: send a prompt and confirm a response.
4. In server channel: mention the bot and confirm a response.

## 8) Troubleshooting

- `doctor` fails on Discord: re-check token and bot intents.
- Bot appears online but does not reply in server:
  - confirm **Message Content Intent** is enabled,
  - mention the bot if `require_mention: true`,
  - verify channel permissions.
- If messages fail intermittently, inspect outbound retry/DLQ:
  - `assistclaw dlq list`
