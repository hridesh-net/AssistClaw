# Telegram Channel Setup Guide

This guide helps you configure AssistClaw's Telegram channel reliably for DM and group usage.

## 1) Create bot token

1. Open Telegram and chat with `@BotFather`.
2. Run `/newbot` and complete prompts.
3. Copy the bot token.

## 2) Configure AssistClaw

In `assistclaw.yaml`:

```yaml
channels:
  telegram:
    bot_token: "123456:ABC..."
    dm_mode: "pairing"        # pairing | allowlist | open | disabled
    require_mention: true     # group chats require @bot mention (recommended)
    allow_from:
      - "123456789"           # optional for allowlist mode
```

## 3) Understand modes

- `pairing`: recommended default while onboarding trusted users.
- `allowlist`: only IDs/usernames in `allow_from` can message.
- `open`: anyone can message.
- `disabled`: Telegram channel is off.

## 4) Group behavior

With `require_mention: true`, AssistClaw replies in groups when:

- message includes `@<bot_username>`, or
- someone replies directly to the bot's last message.

This keeps group chats low-noise and intentional.

## 5) BotFather privacy setting

Telegram bots in groups are affected by Privacy Mode:

- Keep privacy enabled for mention-only behavior.
- Disable privacy if you want the bot to see all group messages.

If you change privacy mode, remove and re-add the bot in target groups.

## 6) Start and verify

1. Start AssistClaw: `assistclaw start`
2. DM the bot and send `/status`
3. In a group, mention the bot (`@botname`) and test one prompt.

## 7) Troubleshooting

- Run `assistclaw doctor --fix` for token/config checks.
- If no replies in groups:
  - confirm bot is in the group,
  - confirm mention includes exact bot username,
  - check BotFather privacy mode,
  - verify `require_mention` value in config.
