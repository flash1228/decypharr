# Decypharr

![ui](docs/src/assets/images/index.png)

**Decypharr** is a **Media Gateway** for Debrid services and Usenet written in Go.

## What is Decypharr?

Decypharr provides a unified interface for Sonarr, Radarr, and other *Arr applications to access Debrid providers and
Usenet streaming.

## Features

- Mock QBittorrent and SABnzbd API that supports the Arrs (Sonarr, Radarr, Lidarr etc)
- Multiple Debrid and Usenet providers support with a single interface
- Direct Usenet streaming via NNTP (no separate download client required)
- Built-in rclone VFS mount for zero-copy symlink-based media delivery
- Automatic repair of broken/incomplete downloads

## Supported Debrid Providers

- [Real Debrid](https://real-debrid.com)
- [Torbox](https://torbox.app)
- [Debrid Link](https://debrid-link.com)
- [All Debrid](https://alldebrid.com)

## Quick Start

### Docker (Recommended)

```yaml
services:
  decypharr:
    image: cy01/blackhole:latest
    container_name: decypharr
    ports:
      - "8282:8282"
    volumes:
      - /mnt/:/mnt:rshared
      - ./configs/:/app # config.json must be in this directory
    restart: unless-stopped
    devices:
      - /dev/fuse:/dev/fuse:rwm
    cap_add:
      - SYS_ADMIN
    security_opt:
      - apparmor:unconfined
```

> Prefer not to self-host? A managed Decypharr instance is available
> via [ElfHosted](https://store.elfhosted.com/product/decypharr/?utm_source=github&utm_medium=readme&utm_campaign=decypharr-readme),
> preconfigured alongside Sonarr/Radarr to route requests to your debrid provider (7-day trial).

---

## Configuration Reference

Decypharr is configured via a single `config.json` file placed in the `/app` volume.

### Top-level fields

| Field | Type | Default | Description |
|---|---|---|---|
| `bind_address` | string | `"0.0.0.0"` | Address to listen on |
| `port` | string | `"8282"` | HTTP port |
| `url_base` | string | `"/"` | URL prefix (useful behind a reverse proxy) |
| `log_level` | string | `"INFO"` | Log verbosity: `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `download_folder` | string | — | Root folder where symlinks/downloads are placed |
| `default_download_action` | string | `"symlink"` | `"symlink"` or `"download"` |
| `folder_naming` | string | `"original_no_ext"` | Naming scheme for download folders |
| `categories` | array | — | Arr category names to handle (e.g. `["sonarr","radarr"]`) |
| `refresh_interval` | string | `"60s"` | How often to poll debrid for completed items |
| `symlinkReadyTimeout` | int | `600` | Seconds to wait for a symlink to become readable before failing |
| `retries` | int | `3` | Number of retries for failed debrid requests |
| `remove_stalled_after` | string | `"10m"` | Remove a download from the queue if stalled this long |
| `skip_pre_cache` | bool | `false` | Skip pre-caching content on the debrid side |
| `allowed_file_types` | array | (media extensions) | Whitelist of file extensions to process |
| `download_uid` | int | `null` | UID to set on created download dirs and symlinks (`-1` = no change) |
| `download_gid` | int | `null` | GID to set on created download dirs and symlinks (`-1` = no change) |

### `debrids[]` — Debrid provider configuration

Each entry in the `debrids` array configures one provider. Multiple providers are supported simultaneously.

```json
{
  "debrids": [
    {
      "provider": "torbox",
      "name": "torbox",
      "api_key": "YOUR_TORBOX_API_KEY",
      "download_api_keys": ["YOUR_TORBOX_API_KEY"],
      "rate_limit": "200/minute",
      "repair_rate_limit": "60/minute",
      "download_rate_limit": "8/minute",
      "unpack_rar": true,
      "skip_pre_cache": true,
      "minimum_free_slot": 2,
      "torrents_refresh_interval": "10m",
      "download_links_refresh_interval": "30m",
      "workers": 100,
      "auto_expire_links_after": "2h"
    },
    {
      "provider": "realdebrid",
      "name": "realdebrid",
      "api_key": "YOUR_REALDEBRID_API_KEY",
      "download_api_keys": ["YOUR_REALDEBRID_API_KEY"],
      "rate_limit": "200/minute",
      "repair_rate_limit": "60/minute",
      "skip_pre_cache": true,
      "minimum_free_slot": 1,
      "torrents_refresh_interval": "10m",
      "download_links_refresh_interval": "30m",
      "workers": 100,
      "auto_expire_links_after": "2h"
    }
  ]
}
```

| Field | Description |
|---|---|
| `provider` | Provider ID: `torbox`, `realdebrid`, `debridlink`, `alldebrid` |
| `name` | Display name (used in logs and Arr client label) |
| `api_key` | Your debrid API key |
| `download_api_keys` | Keys used for download link generation (usually same as `api_key`) |
| `rate_limit` | API request rate limit (e.g. `"200/minute"`) |
| `repair_rate_limit` | Rate limit used during repair sweeps |
| `download_rate_limit` | Rate limit for download link fetches |
| `unpack_rar` | Ask the provider to extract RAR archives (TorBox only) |
| `skip_pre_cache` | Skip pre-caching — set `true` if you only want cached torrents |
| `minimum_free_slot` | Minimum free active-torrent slots to keep available |
| `torrents_refresh_interval` | How often to refresh the torrent list from the provider |
| `download_links_refresh_interval` | How often to refresh expiring download links |
| `workers` | Concurrent worker goroutines for this provider |
| `auto_expire_links_after` | Refresh download links before they expire. Should be less than or equal to `vfs_cache_max_age` in the rclone mount config |

### `arrs[]` — Arr application configuration

```json
{
  "arrs": [
    {
      "name": "sonarr",
      "host": "http://sonarr:8989",
      "token": "YOUR_SONARR_API_KEY",
      "download_uncached": false,
      "cleanup": true
    },
    {
      "name": "radarr",
      "host": "http://radarr:7878",
      "token": "YOUR_RADARR_API_KEY",
      "download_uncached": false,
      "cleanup": true
    }
  ]
}
```

| Field | Description |
|---|---|
| `name` | Arr instance name — must match the category set in the Arr download client |
| `host` | Internal URL to the Arr instance |
| `token` | Arr API key |
| `download_uncached` | Allow adding uncached torrents (will queue until the provider caches them) |
| `cleanup` | Automatically remove completed items from the download queue |

### `usenet` — Usenet / NNTP configuration

Required if you want to use decypharr as a SABnzbd-compatible download client for NZB files.

```json
{
  "usenet": {
    "providers": [
      {
        "host": "news.example.com",
        "port": 563,
        "username": "your-username",
        "password": "your-password",
        "backbone": "ProviderName",
        "max_connections": 15,
        "ssl": true,
        "priority": 1
      }
    ],
    "max_connections": 50,
    "read_ahead": "32MB",
    "processing_timeout": "15m",
    "availability_sample_percent": 5,
    "max_concurrent_nzb": 2,
    "disk_buffer_path": "/mnt/cache/decypharr-streams"
  }
}
```

| Field | Description |
|---|---|
| `providers[].host` | NNTP server hostname |
| `providers[].port` | NNTP port (563 for SSL, 119 for plain) |
| `providers[].username` / `password` | NNTP credentials |
| `providers[].backbone` | Label for logging/identification |
| `providers[].max_connections` | Max simultaneous connections to this server |
| `providers[].ssl` | Use SSL/TLS |
| `providers[].priority` | Lower number = higher priority |
| `max_connections` | Global connection cap across all providers |
| `read_ahead` | Buffer size for streaming NZB segments |
| `processing_timeout` | Abort a stuck NZB after this duration |
| `availability_sample_percent` | Percentage of segments to probe for availability check |
| `max_concurrent_nzb` | Max NZB downloads running simultaneously |
| `disk_buffer_path` | Temporary disk buffer path for NZB stream processing |

**Note:** Decypharr implements the SABnzbd protocol natively. In Sonarr/Radarr, add it as a SABnzbd download client pointing at `http://<decypharr>:8282/sabnzbd`. No separate SABnzbd instance is needed.

### `mount` — rclone VFS mount configuration

```json
{
  "mount": {
    "type": "rclone",
    "mount_path": "/mnt/decypharr",
    "rclone": {
      "port": "5572",
      "cache_dir": "/mnt/cache/decypharr-cache",
      "vfs_cache_mode": "full",
      "vfs_cache_max_age": "12h",
      "vfs_cache_max_size": "42949672960",
      "vfs_cache_poll_interval": "15s",
      "vfs_read_chunk_size": "128M",
      "vfs_read_chunk_size_limit": "512M",
      "vfs_read_ahead": "0M",
      "vfs_fast_fingerprint": true,
      "buffer_size": "32M",
      "async_read": true,
      "transfers": 1,
      "uid": 1000,
      "gid": 1000,
      "attr_timeout": "1h",
      "dir_cache_time": "5m",
      "log_level": "INFO"
    }
  }
}
```

| Field | Description |
|---|---|
| `mount_path` | Where decypharr mounts the debrid virtual filesystem |
| `rclone.port` | rclone RC (remote control) port |
| `rclone.cache_dir` | Local disk cache for VFS; should be on fast storage |
| `vfs_cache_mode` | `full` recommended for media streaming |
| `vfs_cache_max_age` | How long cached files are kept. Set equal to or greater than `auto_expire_links_after` on the debrid provider |
| `vfs_cache_max_size` | Max cache size in bytes (e.g. `42949672960` = 40 GB) |
| `vfs_cache_poll_interval` | How often rclone polls for expired cache entries |
| `vfs_read_chunk_size` | Initial chunk size per read request |
| `vfs_read_chunk_size_limit` | Maximum chunk size after exponential growth |
| `vfs_read_ahead` | Bytes to pre-fetch ahead of current read position (`0M` disables) |
| `vfs_fast_fingerprint` | Use fast file fingerprinting — recommended; avoids full-stat overhead and is safe when file sizes are trusted |
| `buffer_size` | In-memory buffer per transfer |
| `async_read` | Read ahead asynchronously |
| `transfers` | Number of concurrent rclone transfers |
| `uid` / `gid` | User/group for mounted files (should match your media server user) |
| `attr_timeout` | How long to cache file attributes in the FUSE layer |
| `dir_cache_time` | How long to cache directory listings |

### `repair` — Automatic repair configuration

```json
{
  "repair": {
    "enabled": true,
    "source": "arr",
    "schedule": "0 */12 * * *",
    "workers": 5,
    "nntp_connection_percent": 20,
    "strategy": "per_entry",
    "recheck_interval": "14h",
    "auto_repair": true
  }
}
```

| Field | Description |
|---|---|
| `enabled` | Enable the repair subsystem |
| `source` | Where to get the list of items to check: `"arr"` pulls from connected Arr queues |
| `schedule` | Cron expression for repair sweeps (e.g. `"0 */12 * * *"` = every 12 hours) |
| `workers` | Concurrent repair goroutines |
| `nntp_connection_percent` | Percentage of usenet connections to use during repair (avoids saturating live streaming) |
| `strategy` | Repair strategy: `"per_entry"` repairs one item fully before moving to the next |
| `recheck_interval` | Minimum interval before rechecking the same item |
| `auto_repair` | Automatically repair without manual trigger |

---

## Arr Download Client Setup

### QBittorrent (for torrents via Debrid)

In Sonarr/Radarr → Settings → Download Clients → Add → qBittorrent:

| Setting | Value |
|---|---|
| Host | `decypharr` (or IP/hostname) |
| Port | `8282` |
| Category | `sonarr` or `radarr` (must match `categories` in config) |

### SABnzbd (for Usenet / NZB)

In Sonarr/Radarr → Settings → Download Clients → Add → SABnzbd:

| Setting | Value |
|---|---|
| Host | `decypharr` (or IP/hostname) |
| Port | `8282` |
| URL Base | `/sabnzbd` |
| Username | The full URL of your Arr instance (e.g. `http://sonarr:8989`) |
| Password | Your Arr API key |
| Category | `sonarr` or `radarr` |

The Username/Password fields are used internally by decypharr for callback routing, not for authentication.

---

## Fork Changes

This fork ([TwistedRat/decypharr](https://github.com/TwistedRat/decypharr)) contains the following fixes and improvements on top of upstream:

### Bug Fixes

- **`fix: skip directory creation when no files pass allowed_file_types filter`**
  When a torrent contains only files with disallowed extensions (e.g. fake or malware releases with `.exe` or `.scr` files), all files are filtered by `allowed_file_types` during ingestion, leaving the entry with zero eligible files. Previously, `processSymlink` still created the download directory with no symlinks inside it. Sonarr/Radarr would then report "no files found are eligible for import" and stall in the queue indefinitely, requiring manual cleanup and a new search. Now, if no eligible files are found, an error is returned before the directory is created, allowing the Arr to immediately retry with a different release.

- **`fix(nzb): stop normalizeNZBFileSizes from corrupting valid file sizes`**
  `streamSizeFromSegments` uses `max(seg.EndOffset+1)` to estimate file size, which underestimates for sliced RAR segments (offsets are volume-relative, not cumulative). The reduction condition was silently shrinking correctly-parsed multi-GB NZB files down to a single segment size (~50 MB), causing any seek beyond that size to error out. Fix: `normalizeNZBFileSizes` now only fills in missing (zero) sizes from segment data — it never reduces a size that is already positive. Also: `usenet.go` returns a silent error (not a noisy retried error) when `rangeStart >= volume size`, stopping log spam from stale data.

- **`feat: chown download dirs and symlinks to configurable uid/gid`**
  When decypharr runs as root (required for FUSE in Docker/LXC), download directories and symlinks were created owned by root. Arr applications running as uid 1000 then failed to import with `EACCES`. Two new config fields — `download_uid` and `download_gid` — allow ownership to be set on every directory, symlink, and `.strm` file created under `download_folder`. `Lchown` is used rather than `Chown` so symlinks themselves are chowned rather than their rclone VFS targets. Omitting the fields (or setting `-1`) leaves ownership unchanged, preserving existing behaviour. Typical setup: `"download_uid": 1000, "download_gid": 1000`.

- **`fix(arr): poll Arr queue instead of fixed delay before RefreshMonitoredDownloads`**
  For cached torrents, the full pipeline (submit → symlink → complete) finishes in under a second — the same second Sonarr is still writing the grab queue entries to its database. Sending `RefreshMonitoredDownloads` before those entries are committed caused Sonarr to find nothing to import and mark entries as warning, triggering unnecessary retries. Instead of a fixed 5-second sleep, decypharr now polls `GET /api/v3/queue?downloadId=<hash>` once per second (up to 30 s) and fires `RefreshMonitoredDownloads` as soon as the entry appears. This eliminates the race condition without adding unnecessary latency on slow downloads.

- **`fix(qbit): populate completion_on from entry CompletedAt`**
  `completion_on` was hardcoded to `0` in the qBittorrent API response. Sonarr uses this field to detect completed downloads and trigger imports — with it always being 0, imports never fired even when symlinks were fully ready.

- **`fix(sabnzbd): return HTTP 200 for NZB processing failures`**
  Decypharr was returning HTTP 500 for NZB processing errors (article-not-found, no valid file groups, etc.). The real SABnzbd protocol always returns HTTP 200 with a JSON body `{"status": false, "error": "..."}` for failed adds. The HTTP 500 caused Sonarr/Radarr to mark the entire download client as unavailable, blocking all further downloads until manually cleared.

- **`fix(torbox): improve resilience during CDN/API maintenance windows`**
  Added HTTP 503 and 504 to the list of retryable status codes for TorBox API calls. Returns typed `HosterUnavailableError` for 5xx responses so retry/requeue logic is triggered correctly. Transient debrid errors (API timeouts, maintenance, 503/504) now requeue with a 30-second delay instead of being dropped.

- **`fix(torbox): resolve actual CDN URL from requestdl`**
  Resolves the actual CDN download URL once at link-fetch time rather than making an API call per chunk. Eliminates per-chunk API overhead and avoids CDN 502 errors from poisoning the FUSE mount.

- **`fix: prevent CDN 502 errors from permanently poisoning the FUSE mount`**
  CDN transient errors no longer corrupt cached mount entries.

- **`fix: prevent NZB ffprobe D-state deadlock from blocking completeEntry`**
  A stuck ffprobe process could block the completion goroutine indefinitely, preventing any further imports.

- **`fix WebDAV stuck on NNTP 430 article-not-found`**
  NNTP 430 (article not found — common for expired usenet content) no longer causes the WebDAV/streaming layer to hang.

- **`fix: handle missing Usenet articles gracefully in NZB batch downloads`**
  Expired or unavailable NZB segments now fail cleanly rather than blocking the download pipeline.

- **`fix(mount): coalesce concurrent RefreshMount calls via singleflight`**
  Multiple simultaneous requests to refresh the rclone mount now collapse into a single actual refresh, preventing thundering-herd issues.

- **`fix(parser): handle bracket-heavy release names and extensionless RAR files`**
  Improved release name parsing for edge cases common in scene/P2P releases.

- **`feat: smart torrent name truncation to fix Linux NAME_MAX 255-byte limit`**
  Torrent names exceeding 255 bytes (the Linux filesystem limit) are now intelligently truncated to avoid ENAMETOOLONG errors when creating symlinks.

- **`fix(torbox): treat 'incomplete' status as downloading, not error`**
  TorBox `incomplete` torrent status is now correctly treated as in-progress rather than a permanent failure.

- **`fix(torbox): remove trailing slash from /api/torrents/mylist/`** (GAP-003)
  API URL correction for the TorBox torrent list endpoint.

- **`fix(torbox): fully implement 300 req/min rate limit cap`** (GAP-002)
  TorBox enforces a hard 300 req/min ceiling; the rate limiter now respects this.

- **`fix(torbox): correct plan number mapping in GetProfile`** (GAP-010)
  TorBox plan tier detection was mapping incorrect plan numbers, affecting active slot management.

- **`fix: skip blacklist when content already exists in rclone mount`**
  Prevents healthy cached content from being blocklisted when a transient error occurs during import.

- **`fix: replace MarkHistoryFailed with Refresh() to avoid cross-source blacklisting`**
  Using `MarkHistoryFailed` caused Sonarr to blocklist releases across all indexers; `Refresh()` correctly marks only the specific download as failed without poisoning the release history.

- **`fix(#298): respect deleteFiles flag and gate auto-blocklist`**
  Fixes premature file deletion during cleanup and prevents incorrect auto-blocklisting.

- **`fix(#315): abort batch download on GetLink failure and clear IsDownloading on error`**
  Items no longer get permanently stuck in a downloading state when a link fetch fails.

- **`Retry link validation on transient errors (429/502/503/504) with backoff`**
  Link validation retries with exponential backoff on transient HTTP errors rather than failing immediately.

### Features

- **`feat: proactive Arr blacklist on permanent failures + startup re-notification`**
  Permanent debrid failures (e.g. content unavailable on provider) are now proactively reported back to the Arr so it can immediately search for an alternative release.

- **`feat(repair): auto-detect and repair missing NZB metadata`**
  The repair subsystem can now detect and fix entries with missing or corrupt NZB metadata without manual intervention.

---

## Documentation

For complete documentation, please visit [docs.decypharr.com](https://docs.decypharr.com).

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
