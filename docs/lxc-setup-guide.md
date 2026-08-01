# Arr Stack LXC Container Setup Guide

A complete walkthrough for deploying Decypharr with a full Arr automation stack inside a Proxmox LXC container. This guide covers container creation, iGPU passthrough for Plex hardware transcoding, Docker configuration, and the full service stack.

## Stack Overview

| Service | Purpose | Port |
|---|---|---|
| **Decypharr** | Debrid gateway + NNTP streaming + rclone VFS mount | 8282 |
| **Plex** | Media server with iGPU hardware transcoding | 32400 |
| **Radarr** | Movie automation | 7878 |
| **Sonarr** | TV show automation | 8989 |
| **Prowlarr** | Indexer manager | 9696 |
| **Bazarr** | Subtitle downloader | 6767 |
| **Lingarr** | Subtitle translation | 9876 |
| **Seerr** | Media request management | 5055 |
| **Maintainerr** | Library cleanup automation | 6246 |
| **Cleanuparr** | Queue monitoring / stalled download cleanup | 11011 |
| **Tautulli** | Plex statistics and monitoring | 8978 |
| **Zilean** | DMM/Torrentio scrape cache | 8181 |
| **Bitmagnet** | DHT crawler + Torznab indexer | 3333 |
| **Byparr** | Cloudflare bypass proxy for Prowlarr | 8191 (internal) |
| **PostgreSQL ×2** | Databases for Zilean and Bitmagnet | internal |

---

## Prerequisites

