# ding

A tiny scheduled notification sender for [ntfy.sh](https://ntfy.sh).

Schedule notifications from a single config file, packaged as a tiny Docker container that pushes to your ntfy instance.

## Why

You have an ntfy server and you want scheduled reminders — "water the plants on Friday", "trash day on Wednesday", "evening walk at 6pm". Instead of managing crontabs and scripts, this is a single binary that reads a TOML config and does exactly that.

- **~5 MB Docker image** (distroless)
- **One config file** — human-readable TOML
- **Every day, specific weekdays, multiple times per day**
- **Dry-run mode** for testing
- **No dependencies** at runtime

## Quick start

```bash
# 1. Copy and edit the config
cp config.example.toml config.toml
$EDITOR config.toml   # set your ntfy_url, topic, timezone, notifications

# 2. Dry-run to verify
go run . -dry-run

# 3. Run
go run .
```

Or with Docker:

```bash
docker build -t ding .
docker run --rm -v $(pwd)/config.toml:/etc/ding/config.toml ding
```

## Config

See [`config.example.toml`](config.example.toml) for a full example.

### Global settings (required)

| Field | Example | Description |
|---|---|---|
| `ntfy_url` | `"https://ntfy.example.com"` | Your ntfy server URL |
| `topic` | `"reminders"` | The ntfy topic to publish to |
| `timezone` | `"Europe/Paris"` | Go time zone name (IANA) |
| `auth_token` | `"abc123"` | *(optional)* Bearer token for auth |

### Notifications (required, one or more)

Each notification is an entry in the `notifications` array:

| Field | Required? | Type | Description |
|---|---|---|---|
| `title` | **yes** | string | Notification title shown in ntfy |
| `message` | no | string | Notification body text |
| `weekdays` | no | `[]string` | Days to fire. If absent, fires **every day** |
| `times` | **yes** | `[]string` | Times in `"HH:MM"` format. At least one required |

### Examples

**Every day at 7:30am:**
```toml
[[notifications]]
title = "Morning coffee"
message = "Time to brew a fresh cup."
times = ["07:30"]
```

**Every day, two reminders:**
```toml
[[notifications]]
title = "Stretch break"
message = "Stand up and move around."
times = ["10:00", "15:00"]
```

**Every Monday and Wednesday at noon:**
```toml
[[notifications]]
title = "Team standup"
message = "15-min sync call."
weekdays = ["Monday", "Wednesday"]
times = ["12:00"]
```

**Every Friday at 9pm:**
```toml
[[notifications]]
title = "Water the plants"
message = "Balcony plants need water."
weekdays = ["Friday"]
times = ["21:00"]
```

**Trash day — Wed and Sat, two reminders:**
```toml
[[notifications]]
title = "Trash"
message = "Bins out before 7, back after 6."
weekdays = ["Wednesday", "Saturday"]
times = ["06:30", "18:30"]
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `-config` | `./config.toml` or `/etc/ding/config.toml` | Path to config file |
| `-dry-run` | `false` | Print what would fire without sending |

## Docker

```bash
docker build -t ding .
```

Run with config mounted:

```bash
docker run -d \
  --name ding \
  --restart unless-stopped \
  -v /path/to/config.toml:/etc/ding/config.toml:ro \
  ding
```

The final image is based on `gcr.io/distroless/static` (~5 MB). No shell inside — logs go to stdout, read with `docker logs ding`.

## How it works

The program polls every **60 seconds**. On each tick it:

1. Gets the current time in your configured timezone
2. Checks each notification — does the weekday match? Does the minute match?
3. If yes, POSTs a JSON payload to your ntfy server

With a 60-second poll interval each notification fires at most once per minute.

## License

MIT
