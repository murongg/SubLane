# Releases

The `Release` workflow starts when a maintainer pushes a `v*` tag. There is no automatic release on ordinary branch pushes or pull requests.

## Version contract

Use canonical tags such as `v0.1.0` or `v0.1.0-rc.1`. The application version and Docker version tag omit the leading `v`. Leading-zero numeric components, whitespace, shell characters and SemVer build metadata (`+...`) are rejected before publishing. Prerelease numeric identifiers must also be canonical.

Every release includes:

- `sublane_VERSION_linux_amd64.tar.gz` and `sublane_VERSION_linux_arm64.tar.gz`.
- `docker.compose.yaml` and `SHA256SUMS` for the public release files.
- A multi-platform `ghcr.io/murongg/sublane:VERSION` image, plus the two architecture tags.
- Automatically generated GitHub release notes.

Only the highest stable version tag currently present in the repository can update the Docker `latest` channel and GitHub's latest-release designation. A prerelease or an older maintenance version never moves those channels backwards. If the highest stable tag's build fails, rerun that release; lower tags intentionally do not take its place. Deployments should pin a version or digest rather than rely on `latest`.

## First-time repository setup

The workflow uses `GITHUB_TOKEN`, scoped to `contents: write` and `packages: write` only in the publication job. It does not need a Docker Hub account or a personal access token. GitHub Actions and GHCR package publication must be allowed by the repository/organization policy. All action dependencies are pinned to full commit SHAs.

The repository source label links the image to the repository. GHCR packages can initially be private even for a public source repository. After the first successful publication, check the package's access settings and make it public if anonymous pulls are intended. See GitHub's [Container registry documentation](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry). Keep published packages associated with this repository so subsequent workflows retain write access.

## Publish a version

Merge the release workflow and the desired changes first. Review `CHANGELOG.md`, update release-facing documentation, and run `make check`. Choose a new unused version; do not move an existing release tag.

For example, after choosing `v0.1.0`:

```sh
git switch main
git pull --ff-only
make publish-check TAG=v0.1.0
make publish TAG=v0.1.0
```

`make publish-check` runs the same preflight without creating or pushing a tag. `make publish` creates an annotated tag at the inspected commit and pushes only that tag. Both require a clean working tree (including untracked files), the `main` branch, an existing release workflow in that commit, and exactly one `origin` push destination whose `main` matches local HEAD. Local or remote duplicate tags are rejected. No branch is switched, merged or pushed automatically.

The tag determines release type; no separate prerelease flag is needed:

```sh
make publish TAG=v0.1.0-rc.1
```

You can also run `node scripts/publish.mjs v0.1.0 --dry-run` or `node scripts/publish.mjs v0.1.0` directly from the repository. Node.js 24 and Git are required, with working Git push credentials (for example an SSH agent); Docker Hub credentials are not required. The script verifies the remote tag after pushing and prints the GitHub Actions workflow link when the configured remote uses a recognized GitHub URL.

If a push fails or its result cannot be verified, the local tag is retained because the remote may already have accepted it. Inspect the remote and Actions first. If the remote tag is absent, use the single-tag retry command printed by the script. It never deletes tags or force-pushes. The script triggers the existing CI pipeline; it does not report builds or publication as complete merely because the tag push succeeded.

The example version is illustrative. Publish one chosen version at a time and inspect the Actions result before announcing availability. Pushing a version tag is the publication trigger; the workflow creates the GitHub Release itself.

## Workflow sequence

1. Validate the tag and compute the lowercase GHCR image path from the repository name.
2. Call the normal CI workflow: sqlc drift check, Go race tests/vet, frontend checks/build, release metadata tests and Compose validation.
3. Build and smoke-test the Docker image on native Linux amd64 and arm64 runners. On PRs and releases, extract the actual binary from each tested image, package it with deployment files and license notices, then unpack and execute it to verify the version. Release runs also save both tested images as short-lived Actions artifacts.
4. After all checks pass, create or resume a draft GitHub Release. A previously published release is never overwritten.
5. Load and push those tested images, then create the multi-platform version tag. No second application build happens during publication.
6. Upload binary archives, Compose and SHA-256 checksums. Promote the newest stable channel when eligible, then publish the completed release.

Version/source/revision labels are embedded in the image and `sublane --version` reports the packaged version. Image root filesystems contain only Alpine runtime utilities, public CA certificates, the binary and license notices. No source checkout, compiler, Node.js dependencies or local data is included. Build context allowlisting excludes local databases, encryption keys, backups and worktrees.

## Failures and retries

Use **Re-run failed jobs** on the same tag's workflow run when a network, build or registry operation fails. Before completion, release artifacts remain in a draft and can be replaced by a retry. Per-architecture/version images may already exist if a later step failed; the retry completes the same release. Publishing the registry and GitHub Release cannot be one atomic transaction, so check both destinations after a failure.

Artifacts are retained for seven days. After expiry, rerun all jobs to reconstruct and re-test them. Reruns replace same-named Actions artifacts within that run; already published GitHub releases remain protected. Once a release is published, use a new version for code or packaging corrections rather than overwriting it. Deleting or moving a tag does not roll back downloaded binaries, published images or deployed databases.

`GITHUB_TOKEN`-created tag pushes normally do not trigger another workflow. If another automation creates tags, use an appropriately scoped GitHub App/token or trigger the release through an explicitly designed dispatch flow. Manual maintainer pushes work without that extra setup. See GitHub's [workflow triggering rules](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/trigger-a-workflow).

## Local packaging

```sh
make release VERSION=0.0.0-test
```

This builds the frontend once, cross-compiles the two Linux binaries and writes archives plus `SHA256SUMS` under ignored `dist/release/`. Existing archives are not overwritten; use a fresh test version or move previous generated artifacts before rebuilding. This command does not push images or create releases. Keep local build validation separate from native container runtime evidence.
