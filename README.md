# devincord

Discord bot that starts [Devin](https://devin.ai) sessions and relays them into Discord threads (Go + disgo).

- `/devin start prompt:<task> [title]` creates a Devin session and opens a thread for it.
- Devin's replies, new pull requests and status changes (waiting for you, finished, asleep, ended) are posted in the thread.
- Messages you write in the thread (plus attachment links) are sent to the session; a sleeping session wakes up.
- `/devin status` lists your recent sessions.

Only users in `ALLOWED_USER_IDS` can start or message sessions, since sessions spend your ACUs.

## Setup

1. **Discord bot**: create an application at https://discord.com/developers/applications, add a bot, copy its token,
   and enable **Message Content Intent**. Invite it with the `bot` and `applications.commands` scopes and the
   permissions *Send Messages*, *Create Public Threads*, *Send Messages in Threads*, *Read Message History* and *Add Reactions*.
2. **Devin token**: create a personal access token (`cog_...`) under Settings → Devin API → PATs. Sessions are
   attributed to you. Your org ID (`org-...`) is in the same settings page.
3. Run it:

```bash
cp .env.example .env   # fill in DISCORD_TOKEN, DEVIN_API_KEY, DEVIN_ORG_ID, ALLOWED_USER_IDS
go run .
```

Set `DISCORD_GUILD_ID` while developing so slash commands sync instantly to one server.

Thread ↔ session links are stored in `STORE_PATH` (default `devincord.json`), so relaying resumes after restarts.
Active sessions are polled every `POLL_INTERVAL` (default 15s); polling stops when a session sleeps or ends
and restarts when someone replies in the thread.

## Deployment

Merges to `main` deploy automatically: `.github/workflows/deploy.yml` waits for the `test` workflow, then uploads
the tested commit to Discloud with `discloud/deploy-action`. Configure these repository secrets first
(Settings → Secrets and variables → Actions):

| Secret | Value |
| --- | --- |
| `DISCLOUD_TOKEN` | Discloud account token (Dashboard → Tools → API keys). |
| `DISCLOUD_APP_ID` | The devincord application ID shown in the Discloud dashboard. |
| `DISCLOUD_ENV` | Full contents of the production `.env`. |

Without the secrets the deploy fails before uploading and names what is missing.
