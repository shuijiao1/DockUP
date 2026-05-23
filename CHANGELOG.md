# Changelog

## v0.6.4

- Improved manual “check all” feedback to show how many local containers and remote VPS were checked, how many updates were found, and whether any checks failed.
- Added running image version display to Docker and remote Docker detail pages, e.g. `v0.6.4 (abcdef123456)` when image labels or semantic tags are available.

## v0.6.3

- Fixed update detection after manually checking `latest` images. DockUP now compares the running container image ID against the pulled tag image ID, so repeated checks still show an update until the container is actually recreated.

## v0.6.2

- Added manual update checks to remote VPS pages and remote project pages.
- Remote manual checks now reuse the existing update confirmation flow, so updates can be applied from the remote check result.

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
