# Backup and restore

SubLane can export a running instance, verify a backup, and restore it into a new directory. Both the command line and **System settings → Backup and restore** use the same archive validation and restore implementation. Recovery never overwrites the running instance; switching to restored data is an explicit operator restart.

## What is included

A backup is a gzip-compressed tar archive containing exactly:

- `sublane.db`: a consistent SQLite snapshot, including committed WAL data.
- `credentials.key`: the matching encryption key for subscription credentials and recoverable gateway keys.
- `manifest.json`: format version, creation time, SubLane build version, schema version, file sizes and SHA-256 checksums.

Users, password hashes, API keys, subscription accounts, groups, permissions, catalogs, settings, request records, usage summaries, cooldowns and affinity records are included. Browser sessions are captured in the backup but cleared in a restored directory. Browser-local theme/language choices, process environment, deployment files and pending in-memory OAuth attempts are not included.

**The archive contains the encryption key and is not password-encrypted.** Anyone with a complete archive can recover its stored credentials. Keep it in private storage; use encrypted storage when required. Generated archive files use mode `0600`, and staging/restored directories use `0700` on Unix. Browser downloads follow the browser and operating system's destination permissions. SHA-256 checks detect corruption; they do not authenticate an untrusted backup's author.

## Command line

Run the commands from a directory with enough free space for the database snapshot and compressed archive:

```sh
mkdir -p backups
./bin/sublane backup --output ./backups/team.sublane-backup.tar.gz
./bin/sublane backup verify --input ./backups/team.sublane-backup.tar.gz
./bin/sublane restore --input ./backups/team.sublane-backup.tar.gz --data-dir ./data-restored
```

`backup` reads `SUBLANE_DATA_DIR` or `./data`, with an optional `--data-dir` override. `restore` always requires an explicit new directory; even an existing empty directory is rejected. The parent directory must exist. Existing archive files and restore directories are never replaced. Successful commands print a JSON result with the operation, path and backup metadata. `sublane --help` lists the commands.

Maintenance commands do not start the HTTP server, SDK executors, OAuth, or automatic release checks, and do not depend on server address/public-URL configuration. They have a fifteen-minute deadline and respond to SIGINT/SIGTERM. Database snapshots are limited to 4 GiB and archive IO uses bounded buffers. Allow disk space for extraction and migrations when verifying or restoring, in addition to the archive itself.

After a successful restore:

1. Stop the current SubLane service.
2. Keep its original data directory intact.
3. Start SubLane with the restored directory:

```sh
SUBLANE_DATA_DIR=./data-restored ./bin/sublane
```

4. Sign in again and check the expected users, groups, keys and account states before resuming team traffic.

The original directory is available for rollback by stopping SubLane and switching `SUBLANE_DATA_DIR` back. Do not run two gateway instances against the same data directory. Restoring rolls back all stored state to the snapshot: later key revocations, password changes and new records are not included. A subscription provider may have invalidated older refresh tokens since the backup; reauthorize affected accounts when necessary. Verification checks decryption and local integrity without contacting providers.

## Web interface

Only enabled administrators can use the page or its endpoints. Mutations require the exact browser origin, and export rechecks the administrator session after preparing the file.

- **Download backup** creates and downloads a complete archive while the instance remains available.
- **Verify backup** checks the selected file and displays its creation time, SubLane version and database size.
- **Prepare restore** uploads and validates the file again, then creates `<current data directory>/restore-ready`. The page shows the full path and restart instructions. The current instance keeps running with its original data until the operator switches directories.

An existing `restore-ready` path blocks another preparation. Inspect it on the server or use the CLI with a different new target. No directory can be overwritten through the web API. If recovery data was prepared but the live audit write failed, the directory is retained and the page reports the audit failure; inspect it before retrying.

Web uploads and downloads are limited to 256 MiB of compressed data; larger backups require the CLI. One export/verification/restore operation runs at a time per server. Upload, processing and download have bounded deadlines. Temporary files are removed after completion, failure or normal cancellation. A forced process kill or power loss may leave a private `.sublane-backup-*` directory; remove leftover staging directories only when no backup operation is running.

Exports and prepared restores are audited without recording archive contents, filenames, filesystem paths or credentials. Downloads use `Cache-Control: no-store`. The page streams uploads from the selected file; the browser receives the downloaded archive as a temporary blob and releases its URL after starting the download.

## Docker Compose

To export and verify within the running container:

```sh
docker compose -f docker.compose.yaml exec sublane sh -c 'mkdir -p /data/backups'
docker compose -f docker.compose.yaml exec sublane sublane backup --output /data/backups/team.sublane-backup.tar.gz
docker compose -f docker.compose.yaml exec sublane sublane backup verify --input /data/backups/team.sublane-backup.tar.gz
docker compose -f docker.compose.yaml cp sublane:/data/backups/team.sublane-backup.tar.gz ./team.sublane-backup.tar.gz
```

For recovery, copy a backup into a private writable location visible inside the container, then prepare a separate directory:

```sh
docker compose -f docker.compose.yaml exec sublane sublane restore --input /data/backups/team.sublane-backup.tar.gz --data-dir /data/restore-ready
```

The web restore action produces the same target. To activate it, create a local Compose override, for example `docker.compose.restore.yaml`:

```yaml
services:
  sublane:
    environment:
      SUBLANE_DATA_DIR: /data/restore-ready
```

Then stop the original process and start with the override:

```sh
docker compose -f docker.compose.yaml stop sublane
docker compose -f docker.compose.yaml -f docker.compose.restore.yaml up -d
```

For an already stopped container, use `docker compose -f docker.compose.yaml run --rm --no-deps sublane ...` instead of `exec` to run maintenance commands against its named data volume. Keep the selected override in subsequent deployment commands. Do not remove the named volume during this procedure.

## Validation and failure behavior

Verification checks exact archive entries and sizes, SHA-256 digests, gzip integrity, a recognized contiguous migration history, SQLite integrity and foreign keys, bounded credential records, and decryption of account credentials and encrypted API keys. Validation connections also cap individual SQLite values and SQL statements at 1 MiB. Symlinks, path traversal, duplicate/unexpected files, missing files, truncated archives and unknown future schemas are rejected. Known older schemas are migrated only inside the private extracted copy.

Restore closes and synchronizes its prepared database/key before publishing the entire new directory with an atomic no-replace operation. Linux and macOS require filesystem support for that operation; an unsupported operation fails without falling back to an overwrite. No extra database table or schema migration is introduced by backup support. SQLite snapshot creation uses the [SQLite Online Backup API](https://www.sqlite.org/backup.html) through the pinned Go driver, rather than copying a live database file.

| Endpoint | Purpose |
| --- | --- |
| `GET /api/settings/backup` | Read the web size limit and fixed restore destination |
| `POST /api/settings/backup/export` | Empty JSON object; return an authenticated gzip attachment |
| `POST /api/settings/backup/verify` | Raw gzip upload; return verified metadata |
| `POST /api/settings/backup/restore` | Raw gzip upload; prepare a new directory and return its location |

Use `application/gzip` or `application/octet-stream` for uploads. Invalid archives return 400, an existing destination returns 409, oversized uploads return 413, and an occupied operation slot returns 429 with `Retry-After`. Storage and other operational failures return sanitized 503 errors.
