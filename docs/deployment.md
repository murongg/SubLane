# Deployment and upgrades

SubLane runs as one process with an embedded frontend and SQLite. Production does not require Node.js, Redis or a separate proxy service. Run one instance against each data directory.

## Published Docker images

Tagged releases publish `ghcr.io/murongg/sublane` for `linux/amd64` and `linux/arm64`. Docker selects the host architecture. Images become available after the first successful release workflow; before that, use the local source build below.

| Tag | Meaning |
| --- | --- |
| `0.1.0` | Example of a fixed stable version; use an actually published version |
| `0.1.0-rc.1` | Example of a fixed prerelease; does not update `latest` |
| `latest` | Newest successfully promoted stable version |
| `0.1.0-amd64` / `0.1.0-arm64` | Per-architecture release images |

Use a fixed version or digest for repeatable deployments. A new image appearing in GHCR does not update a running container automatically.

Download `docker.compose.yaml` from the selected [GitHub Release](https://github.com/murongg/SubLane/releases). In the directory containing it, create a Compose `.env` file with your chosen published version:

```dotenv
SUBLANE_IMAGE=ghcr.io/murongg/sublane:0.1.0
SUBLANE_BIND_ADDRESS=127.0.0.1
SUBLANE_PORT=8080
SUBLANE_LOG_LEVEL=info
# Add this when using an HTTPS reverse proxy:
# SUBLANE_PUBLIC_URL=https://sublane.example.com
```

Here `0.1.0` is an example, not a claim that the release already exists. Compose reads `.env` for interpolation; the standalone Go application does not load `.env` itself. An immutable digest can be used as the entire `SUBLANE_IMAGE` value instead of a tag.

```sh
docker compose -f docker.compose.yaml pull
docker compose -f docker.compose.yaml up -d
docker compose -f docker.compose.yaml ps
docker compose -f docker.compose.yaml logs --tail=100 sublane
```

Open `http://127.0.0.1:8080` and complete administrator setup before allowing team access. For a remote host, use an SSH tunnel for initial setup. The default Compose mapping stays on loopback; publishing `0.0.0.0` is an explicit operator choice.

The image runs as the existing Alpine `sublane` service account, includes CA certificates and license notices, and checks `/readyz`. Compose uses a read-only root filesystem, a writable 64 MiB `/tmp` tmpfs for SDK scratch files, dropped Linux capabilities, bounded container logs and a 30-second stop grace period. The container listens on port 8080; customize the host port through `SUBLANE_PORT`. If overriding the internal listening port yourself, also change the health check and port mapping.

### Persistent data

The named `sublane-data` volume holds `/data/sublane.db`, its WAL files and `/data/credentials.key`. Keep the encryption key with the database. The existing Compose service/volume keys are preserved, so upgrades from earlier source builds keep using the same volume when run from the same Compose project.

Keep the Compose project name/directory consistent. Changing it creates a different named volume and can make an existing instance appear uninitialized. Do not run `docker compose -f docker.compose.yaml down -v` when upgrading or recovering: that removes the data volume. Bind mounts are supported, but their host permissions must let the image's non-root account write the mounted directory. Prefer the default named volume unless host path ownership is intentional.

### Build from source

The source-build override keeps local builds separate from the published image selection:

```sh
docker compose -f docker.compose.yaml -f docker.compose.build.yaml up --build -d
```

Use both files for subsequent commands against that deployment. To set build metadata:

```sh
VERSION=0.1.0-dev REVISION=$(git rev-parse HEAD) \
  docker compose -f docker.compose.yaml -f docker.compose.build.yaml build
```

The Dockerfile compiles Go for `TARGETOS`/`TARGETARCH` from native build stages, so building ARM64 on x86-64 does not emulate the compiler or frontend build. A multi-platform image can be built with a configured Buildx builder:

```sh
docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=0.1.0-dev --output type=oci,dest=dist/sublane.oci.tar .
```

Create `dist` first. A regular local build/load normally targets one platform. See Docker's [cross-compilation guidance](https://docs.docker.com/build/building/multi-platform/#cross-compilation).

## Standalone Linux binary

Download the archive matching your machine and `SHA256SUMS` from the same release:

- `sublane_VERSION_linux_amd64.tar.gz`: x86-64.
- `sublane_VERSION_linux_arm64.tar.gz`: AArch64.

On Linux, verify the downloaded files, extract the archive into a new versioned directory, and run:

```sh
sha256sum --check --ignore-missing SHA256SUMS
./sublane --version
SUBLANE_ADDR=127.0.0.1:8080 SUBLANE_DATA_DIR=/srv/sublane/data ./sublane
```

The archive contains the executable, AGPL and third-party license texts, the Compose file, an environment example and deployment/backup documentation. Keep the data directory outside the versioned executable directory, make it private and writable by the service user, and use a process supervisor for persistent operation. Do not run the gateway as root.

## Runtime configuration

The standalone service reads these process environment variables; `.env.example` lists examples. It does not load a `.env` file automatically. Compose reads `.env` for its own interpolation and passes the configured environment into the container.

| Variable | Standalone default | Purpose |
| --- | --- | --- |
| `SUBLANE_ADDR` | `127.0.0.1:8080` | HTTP listening address |
| `SUBLANE_DATA_DIR` | `./data` | SQLite database and encryption-key directory |
| `SUBLANE_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `SUBLANE_PUBLIC_URL` | Unset | Exact external origin; HTTPS enables secure session cookies |

The Docker image overrides the listen address to `0.0.0.0:8080` and data directory to `/data`. `SUBLANE_IMAGE`, `SUBLANE_BIND_ADDRESS` and `SUBLANE_PORT` configure Compose only. SQLite applies ordered migrations at startup and uses a single database connection.

## HTTPS reverse proxy

Set `SUBLANE_PUBLIC_URL` to the exact external origin, such as `https://sublane.example.com`, and recreate the service so the environment changes take effect. This origin is used for browser request checks and secure session cookies. Forward the original Host header and preserve streaming and WebSocket upgrades.

For a Caddy proxy running on the same host:

```caddyfile
sublane.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

Caddy handles TLS and WebSocket upgrades and flushes SSE responses automatically. Keep the default flush behavior so client disconnects can cancel upstream work; negative `flush_interval` changes that cancellation behavior. See the [Caddy reverse proxy reference](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy#streaming). If the proxy is another container, use the Compose service name `sublane:8080` on a shared network; `127.0.0.1` inside the proxy container refers to that container itself.

For an existing Nginx installation, put the map in the `http` context and use these locations inside its HTTPS server block:

```nginx
map $http_upgrade $sublane_connection {
    default upgrade;
    ''      close;
}

location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $http_host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $sublane_connection;
    proxy_buffering off;
    proxy_read_timeout 650s;
    proxy_send_timeout 650s;
    client_max_body_size 8m;
}