- Proxmox VE host (tested on 8.x)
- Internet access from the LXC (for Docker image pulls and debrid API calls)
- Debrid provider account (TorBox and/or RealDebrid)
- Usenet provider account(s) if using NNTP streaming
- TMDb API key (free at [themoviedb.org](https://www.themoviedb.org/settings/api)) — for Bitmagnet

For hardware transcoding in Plex:
- Intel CPU with integrated graphics (UHD 600-series or newer for HEVC)
- iGPU must be visible on the Proxmox host (`ls /dev/dri/`)

---

## 1. Create the LXC Container in Proxmox

Create a Debian 12 (Bookworm) unprivileged container with the following minimum specs:

| Parameter | Recommended |
|---|---|
| Template | `debian-12-standard_*.tar.zst` |
| RAM | 12 GB (more if using Bitmagnet DHT crawler) |
| CPU cores | 6–8 |
| Root disk | 50 GB (on fast storage) |
| Unprivileged | Yes |
| Nesting | Yes |

> **Storage note:** Decypharr's rclone VFS cache and the Bitmagnet database grow over time. Mount a separate volume for cache (e.g. `/mnt/arr-cache`) on an SSD rather than the container root disk.

---

## 2. Configure the LXC (Proxmox host)

After creating the container, edit its config file on the Proxmox host:

```bash
nano /etc/pve/lxc/<VMID>.conf
```

Add the following lines. The FUSE and nesting options are **required** — without them the Decypharr rclone mount will not work inside the container.

```ini
# Required for rclone FUSE mount inside LXC
features: fuse=1,keyctl=1,nesting=1

# Required for Docker
lxc.apparmor.profile: unconfined
lxc.cap.drop:
lxc.cgroup2.devices.allow: a
lxc.mount.auto: proc:rw sys:rw
```

### 2a. iGPU passthrough (Plex hardware transcoding)

If you want Plex hardware transcoding, also add the Intel iGPU devices. Run `ls /dev/dri/` on the **Proxmox host** to confirm the device names, then add:

```ini
# Intel iGPU passthrough for Plex VAAPI hardware transcoding
lxc.cgroup2.devices.allow: c 226:0 rwm
lxc.cgroup2.devices.allow: c 226:128 rwm
lxc.mount.entry: /dev/dri/card0 dev/dri/card0 none bind,optional,create=file
lxc.mount.entry: /dev/dri/renderD128 dev/dri/renderD128 none bind,optional,create=file
```

> The `226:0` and `226:128` major:minor numbers are standard for Intel DRM devices. Verify with `ls -la /dev/dri/` on the host if your card uses different numbers.

Start the container after saving:

```bash
pct start <VMID>
```

---

## 3. Container Base Setup

Enter the container and install dependencies:

```bash
pct exec <VMID> -- bash
```

```bash
apt-get update && apt-get install -y \
    curl ca-certificates fuse3 mergerfs \
    lsb-release gnupg2 apt-transport-https \
    jq python3 ffmpeg
```

Create the non-root media user that all Arr services will run as:

```bash
useradd -u 1000 -g 1000 -m -s /bin/bash media
```

---

## 4. Install Docker

```bash
curl -fsSL https://get.docker.com | sh
systemctl enable --now docker
```

Verify Docker is working:

```bash
docker run --rm hello-world
```

---

## 5. Directory Structure

Create all required directories:

```bash
# Stack config and data (on fast root disk or dedicated volume)
mkdir -p /opt/docker/arrstack/{decypharr,radarr,sonarr,prowlarr,bazarr,lingarr}
mkdir -p /opt/docker/arrstack/{seerr,maintainerr,cleanuparr,tautulli,plex}
mkdir -p /opt/docker/arrstack/{zilean,bitmagnet,postgres}

# rclone VFS cache and NZB stream buffer (on SSD — grows over time)
mkdir -p /mnt/arr-cache/decypharr-cache
mkdir -p /mnt/arr-cache/decypharr-streams
mkdir -p /mnt/arr-cache/bitmagnet-db

# rclone FUSE mount point (must exist and be empty before starting)
mkdir -p /mnt/decypharr

# Symlink target — where Radarr/Sonarr/Plex see completed media
mkdir -p /srv/media/movies
mkdir -p /srv/media/shows
mkdir -p /srv/media/anime

# Set ownership so media user can write
chown -R 1000:1000 /srv/media
chown -R 1000:1000 /opt/docker/arrstack
```

---

## 6. Decypharr Configuration

Create `/opt/docker/arrstack/decypharr/config.json`. A full annotated example is in [`config.example.json`](config.example.json). Below is the minimal production config for this stack:

```json
{
  "port": "8282",
  "log_level": "INFO",

  "download_folder": "/srv/media",
  "default_download_action": "symlink",
  "folder_naming": "original_no_ext",
  "categories": ["sonarr", "radarr"],

  "refresh_interval": "30s",
  "retries": 3,
  "remove_stalled_after": "10m",

  "allowed_file_types": ["mkv", "mp4", "avi", "m2ts", "ts"],
  "allow_samples": false,
  "min_file_size": "50MB",
  "bd_main_file_only": true,

  "download_uid": 1000,
  "download_gid": 1000,

  "debrids": [
    {
      "provider": "torbox",
      "name": "torbox",
      "api_key": "YOUR_TORBOX_API_KEY",
      "rate_limit": "200/minute",
      "minimum_free_slot": 2,
      "torrents_refresh_interval": "10m",
      "download_links_refresh_interval": "30m",
      "workers": 100,
      "auto_expire_links_after": "12h",
      "usenet_backend": "nntp"
    },
    {
      "provider": "realdebrid",
      "name": "realdebrid",
      "api_key": "YOUR_REALDEBRID_API_KEY",
      "rate_limit": "200/minute",
      "minimum_free_slot": 1,
      "torrents_refresh_interval": "10m",
      "download_links_refresh_interval": "30m",
      "workers": 100,
      "auto_expire_links_after": "12h"
    }
  ],

  "arrs": [
    {
      "name": "sonarr",
      "host": "http://sonarr:8989",
      "token": "YOUR_SONARR_API_KEY",
      "cleanup": true
    },
    {
      "name": "radarr",
      "host": "http://radarr:7878",
      "token": "YOUR_RADARR_API_KEY",
      "cleanup": true
    }
  ],

  "usenet": {
    "providers": [
      {
        "host": "news.provider1.com",
        "port": 563,
        "username": "YOUR_NNTP_USERNAME",
        "password": "YOUR_NNTP_PASSWORD",
        "backbone": "Provider1Backbone",
        "max_connections": 15,
        "ssl": true,
        "priority": 1
      },
      {
        "host": "news.provider2.com",
        "port": 563,
        "username": "YOUR_NNTP_USERNAME",
        "password": "YOUR_NNTP_PASSWORD",
        "backbone": "Provider2Backbone",
        "max_connections": 10,
        "ssl": true,
        "priority": 1
      }
    ],
    "max_connections": 15,
    "read_ahead": "16MB",
    "processing_timeout": "10m",
    "availability_sample_percent": 10,
    "max_concurrent_nzb": 2,
    "disk_buffer_path": "/mnt/arr-cache/decypharr-streams"
  },

  "mount": {
    "type": "rclone",
    "mount_path": "/mnt/decypharr",
    "rclone": {
      "port": "5572",
      "cache_dir": "/mnt/arr-cache/decypharr-cache",
      "vfs_cache_mode": "full",
      "vfs_cache_max_age": "12h",
      "vfs_cache_max_size": "40G",
      "vfs_cache_poll_interval": "1m",
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
      "timeout": "5m",
      "connect_timeout": "30s",
      "daemon_timeout": "30s",
      "log_level": "INFO"
    }
  },

  "repair": {
    "enabled": true,
    "source": "arr",
    "schedule": "0 3 * * *",
    "workers": 5,
    "nntp_connection_percent": 20,
    "recheck_interval": "168h",
    "auto_repair": true
  }
}
```

> **`usenet_backend: "nntp"`** on the TorBox entry forces all NZBs through your own NNTP providers. Change to `"auto"` if you have a TorBox Pro plan and want TorBox to handle NZB downloading when slots are available (falls back to NNTP automatically).

---

## 7. Docker Compose

Save the following as `/opt/docker/arrstack/docker-compose.yml`:

```yaml
services:
  # ---------------------------------------------------------------------------
  # PostgreSQL — Database for Zilean
  # ---------------------------------------------------------------------------
  postgres:
    image: postgres:16-alpine
    container_name: postgres
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: zilean
      POSTGRES_DB: zilean
    volumes:
      - /opt/docker/arrstack/postgres:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 10s
      timeout: 5s
      retries: 5

  # ---------------------------------------------------------------------------
  # PostgreSQL — Dedicated database for Bitmagnet (on SSD to reduce I/O on root)
  # ---------------------------------------------------------------------------
  bitmagnet-db:
    image: postgres:16-alpine
    container_name: bitmagnet-db
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: bitmagnet
      POSTGRES_DB: bitmagnet
    volumes:
      - /mnt/arr-cache/bitmagnet-db:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 10s
      timeout: 5s
      retries: 5

  # ---------------------------------------------------------------------------
  # Bitmagnet — DHT crawler and Torznab indexer
  # Requires a free TMDb API key: https://www.themoviedb.org/settings/api
  # ---------------------------------------------------------------------------
  bitmagnet:
    image: ghcr.io/bitmagnet-io/bitmagnet:latest
    container_name: bitmagnet
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    command: worker run --all=true
    depends_on:
      bitmagnet-db:
        condition: service_healthy
    environment:
      - POSTGRES_HOST=bitmagnet-db
      - POSTGRES_PORT=5432
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=bitmagnet
      - POSTGRES_DATABASE=bitmagnet
      - HTTP_SERVER_PORT=3333
      - TMDB_API_KEY=YOUR_TMDB_API_KEY
      - DHT_CRAWLER_SAVE_FILES_THRESHOLD=0
      - DHT_CRAWLER_SAVE_PIECES=false
    ports:
      - "3333:3333"
      - "9596:9596/tcp"
      - "9596:9596/udp"
    volumes:
      - /opt/docker/arrstack/bitmagnet:/root/.config/bitmagnet

  # ---------------------------------------------------------------------------
  # Zilean — DMM/Torrentio scrape cache (speeds up Prowlarr searches)
  # ---------------------------------------------------------------------------
  zilean:
    image: ipromknight/zilean:latest
    container_name: zilean
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      - ConnectionStrings__Database=Host=postgres;Port=5432;Database=zilean;Username=postgres;Password=zilean
      - UMASK=002
    ports:
      - "8181:8181"
    volumes:
      - /opt/docker/arrstack/zilean:/app/data

  # ---------------------------------------------------------------------------
  # Decypharr — Debrid gateway + NNTP streaming + rclone VFS mount
  #
  # Built from source (TwistedRat fork). The build is cached by Docker — only
  # rebuilds when the compose file changes or you run 'docker compose build'.
  #
  # PUID/PGID=0: runs as root inside container. Required for FUSE in LXC.
  # privileged: required for FUSE mount operations inside the container.
  # /mnt/decypharr :rshared — mount propagation so Radarr/Sonarr/Plex see
  # the rclone VFS even though it is mounted inside the Decypharr container.
  # ---------------------------------------------------------------------------
  decypharr:
    build: https://github.com/TwistedRat/decypharr.git
    image: decypharr
    container_name: decypharr
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    privileged: true
    cap_add:
      - SYS_ADMIN
    devices:
      - /dev/fuse:/dev/fuse:rwm
    environment:
      - PUID=0
      - PGID=0
      - LANG=en_US.UTF-8
      - LC_ALL=en_US.UTF-8
    ports:
      - "8282:8282"
    volumes:
      - /opt/docker/arrstack/decypharr:/app
      - /mnt/decypharr:/mnt/decypharr:rshared
      - /srv/media:/srv/media:rshared
      - /mnt/arr-cache/decypharr-cache:/mnt/arr-cache/decypharr-cache
      - /mnt/arr-cache/decypharr-streams:/mnt/arr-cache/decypharr-streams
    healthcheck:
      test: ["CMD-SHELL", "mountpoint -q /mnt/decypharr"]
      interval: 10s
      timeout: 5s
      retries: 3

  # ---------------------------------------------------------------------------
  # Prowlarr — Indexer manager (NZB + torrent indexers)
  # Add Decypharr as a SABnzbd download client, not as a separate indexer.
  # ---------------------------------------------------------------------------
  prowlarr:
    image: lscr.io/linuxserver/prowlarr:latest
    container_name: prowlarr
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    depends_on:
      - zilean
      - bitmagnet
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=YOUR_TIMEZONE
    ports:
      - "9696:9696"
    volumes:
      - /opt/docker/arrstack/prowlarr:/config

  # ---------------------------------------------------------------------------
  # Radarr — Movie automation
  # Mounts /mnt/decypharr and /srv/media as rslave so it sees Decypharr's VFS.
  # ---------------------------------------------------------------------------
  radarr:
    image: lscr.io/linuxserver/radarr:latest
    container_name: radarr
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    stop_grace_period: 1m
    privileged: true
    depends_on:
      decypharr:
        condition: service_healthy
    environment:
      - PUID=1000
      - PGID=1000
      - UMASK=002
      - TZ=YOUR_TIMEZONE
    ports:
      - "7878:7878"
    volumes:
      - /opt/docker/arrstack/radarr:/config
      - /mnt/arr-cache:/mnt/arr-cache
      - /mnt/decypharr:/mnt/decypharr:rslave
      - /srv/media:/srv/media:rslave

  # ---------------------------------------------------------------------------
  # Sonarr — TV show automation
  # ---------------------------------------------------------------------------
  sonarr:
    image: lscr.io/linuxserver/sonarr:latest
    container_name: sonarr
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    stop_grace_period: 1m
    privileged: true
    depends_on:
      decypharr:
        condition: service_healthy
    environment:
      - PUID=1000
      - PGID=1000
      - UMASK=002
      - TZ=YOUR_TIMEZONE
    ports:
      - "8989:8989"
    volumes:
      - /opt/docker/arrstack/sonarr:/config
      - /mnt/arr-cache:/mnt/arr-cache
      - /mnt/decypharr:/mnt/decypharr:rslave
      - /srv/media:/srv/media:rslave

  # ---------------------------------------------------------------------------
  # Bazarr — Subtitle downloader
  # ---------------------------------------------------------------------------
  bazarr:
    image: lscr.io/linuxserver/bazarr:latest
    container_name: bazarr
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    privileged: true
    depends_on:
      - radarr
      - sonarr
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=YOUR_TIMEZONE
    ports:
      - "6767:6767"
    volumes:
      - /opt/docker/arrstack/bazarr:/config
      - /mnt/decypharr:/mnt/decypharr:rslave
      - /srv/media/movies:/srv/media/movies:rslave
      - /srv/media/shows:/srv/media/shows:rslave

  # ---------------------------------------------------------------------------
  # Lingarr — Subtitle translation
  # ---------------------------------------------------------------------------
  lingarr:
    image: lingarr/lingarr:latest
    container_name: lingarr
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    privileged: true
    depends_on:
      - radarr
      - sonarr
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=YOUR_TIMEZONE
      - ASPNETCORE_URLS=http://+:9876
    ports:
      - "9876:9876"
    volumes:
      - /opt/docker/arrstack/lingarr:/app/config
      - /mnt/decypharr:/mnt/decypharr:rslave
      - /srv/media/movies:/srv/media/movies:rslave
      - /srv/media/shows:/srv/media/shows:rslave

  # ---------------------------------------------------------------------------
  # Seerr — Media request management (Overseerr fork)
  # ---------------------------------------------------------------------------
  seerr:
    image: ghcr.io/seerr-team/seerr:latest
    container_name: seerr
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    depends_on:
      - sonarr
      - radarr
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=YOUR_TIMEZONE
    ports:
      - "5055:5055"
    volumes:
      - /opt/docker/arrstack/seerr:/app/config

  # ---------------------------------------------------------------------------
  # Maintainerr — Library cleanup automation
  # ---------------------------------------------------------------------------
  maintainerr:
    image: ghcr.io/maintainerr/maintainerr:latest
    container_name: maintainerr
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    user: "1000:1000"
    depends_on:
      - seerr
      - radarr
      - sonarr
    environment:
      - TZ=YOUR_TIMEZONE
    ports:
      - "6246:6246"
    volumes:
      - /opt/docker/arrstack/maintainerr:/opt/data

  # ---------------------------------------------------------------------------
  # Byparr — Cloudflare / DDoS-GUARD bypass proxy for Prowlarr
  # Internal only — not exposed on host network.
  # In Prowlarr: Settings → Indexers → FlareSolverr URL = http://byparr:8191
  # ---------------------------------------------------------------------------
  byparr:
    image: ghcr.io/thephaseless/byparr:latest
    container_name: byparr
    restart: unless-stopped
    security_opt:
      - apparmor:unconfined
    environment:
      - TZ=YOUR_TIMEZONE
      - PORT=8191

  # ---------------------------------------------------------------------------
  # Cleanuparr — Monitors queues and removes stalled / DMCA-blocked downloads
  # ---------------------------------------------------------------------------
  cleanuparr:
    image: ghcr.io/cleanuparr/cleanuparr:latest
    container_name: cleanuparr
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    depends_on:
      - radarr
      - sonarr
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=YOUR_TIMEZONE
    ports:
      - "11011:11011"
    volumes:
      - /opt/docker/arrstack/cleanuparr:/config

  # ---------------------------------------------------------------------------
  # Tautulli — Plex monitoring and statistics
  # ---------------------------------------------------------------------------
  tautulli:
    image: lscr.io/linuxserver/tautulli:latest
    container_name: tautulli
    security_opt:
      - apparmor:unconfined
    restart: unless-stopped
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=YOUR_TIMEZONE
    ports:
      - "8978:8181"
    volumes:
      - /opt/docker/arrstack/tautulli:/config

  # ---------------------------------------------------------------------------
  # Plex Media Server
  #
  # PLEX_CLAIM: one-time claim token from https://plex.tv/claim
  #   Required only on the very first start to link the server to your account.
  #   Leave empty after the server has been claimed.
  #
  # ADVERTISE_IP: the LAN IP:port Plex advertises to clients.
  #   Change YOUR_LAN_IP to the container's LAN IP address.
  #
  # iGPU devices: exposed via LXC config (see section 2a).
  #   card0 + renderD128 enable VAAPI hardware transcoding (H.264/HEVC).
  # ---------------------------------------------------------------------------
  plex:
    image: plexinc/pms-docker:latest
    container_name: plex
    restart: unless-stopped
    stop_grace_period: 60s
    security_opt:
      - apparmor:unconfined
    depends_on:
      decypharr:
        condition: service_healthy
    environment:
      - PLEX_UID=1000
      - PLEX_GID=1000
      - TZ=YOUR_TIMEZONE
      - PLEX_CLAIM=
      - ADVERTISE_IP=http://YOUR_LAN_IP:32400
    ports:
      - "32400:32400/tcp"
    devices:
      - /dev/dri/card0:/dev/dri/card0
      - /dev/dri/renderD128:/dev/dri/renderD128
    volumes:
      - /opt/docker/arrstack/plex:/config
      - /srv/media/movies:/srv/media/movies:rslave
      - /srv/media/shows:/srv/media/shows:rslave
      - /srv/media/anime:/srv/media/anime:rslave
      - /mnt/decypharr:/mnt/decypharr:rslave
```

Replace every `YOUR_*` placeholder before starting.

---

## 8. start.sh and stop.sh

Save both scripts to `/opt/docker/arrstack/` and make them executable:

```bash
chmod +x /opt/docker/arrstack/start.sh /opt/docker/arrstack/stop.sh
```

### start.sh

```bash
#!/bin/bash
echo "=== Starting arr-stack ==="

# Step 1: Clear stale rclone VFS cache databases.
# rclone's VFS cache stores file metadata in bolt DBs inside cache_dir.
# If decypharr was killed uncleanly these can be corrupt, causing rclone
# to fail on mount. Wiping them forces a clean rebuild on every start.
# The actual cached file data (in decypharr-cache/) is preserved so Plex
# does not have to re-download everything; only the index is cleared.
echo "--> Clearing stale VFS cache databases..."
rm -rf /mnt/arr-cache/decypharr-cache/*
rm -rf /opt/docker/arrstack/decypharr/cache/*

# Step 2: Start Decypharr first and wait for it to mount.
# Decypharr runs rclone internally and mounts the debrid filesystem at
# /mnt/decypharr. All other services (Radarr, Sonarr, Plex) mount
# /mnt/decypharr as rslave — if the mount is not ready when they start,
# those services will see an empty directory and fail to find media.
# The healthcheck (mountpoint -q /mnt/decypharr) prevents dependent
# containers from starting until the mount is confirmed live, but the
# 120-second sleep gives rclone time to authenticate with the debrid
# provider, build the initial directory tree, and settle before any
# client tries to stat files.
echo "--> Starting Decypharr (rclone mount)..."
cd /opt/docker/arrstack
docker compose up -d decypharr

echo "Waiting 120s for rclone mount to stabilise..."
sleep 120

# Step 3: Start the rest of the stack.
# --remove-orphans cleans up any containers left over from a previous
# compose file that had more services than the current one.
echo "--> Starting remaining services..."
docker compose up -d --remove-orphans

echo "=== Stack is up ==="
```

### stop.sh

```bash
#!/bin/bash
# stop.sh — Gracefully stop the arr-stack with fallback kill and Docker restart.
#
# Usage:
#   ./stop.sh           Normal stop
#   ./stop.sh --clean   Normal stop + wipe containerd image/snapshot stores
#                       (reclaims disk space; images will be re-pulled on next start)

COMPOSE_DIR="/opt/docker/arrstack"
RCLONE_MOUNT="/mnt/decypharr"
TIMEOUT=30   # seconds to wait for containers to stop before force-killing

CLEAN=false
for arg in "$@"; do
    case "$arg" in
        --clean) CLEAN=true ;;
    esac
done

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Stopping arr-stack..."

# Step 0: Kill any running maintenance scripts.
# Scripts like arr_fsck.sh or plex_sync.sh hold file handles inside the
# rclone VFS. If they are running when the mount is torn down they will enter
# D-state (uninterruptible sleep), preventing the mount from being cleanly
# unmounted. Kill them before stopping any containers.
for SCRIPT in plex_sync.sh arr_fsck.sh; do
    PIDS=$(pgrep -f "$SCRIPT" 2>/dev/null)
    if [ -n "$PIDS" ]; then
        echo "Killing background $SCRIPT: PIDs $PIDS"
        kill -9 $PIDS 2>/dev/null || true
    fi
done

# Step 1: Normal compose down.
# docker compose down sends SIGTERM to each container, waits up to $TIMEOUT
# seconds, then sends SIGKILL. Services with stop_grace_period (Radarr, Sonarr,
# Plex) get their grace period honoured here so databases are flushed cleanly.
cd "$COMPOSE_DIR" || { echo "ERROR: Cannot cd to $COMPOSE_DIR"; exit 1; }
docker compose down --timeout "$TIMEOUT"
COMPOSE_EXIT=$?

if [ $COMPOSE_EXIT -eq 0 ]; then
    echo "All containers stopped cleanly."
else
    echo "WARNING: 'docker compose down' exited $COMPOSE_EXIT — checking for stragglers..."
fi

# Step 2: Find and force-kill any containers still running from this project.
# Occasionally a container ignores SIGTERM and survives compose down
# (e.g. Plex writing a large DB transaction). We identify survivors by the
# Docker Compose project label rather than by name so this is robust to
# compose file renames.
PROJECT_NAME=$(basename "$COMPOSE_DIR")
RUNNING=$(docker ps --filter "label=com.docker.compose.project=$PROJECT_NAME" -q)

if [ -n "$RUNNING" ]; then
    echo "Containers still running: $RUNNING — force-killing..."
    docker kill $RUNNING 2>/dev/null || true
    sleep 3

    STILL_RUNNING=$(docker ps --filter "label=com.docker.compose.project=$PROJECT_NAME" -q)
    if [ -n "$STILL_RUNNING" ]; then
        # Last resort: restart the Docker daemon itself.
        # This handles the rare case where dockerd has a zombie container it
        # cannot kill (e.g. a FUSE mount inside the container has wedged the
        # kernel's container teardown path). We stop dockerd with a 10-second
        # timeout before SIGKILL to avoid waiting the full systemd default (90s).
        echo "Containers still alive — restarting Docker daemon..."
        systemctl stop docker &
        STOP_PID=$!
        sleep 10
        if kill -0 $STOP_PID 2>/dev/null; then
            systemctl kill -s SIGKILL docker 2>/dev/null || true
            wait $STOP_PID 2>/dev/null || true
        fi
        systemctl start docker
        for i in $(seq 1 20); do
            docker info &>/dev/null && echo "Docker is back up." && break
            sleep 1
        done
    else
        echo "All containers killed successfully."
    fi
else
    echo "No straggler containers found."
fi

# Step 3: Unmount the rclone FUSE mount.
# Always runs regardless of container stop outcome. Uses lazy unmount (-l) so
# the call succeeds even if a process still has the mount open — the kernel
# will finish the unmount once all file handles are closed.
echo "Unmounting $RCLONE_MOUNT..."
umount -l "$RCLONE_MOUNT" 2>/dev/null
echo "Unmount done."

# Step 4: Remove the Decypharr container to clear FUSE overlay accumulation.
# The rclone FUSE mount inside the container causes Docker's writable overlay
# layer (overlayfs) to accumulate inode metadata over time. Removing the
# container on every stop wipes the overlay; start.sh recreates it fresh via
# 'docker compose up -d'. This is the same as 'docker rm' — it does NOT
# delete any persistent data because all state lives in the /app volume mount.
echo "Removing decypharr container overlay..."
docker compose rm -f decypharr 2>/dev/null || true
echo "Overlay cleared."

# Step 5 (optional --clean): Wipe containerd snapshot and content stores.
# Run this when disk space is low or after upgrading images to force a full
# re-pull. This removes ALL cached Docker image layers — every image will be
# downloaded again on the next start.sh run.
if [ "$CLEAN" = true ]; then
    echo "--clean: wiping containerd stores..."
    systemctl stop containerd
    rm -rf /var/lib/containerd/io.containerd.snapshotter.v1.overlayfs \
           /var/lib/containerd/io.containerd.content.v1.content
    systemctl start containerd
    echo "Containerd stores wiped."
fi

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Stack stopped."
```

### Why this start/stop order matters

The stack has an unusual dependency chain driven by the rclone FUSE mount:

```
Decypharr (rclone mounts /mnt/decypharr)
    └── Radarr, Sonarr, Plex (mount /mnt/decypharr as rslave)
            └── Bazarr, Lingarr, Seerr, Maintainerr, ...
```

**On start:** if Radarr or Plex start before `/mnt/decypharr` is populated, they see an empty directory, cache an empty file list, and will not find any media until manually refreshed. The 120-second wait gives rclone time to authenticate with the debrid provider and build the initial directory tree before any client inspects it.

**On stop:** if the FUSE mount is still active when Docker removes the Decypharr container, the kernel leaves the mount in a disconnected state (`Transport endpoint is not connected`). The next `docker compose up` then fails because Docker tries to bind-mount a path that is in this broken state. `umount -l` in stop.sh tears it down cleanly every time.

---

## 9. Initial Build and First Start

The first start builds the Decypharr image from source, which takes 3–5 minutes:

```bash
cd /opt/docker/arrstack

# Build Decypharr image (only needed once, or after fork updates)
docker compose build decypharr

# Start the full stack
./start.sh
```

Monitor the startup:

```bash
# Watch all container states
watch docker ps

# Follow Decypharr logs (look for "Successfully mounted rclone filesystem")
docker logs -f decypharr

# Confirm mount is live
mountpoint /mnt/decypharr && ls /mnt/decypharr
```

---

## 10. Arr Download Client Setup

### Decypharr as a qBittorrent client (torrents via Debrid)

In Radarr/Sonarr → Settings → Download Clients → Add → **qBittorrent**:

| Setting | Value |
|---|---|
| Name | `Decypharr` |
| Host | `decypharr` (Docker service name) or container LAN IP |
| Port | `8282` |
| Category | `radarr` or `sonarr` (must match `categories` in config.json) |

### Decypharr as a SABnzbd client (NZB / Usenet)

In Radarr/Sonarr → Settings → Download Clients → Add → **SABnzbd**:

| Setting | Value |
|---|---|
| Name | `Decypharr NZB` |
| Host | `decypharr` |
| Port | `8282` |
| URL Base | `/sabnzbd` |
| Username | Full URL of this Arr instance (e.g. `http://sonarr:8989`) |
| Password | This Arr's API key |
| Category | `radarr` or `sonarr` |

> The Username/Password fields are not used for authentication — Decypharr uses them internally to route completed NZB imports back to the correct Arr instance.

### Prowlarr → Byparr (Cloudflare bypass)

In Prowlarr → Settings → Indexers → FlareSolverr:

| Setting | Value |
|---|---|
| URL | `http://byparr:8191` |

### Prowlarr → Zilean (DMM scrape cache)

Add Zilean as a Prowlarr indexer under Torznab:

| Setting | Value |
|---|---|
| URL | `http://zilean:8181` |

### Prowlarr → Bitmagnet (DHT Torznab)

Add Bitmagnet as a Torznab indexer:

| Setting | Value |
|---|---|
| URL | `http://bitmagnet:3333/torznab` |

---

## 11. Plex Hardware Transcoding

After first start, log into Plex at `http://YOUR_LAN_IP:32400/web` and enable hardware transcoding:

Settings → Transcoder → **Use hardware acceleration when available** → Save

Verify the iGPU is visible inside the Plex container:

```bash
docker exec plex ls /dev/dri/
```

You should see `card0` and `renderD128`. If not, re-check the LXC config from section 2a and restart the container.

---

## 12. Updating Decypharr

To pull and deploy a new version of the fork:

```bash
cd /opt/docker/arrstack
./stop.sh
docker compose build --no-cache decypharr
./start.sh
```

The `--no-cache` flag forces Docker to re-clone the repository. Omit it to use the BuildKit cache (faster, but may not pick up the very latest commit).

---

## 13. Troubleshooting

### `/mnt/decypharr` is empty after start

Decypharr did not finish mounting before the other services started. Check:

```bash
docker logs decypharr | grep -i "mount\|error\|rclone"
```

If you see `transport endpoint is not connected`, run `./stop.sh` then `./start.sh` — stop.sh includes a lazy unmount that clears this state.

### Plex cannot find media / shows "unavailable"

The rslave mounts in Radarr/Sonarr/Plex only work if `/mnt/decypharr` was already mounted when those containers started. Always use `./start.sh` rather than `docker compose up -d` directly.

### Decypharr returns 429 errors to Radarr/Sonarr

Decypharr is hitting the debrid provider's rate limit. Reduce `rate_limit` in config.json, or add a second debrid provider to spread load.

### NZB downloads fail with "article not found"

The Usenet article segments have expired or been taken down on all configured providers. Decypharr will report this as a permanent failure and trigger a Sonarr/Radarr re-search automatically. If all providers consistently fail for new content, check provider retention and consider adding a provider with a different backbone.

### `docker compose build` is slow

The build uses Docker BuildKit. On the first build it clones the full Go module cache; subsequent builds reuse it. If the cache is corrupted, run `./stop.sh --clean` then rebuild.
