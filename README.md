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

A complete annotated example configuration is available at [`docs/config.example.json`](docs/config.example.json). Copy it to `/app/config.json`, replace every `YOUR_*` placeholder with real values, and remove any sections you don't need.

---

## Configuration Reference

Decypharr is configured via a single `config.json` file placed in the `/app` volume.

### Top-level fields

| Field | Type | Default | Description |
|---|---|---|---|
| `bind_address` | string | `"0.0.0.0"` | Address to listen on |
| `port` | string | `"8282"` | HTTP port |
| `url_base` | string | `"/"` | URL prefix (useful behind a reverse proxy) |
| `app_url` | string | — | External base URL (e.g. `"https://decypharr.example.com"`) — used in callbacks and notifications |
| `log_level` | string | `"INFO"` | Log verbosity: `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `download_folder` | string | — | Root folder where symlinks/downloads are placed |
| `default_download_action` | string | `"symlink"` | `"symlink"`, `"download"`, `"strm"`, or `"none"` |
| `folder_naming` | string | `"original_no_ext"` | Naming scheme: `"filename"`, `"original"`, `"filename_no_ext"`, `"original_no_ext"`, `"infohash"` |
| `categories` | array | `["sonarr","radarr"]` | Arr category names to handle |
| `refresh_interval` | string | `"30s"` | How often to poll debrid providers for completed items |
| `retries` | int | `3` | Number of retries for failed debrid API requests |
| `remove_stalled_after` | string | — | Remove a download from the queue if stalled this long (e.g. `"10m"`) |
| `skip_pre_cache` | bool | `false` | Skip pre-caching content on the debrid side |
| `max_downloads` | int | `0` | Maximum concurrent active downloads (0 = unlimited) |
| `skip_multi_season` | bool | `false` | Skip multi-season torrent packs |
| `always_rm_tracker_urls` | bool | `false` | Always strip tracker URLs from magnet links |
| `allowed_file_types` | array | (media extensions) | Whitelist of file extensions to process |
| `allow_samples` | bool | `false` | Include sample/trailer/extras files (normally filtered out) |
| `min_file_size` | string | — | Minimum file size to process (e.g. `"100MB"`) |
| `max_file_size` | string | — | Maximum file size to process (e.g. `"100GB"`) |
| `bd_main_file_only` | bool | `true` | For Blu-ray rips with multiple `.m2ts` files, only expose the largest (main feature). Prevents unnecessary probing of secondary streams by Plex/rclone |
| `prefer_ascii_name` | bool | `true` | Extract ASCII title from mixed-script release names (e.g. CJK/Cyrillic). Disable only if your library is intentionally non-Western |
| `download_uid` | int | — | UID to set on created download dirs and symlinks (`-1` = no change). Typical Docker/LXC setup: `1000` |
| `download_gid` | int | — | GID to set on created download dirs and symlinks (`-1` = no change). Typical Docker/LXC setup: `1000` |
| `use_auth` | bool | `false` | Enable built-in username/password authentication for the web UI and API |
| `disable_webdav` | bool | `false` | Disable the built-in WebDAV server |
| `nzb_user_agent` | string | — | Custom User-Agent header for downloading NZB files |
| `skip_auto_move` | bool | `false` | Skip automatic file moving after download |

### `debrids[]` — Debrid provider configuration

Each entry in the `debrids` array configures one provider. Multiple providers are supported simultaneously.

```json
{
  "debrids": [
    {
      "provider": "torbox",
      "name": "torbox",
      "api_key": "YOUR_TORBOX_API_KEY",
      "rate_limit": "200/minute",
      "repair_rate_limit": "60/minute",
      "download_rate_limit": "8/minute",
      "unpack_rar": true,
      "minimum_free_slot": 2,
      "torrents_refresh_interval": "10m",
      "download_links_refresh_interval": "30m",
      "workers": 100,
      "auto_expire_links_after": "2h",
      "usenet_backend": "auto"
    },
    {
      "provider": "realdebrid",
      "name": "realdebrid",
      "api_key": "YOUR_REALDEBRID_API_KEY",
      "rate_limit": "200/minute",
      "repair_rate_limit": "60/minute",
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
| `api_key` | Your debrid API key. Also used for download link generation unless `download_api_keys` is set |
| `download_api_keys` | Override keys for download link generation. Omit to reuse `api_key` (the default) |
| `rate_limit` | API request rate limit (e.g. `"200/minute"`) |
| `repair_rate_limit` | Rate limit used during repair sweeps |
| `download_rate_limit` | Rate limit for download link fetches |
| `unpack_rar` | Ask the provider to extract RAR archives (TorBox only) |
| `download_uncached` | Allow adding uncached torrents (will queue until the provider caches them) |
| `minimum_free_slot` | Minimum free active-torrent slots to keep available |
| `limit` | Maximum total torrents allowed on this provider (0 = unlimited) |
| `torrents_refresh_interval` | How often to refresh the torrent list from the provider |
| `download_links_refresh_interval` | How often to refresh expiring download links |
| `workers` | Concurrent worker goroutines for this provider |
| `auto_expire_links_after` | Refresh download links before they expire. Should be ≤ `vfs_cache_max_age` |
| `proxy` | HTTP proxy URL for all API calls to this provider |
| `user_agent` | Custom User-Agent header for API calls to this provider |
| `usenet_backend` | NZB routing for this debrid provider: `"auto"` (default — use TorBox usenet API when Pro plan detected, fall back to NNTP), `"torbox"` (always use TorBox API, fails if not Pro), `"nntp"` (always use configured NNTP providers, skip TorBox API). TorBox-only field |

### `arrs[]` — Arr application configuration

```json
{
  "arrs": [
    {
      "name": "sonarr",
      "host": "http://sonarr:8989",
      "token": "YOUR_SONARR_API_KEY",
      "download_uncached": false,
      "cleanup": true,
      "skip_repair": false
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
| `skip_repair` | Exclude this Arr's items from repair sweeps |
| `selected_debrid` | Pin this Arr to a specific debrid provider by name (omit to use all configured providers) |

### `usenet` — Usenet / NNTP configuration

Required if you want to use decypharr as a SABnzbd-compatible download client for NZB files.

Multiple NNTP providers are supported simultaneously. Providers with the same `backbone` value share article-availability state — if one backbone returns 430 (article not found), all providers on that backbone are skipped for that article and the next distinct backbone is tried. Set `priority: 1` on all providers for round-robin load balancing; use higher priority numbers for fallback-only providers.

```json
{
  "usenet": {
    "providers": [
      {
        "host": "news.provider1.com",
        "port": 563,
        "username": "your-username",
        "password": "your-password",
        "backbone": "ProviderBackbone",
        "max_connections": 15,
        "ssl": true,
        "priority": 1
      },
      {
        "host": "news.provider2.com",
        "port": 563,
        "username": "your-username",
        "password": "your-password",
        "backbone": "AnotherBackbone",
        "max_connections": 10,
        "ssl": true,
        "priority": 1
      }
    ],
    "max_connections": 50,
    "read_ahead": "32MB",
    "processing_timeout": "15m",
    "availability_sample_percent": 5,
    "max_concurrent_nzb": 2,
    "disk_buffer_path": "/mnt/cache/decypharr-streams",
    "skip_repair": false
  }
}
```

| Field | Description |
|---|---|
| `providers[].host` | NNTP server hostname |
| `providers[].port` | NNTP port (563 for SSL, 119 for plain) |
| `providers[].username` / `password` | NNTP credentials |
| `providers[].backbone` | Backbone identifier for failover grouping. Providers sharing the same backbone are treated as redundant — 430 on one excludes the entire backbone before trying the next |
| `providers[].max_connections` | Max simultaneous connections to this server (default: 20) |
| `providers[].ssl` | Use SSL/TLS |
| `providers[].priority` | Failover priority — lower number = higher priority. Providers at the same priority level are load-balanced |
| `max_connections` | Per-file connection cap across all providers (default: 15) |
| `read_ahead` | Bytes to prefetch ahead of current read position (default: `"16MB"`) |
| `processing_timeout` | Abort a stuck NZB after this duration (default: `"10m"`) |
| `availability_sample_percent` | Percentage of segments to probe for availability check (1–100, default: 10) |
| `max_concurrent_nzb` | Max NZBs processed in parallel (default: 2) |
| `disk_buffer_path` | Temporary disk buffer path for NZB stream data |
| `skip_repair` | Exclude NNTP entries from repair sweeps |

**Note:** Decypharr implements the SABnzbd protocol natively. In Sonarr/Radarr, add it as a SABnzbd download client pointing at `http://<decypharr>:8282/sabnzbd`. No separate SABnzbd instance is needed.

### `mount` — Mount configuration

Decypharr supports four mount types set via `mount.type`:

| Type | Description |
|---|---|
| `rclone` | Built-in rclone VFS mount (recommended for most setups) |
| `dfs` | Built-in DFS (Distributed Filesystem) mount — alternative to rclone |
| `external_rclone` | Connect to an already-running rclone RC instance |
| `none` | No mount — use only if you manage the debrid filesystem externally |

#### `mount.type = "rclone"` (recommended)

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
      "vfs_cache_max_size": "40G",
      "vfs_cache_poll_interval": "15s",
      "vfs_read_chunk_size": "256M",
      "vfs_read_chunk_size_limit": "1G",
      "vfs_read_ahead": "512M",
      "vfs_fast_fingerprint": true,
      "buffer_size": "32M",
      "async_read": true,
      "transfers": 2,
      "uid": 1000,
      "gid": 1000,
      "attr_timeout": "1h",
      "dir_cache_time": "5m",
      "daemon_timeout": "30s",
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
| `vfs_cache_mode` | `"full"` recommended for media streaming (`"off"`, `"minimal"`, `"writes"`, `"full"`) |
| `vfs_cache_max_age` | How long cached files are kept. Set ≥ `auto_expire_links_after` on the debrid provider |
| `vfs_cache_max_size` | Max VFS cache size as a human-readable string: `"40G"`, `"500M"`, etc. |
| `vfs_disk_space_total` | Total disk space available for the VFS cache (alternative to `vfs_cache_max_size`) |
| `vfs_cache_min_free_space` | Minimum free disk space to maintain (e.g. `"10G"`) |
| `vfs_cache_poll_interval` | How often rclone evicts expired cache entries |
| `vfs_read_chunk_size` | Initial chunk size per read request (e.g. `"256M"`) |
| `vfs_read_chunk_size_limit` | Maximum chunk size after exponential growth (e.g. `"1G"`). `"off"` disables the limit |
| `vfs_read_chunk_streams` | Number of parallel streams per chunk fetch (0 = rclone default) |
| `vfs_read_ahead` | Bytes to pre-fetch ahead of current read position (e.g. `"512M"`, `"0"` to disable) |
| `vfs_fast_fingerprint` | Use fast file fingerprinting — recommended; safe when file sizes are trusted |
| `buffer_size` | In-memory buffer per transfer |
| `async_read` | Read ahead asynchronously (default: `true`) |
| `transfers` | Number of concurrent chunk pre-fetches. Set to `2` or higher to eliminate stalls at chunk boundaries for uncached content |
| `uid` / `gid` | User/group for mounted files (should match your media server user) |
| `umask` | Octal permission mask for mounted files (e.g. `"022"`) |
| `attr_timeout` | How long to cache file attributes in the FUSE layer |
| `dir_cache_time` | How long to cache directory listings |
| `timeout` | HTTP IO idle timeout (e.g. `"5m"`). Shorter values prevent hung FUSE reads on stalled connections |
| `connect_timeout` | HTTP connect timeout (e.g. `"1m"`) |
| `daemon_timeout` | FUSE kernel operation timeout. After this duration, the kernel returns `ETIMEDOUT`, unblocking D-state processes. Recommended: `"30s"` |
| `bw_limit` | Bandwidth limit for all rclone transfers (e.g. `"50M"`, `"off"`) |
| `no_modtime` | Disable modification time reads/writes |
| `no_checksum` | Disable file checksumming on upload |
| `use_mmap` | Use memory-mapped I/O |
| `log_level` | rclone log verbosity (`"DEBUG"`, `"INFO"`, `"NOTICE"`, `"ERROR"`) |

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
    "recheck_interval": "168h",
    "auto_repair": true,
    "notify_on_complete": false,
    "arrs": []
  }
}
```

| Field | Description |
|---|---|
| `enabled` | Enable the repair subsystem |
| `source` | Where to enumerate items: `"arr"` (pull from connected Arr queues), `"managed"` |
| `schedule` | Cron expression for repair sweeps (e.g. `"0 */12 * * *"` = every 12 hours) |
| `workers` | Concurrent repair goroutines (default: 5) |
| `nntp_connection_percent` | Percentage of total NNTP connections reserved for repair (default: 20). Prevents repair from starving live streaming |
| `strategy` | Repair strategy: `"per_entry"` repairs one item fully before moving to the next |
| `recheck_interval` | Minimum interval before rechecking the same item (default: `"168h"` = 7 days) |
| `auto_repair` | Automatically trigger repair without manual intervention |
| `notify_on_complete` | Send a notification when a repair sweep completes |
| `arrs` | Optional list of Arr names to limit repair scope (e.g. `["sonarr"]`). Empty = repair items from all configured Arrs |

### `notifications` — Notification configuration

```json
{
  "notifications": {
    "enabled": true,
    "webhook_url": "https://discord.com/api/webhooks/YOUR_WEBHOOK",
    "callback_url": "https://your-service.example.com/decypharr/callback",
    "events": ["download_complete", "download_failed", "repair_complete", "repair_failed"]
  }
}
```

| Field | Description |
|---|---|
| `enabled` | Enable notifications globally |
| `webhook_url` | Discord webhook URL for event notifications |
| `callback_url` | HTTP endpoint for status callbacks |
| `events` | List of events to notify. Omit or leave empty to notify on all events. Valid values: `download_complete`, `download_failed`, `repair_pending`, `repair_complete`, `repair_failed`, `repair_cancelled` |

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

- **`fix(torbox): skip NZB entries in torrent sync to prevent deletion`** (commit `254d45b`)
  `detectTorrentChanges` iterates all storage entries and removes any whose InfoHash is absent from TorBox's torrent API. TorBox usenet entries use UUID InfoHashes that never appear in the torrent API, so they were silently deleted from storage on every 10-minute sync cycle. Fixed by adding a `ProtocolNZB` guard so usenet entries are skipped by the torrent sync loop entirely.

- **`fix(torbox): serialize concurrent NZB submissions to prevent slot race`** (commit `4e96be5`)
  Multiple NZB goroutines all called `GetActiveUsenetCount` simultaneously before any submission had registered, all saw `active=0`, all bypassed the 6-slot limit, causing TorBox 500/504 errors. Fixed with `torboxUsenetMu sync.Mutex` wrapping the count-check + submit sequence so only one goroutine at a time can claim a slot.

- **`fix(torbox): recover TorBox usenet entries interrupted mid-symlink on restart`** (commit `7d57e7a`)
  Entries in state `pausedUP` + `IsComplete=false` (finalization complete, symlink goroutine killed by restart) were ignored by `processQueuedEntries` (only handles `downloading`) and `renotifyCompletedEntries` (only triggers Arr refresh). Added `recoverTorboxUsenetEntries()` called from `runInitialCalls` to re-fire `processAction` for these entries on startup.

- **`fix: prime rclone VFS cache before notifying Sonarr of completed download`** (commit `4c509e7`)
  `verifySymlinkFileReady` only called `os.Open` + `f.Close()` without reading any data. rclone VFS is lazy — it fetches nothing from the CDN until bytes are actually read. Sonarr's MediaInfo would run immediately after the symlink check and race the CDN fetch, causing "Unable to determine if file is a sample" import failures. Fixed by reading 64 KB from the file to prime the VFS cache before `completeEntry` notifies the Arr.

- **`fix(usenet): fall back to NNTP when TorBox Pro usenet slots are full`** (commit `2a1d4de`)
  When all 6 TorBox Pro usenet slots are busy, the NZB entry was marked as error and Sonarr retried later — but only against TorBox, never checking NNTP providers. Fixed by re-routing the existing queue entry (preserving the ID Sonarr is tracking) through `processNewNzb` when `activeCount >= 6` and NNTP providers are configured. If no NNTP providers are configured the previous error behaviour is unchanged. Closes [#4](https://github.com/TwistedRat/decypharr/issues/4).

- **`fix(nntp): skip CDN prime read for NNTP entries; probe start + end instead`** (commits `d859a90`, `2452c6f`)
  The 64 KB prime read in `verifySymlinkFileReady` was designed for TorBox CDN-backed files where rclone VFS is lazy. NNTP files stream on demand — the prime read was both unnecessary and misleading (the first segment passing gave no signal about later segments). Changed to protocol-aware behaviour: CDN entries (TorBox torrent/usenet) keep the start-only prime read; NNTP entries read 64 KB from the **start and end** of the file. This catches the most common incomplete-retention failure (early segments present, later segments missing) before Sonarr is notified, without reading the whole file. Closes [#5](https://github.com/TwistedRat/decypharr/issues/5).

- **`fix(nntp): require n>0 from start/end probe reads; flag failures as permanent`** (commits `441b82f`, `59b15d2`)
  Two additional gaps in the NNTP probe: (1) `f.Read()` returning `(0, nil)` — rclone VFS serving a virtual file not yet fetched — passed the error-only check as "ready" in ~1 ms. Now both start and end reads require `n > 0`. A zero-byte start read retries (transient); a zero-byte end read is a permanent failure. (2) Permanent probe errors now surface immediately from the retry loop without waiting for the full timeout, and NNTP probe timeouts are also wrapped as permanent. Both permanent paths call `notifyArrFailedAndRemove` so Sonarr/Radarr blacklists the release and re-searches instead of leaving the entry stuck as "Downloading".

- **`fix(nntp): treat ActiveProvider=='usenet' as NNTP in queue recovery paths`** (commit `dc8ca1c`)
  `processQueuedEntries` and `recoverTorboxUsenetEntries` both checked `ActiveProvider == ""` to identify NNTP entries. NNTP entries carry `ActiveProvider = "usenet"` (the NNTP placement key), so any NNTP entry surviving a restart was incorrectly routed into the TorBox usenet resume branch, producing `ERROR: TorBox usenet resume: provider client not found debrid=usenet`. Extended both guards to also treat `"usenet"` as the NNTP path, matching the same fix already in `stream.go` and `DeleteEntry`.

- **`fix: code review hardening — EOF comparison, HTTP body leaks, redundant timeout`** (commit `22bf4d1`)
  Four issues identified by structured code review and fixed atomically. (1) `verifySymlinkFileReady` used `err.Error() != "EOF"` string comparison which fails to match wrapped `io.EOF` or `io.ErrUnexpectedEOF` returned by rclone VFS — replaced with `errors.Is(err, io.EOF)`. (2) `GetActiveUsenetCount`, `GetUsenetDownload`, `fetchUsenetDownloadLink`, and `controlUsenetDownload` in the TorBox usenet client did not close the HTTP response body on non-2xx status paths, leaking connections from the transport pool under sustained API errors — added `defer resp.Body.Close()` after each `doGet`/`doPost` call. (3) `WaitForUsenetCached` accepted a redundant `timeout time.Duration` parameter alongside a context that already carried the same deadline — the explicit `time.Now().After(deadline)` check was dead code; removed the parameter from the interface, implementation, and both callers. (4) Year range magic constants `1900`/`2099` in `nameparser.go` replaced with named `minNameYear`/`maxNameYear`.

- **`fix(stream): NNTP entries incorrectly routed to streamHTTP instead of streamUsenet`** (commit `f14799d`)
  All NZB entries — including NNTP — have `ActiveProvider = "usenet"` set (the NNTP placement key). The `Stream()` routing check `ActiveProvider != ""` was treating this as a TorBox CDN entry and calling `streamHTTP`, which then failed with "client for provider usenet not found" because "usenet" is not a registered debrid client. This caused every NNTP webdav request to return `file_not_available after refresh`. Fixed by tightening the routing condition to `ActiveProvider != "" && ActiveProvider != "usenet"` — only actual debrid names (e.g. "torbox") route to HTTP streaming; "usenet" and empty-string both go to `streamUsenet`. This was the root cause of NZB downloads completing (symlinks created, probe passed) but Plex/rclone getting `file_not_available` for every file.

- **`fix(nntp): treat yenc decode corruption as permanent failure`** (commit `3e77c63`)
  A yenc segment size mismatch (e.g. rapidyenc `expected size X but got Y: data corruption detected`) means the segment data on Usenet is corrupted — retrying will always fail. Previously these errors fell through as plain retriable errors, causing rclone VFS to retry the WebDAV read every 60 seconds indefinitely. Now handled the same as article-not-found: stored in `failedFiles`, persisted via `markNZBFileDeleted`, and returned as `NewArticleNotFoundError` so rclone receives 410 Gone (no retry) and Sonarr/Radarr blacklists the release for re-search.

- **`fix: TorBox CDN primeCache path also ignored read byte count`** (commit `c896e48`)
  The same `(0, nil)` false-positive existed on the `primeCache=true` (TorBox CDN) path — `n` was discarded and only `err` was checked. Added the same `n > 0` guard. `verifySymlinkFileReady` is now the verified byte-read gate for both TorBox CDN and NNTP, which also closes the false-positive risk from `WaitForUsenetCached` (TorBox API-state-only check) and `PreCache` (non-blocking prefetch) identified in a full verification audit.

### Features

- **`feat(torbox): TorBox Pro usenet API backend for NZB downloads`**
  When a TorBox debrid provider is configured and the account is on the **Pro plan**, incoming NZBs (from Sonarr/Radarr via the SABnzbd handler) are automatically routed through TorBox's usenet API (`/api/usenet/createusenetdownload`) instead of being streamed over NNTP. decypharr polls until the content is cached on TorBox's CDN, then builds the download entry and hands it to the normal symlink/arr-notification pipeline. Falls back to NNTP transparently on any TorBox error or timeout. Own NNTP providers remain fully functional and are always used as the fallback. Plan detection is automatic — no extra config required on Pro; the feature self-disables on Essential/Standard plans. Power users can override routing per debrid with `"usenet_backend": "torbox"` (force TorBox) or `"usenet_backend": "nntp"` (force NNTP, skip TorBox API entirely).

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