# Web backup uploads/downloads can be larger and last up to 15 minutes.
location /api/settings/backup/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $http_host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_request_buffering off;
    proxy_buffering off;
    proxy_read_timeout 950s;
    proxy_send_timeout 950s;
    client_max_body_size 256m;
}
```

Configure the HTTPS certificate in the parent Nginx server. The [Nginx WebSocket guide](https://nginx.org/en/docs/http/websocket.html) explains the explicit upgrade headers. Limits must also permit the request/backup sizes at any other proxy layer. The gateway's own authorization and body limits remain in force.

## Upgrade

1. Review the release notes and record the current image version/digest.
2. Export a consistent backup and copy it off the data volume:

   ```sh
   docker compose -f docker.compose.yaml exec sublane sh -c 'mkdir -p /data/backups'
   docker compose -f docker.compose.yaml exec sublane sublane backup --output /data/backups/pre-upgrade.sublane-backup.tar.gz
   docker compose -f docker.compose.yaml exec sublane sublane backup verify --input /data/backups/pre-upgrade.sublane-backup.tar.gz
   docker compose -f docker.compose.yaml cp sublane:/data/backups/pre-upgrade.sublane-backup.tar.gz ./pre-upgrade.sublane-backup.tar.gz
   chmod 600 ./pre-upgrade.sublane-backup.tar.gz
   ```

   Use a new filename for each upgrade; existing backups are never overwritten. The archive includes the encryption key and belongs in private storage.
3. Change `SUBLANE_IMAGE` in `.env` to the selected version, then pull and recreate:

   ```sh
   docker compose -f docker.compose.yaml pull
   docker compose -f docker.compose.yaml up -d
   docker compose -f docker.compose.yaml ps
   docker compose -f docker.compose.yaml logs --tail=100 sublane
   ```

4. Confirm health, sign-in, expected accounts/groups/keys, and one explicitly authorized client call. SQLite migrations run at startup. Image health verifies process/database readiness, not real provider authorization.

### Recovery and rollback

Changing an image tag alone is not a database rollback. If a release migrated the schema, restore the pre-upgrade archive into a new directory with the intended older binary/image, then stop the current service and switch to that restored directory. Retain the original volume until verification is complete. Never let two processes share the same SQLite directory.

The [backup guide](backup.md) covers CLI and web restore preparation and the Compose override for `/data/restore-ready`. Continue using the restore override on later starts while that directory is active. A backup from a newer schema cannot be opened by an older release that does not recognize it.

## Container verification

With a Docker engine running:

```sh
docker compose -f docker.compose.yaml config --quiet
docker compose -f docker.compose.yaml -f docker.compose.build.yaml config --quiet
docker buildx build --load --build-arg VERSION=0.0.0-test -t sublane:smoke .
bash scripts/container-smoke.sh sublane:smoke 0.0.0-test
```

The smoke script creates its own disposable named volume and container, with networking disabled. It checks health, version, non-root execution, read-only runtime paths, first-run setup with synthetic credentials, key permissions, backup/verify/restore and persistence after restart. It removes only those temporary resources. It does not contact upstream providers or use existing instance data. CI runs the same checks on native amd64 and arm64 runners.
