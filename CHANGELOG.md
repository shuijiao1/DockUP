# Changelog

## v0.6.1

- Fixed preconfigured reverse Agents using `DOCKUP_AGENTS` with an empty URL, e.g. `id||Name|token`.
- Improved Telegram server onboarding so sending an IP address or Agent URL as the first reply directly generates the Agent install command with a sensible default name.

## v0.6.0

- Added a remote VPS removal flow in Telegram.
  - The bot shows a fixed Agent uninstall command for the user to run on the remote host.
  - The server removes the local record only after the Agent is offline.
  - A force-remove option is available for already-destroyed remote hosts.
- Added automatic `DOCKUP_DATA` backups before removing remote VPS records.
- Added Telegram command and callback authorization against `TG_CHAT_ID`.
- Improved common user-facing error messages for offline Agents, network timeouts, auth failures, missing images, Docker permission issues, and stale projects.
- Updated Chinese and English README documentation for the safer remote VPS deletion behavior.

## v0.5.0

- Added reverse Agent VPS management.
