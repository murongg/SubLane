#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { parseArgs } from "node:util";
import { pathToFileURL } from "node:url";
import { releaseMetadata } from "./release.mjs";

export function workflowURL(remote) {
  try {
    const url = new URL(
      remote.replace(/^git@github\.com:/, "https://github.com/"),
    );
    if (
      url.hostname !== "github.com" ||
      !["https:", "ssh:"].includes(url.protocol)
    )
      return undefined;
    const repository = url.pathname
      .replace(/^\//, "")
      .replace(/\/$/, "")
      .replace(/\.git$/, "");
    releaseMetadata("v0.0.0", repository);
    return `https://github.com/${repository}/actions/workflows/release.yaml`;
  } catch {
    return undefined;
  }
}

export function publishRelease(
  tag,
  { cwd = process.cwd(), dryRun = false } = {},
) {
  const { prerelease } = releaseMetadata(tag, "owner/repo");
  function git(args, message, missingStatus) {
    const result = spawnSync("git", args, {
      cwd,
      encoding: "utf8",
      timeout: 60_000,
      stdio: ["ignore", "pipe", "pipe"],
    });
    if (missingStatus !== undefined && result.status === missingStatus)
      return undefined;
    // Git transport errors can echo credential-bearing URLs. Report the operation, not raw stderr.
    if (result.error || result.status !== 0) throw new Error(message);
    return result.stdout.trim();
  }
  function requireCleanMain() {
    const branch = git(
      ["symbolic-ref", "--quiet", "--short", "HEAD"],
      "Publish from main; detached HEAD is not supported.",
    );
    if (branch !== "main") throw new Error("Publish from the main branch.");
    if (
      git(
        ["status", "--porcelain", "--untracked-files=normal"],
        "Unable to inspect the working tree.",
      )
    ) {
      throw new Error(
        "A clean working tree is required; commit or set aside uncommitted changes first.",
      );
    }
  }
  requireCleanMain();
  const commit = git(
    ["rev-parse", "HEAD"],
    "Unable to resolve the release commit.",
  );
  git(
    ["cat-file", "-e", `${commit}:.github/workflows/release.yaml`],
    "Merge the release workflow into main before publishing.",
  );
  const urls = git(
    ["remote", "get-url", "--push", "--all", "origin"],
    "Configure an origin push destination before publishing.",
  ).split("\n");
  if (urls.length !== 1 || !urls[0])
    throw new Error("Exactly one origin push destination is required.");
  const destination = urls[0];
  const main = git(
    [
      "ls-remote",
      "--exit-code",
      "--refs",
      "--",
      destination,
      "refs/heads/main",
    ],
    "Unable to read main from the origin push destination.",
  );
  if (main.split(/\s+/)[0] !== commit)
    throw new Error(
      "Local main must exactly match main at the origin push destination. Synchronize main first.",
    );
  const ref = `refs/tags/${tag}`;
  if (
    git(
      ["show-ref", "--verify", "--quiet", ref],
      "Unable to inspect local tags.",
      1,
    ) !== undefined
  ) {
    throw new Error(
      `Local tag ${tag} already exists; it will not be replaced.`,
    );
  }
  if (
    git(
      ["ls-remote", "--exit-code", "--tags", "--refs", "--", destination, ref],
      "Unable to inspect remote tags.",
      2,
    ) !== undefined
  ) {
    throw new Error(
      `Remote tag ${tag} already exists; it will not be replaced.`,
    );
  }
  const configuredURL =
    git(
      ["config", "--get", "remote.origin.pushurl"],
      "Unable to read origin push configuration.",
      1,
    ) ??
    git(
      ["config", "--get", "remote.origin.url"],
      "Unable to read origin configuration.",
    );
  const actions = workflowURL(configuredURL);
  const kind = prerelease ? "prerelease" : "stable release";
  console.log(`${tag}: ${kind}, commit ${commit}, remote origin`);
  if (dryRun) {
    console.log("Dry run passed. No tag was created or pushed.");
  } else {
    requireCleanMain();
    if (git(["rev-parse", "HEAD"], "Unable to recheck HEAD.") !== commit)
      throw new Error("HEAD changed during preflight; run the checks again.");
    // Pin the inspected commit explicitly; a concurrent checkout must not change the release target.
    git(
      ["tag", "--annotate", "--message", `Release ${tag}`, tag, commit],
      "Unable to create the annotated tag; check your Git identity and signing configuration.",
    );
    const retry = `git push --no-follow-tags --no-mirror origin ${ref}:${ref}`;
    git(
      [
        "push",
        "--no-follow-tags",
        "--no-mirror",
        "--",
        destination,
        `${ref}:${ref}`,
      ],
      `Push failed or could not be confirmed. Local tag ${tag} was retained. Inspect the remote tag first; if absent, retry with: ${retry}`,
    );
    const published = git(
      ["ls-remote", "--exit-code", "--tags", "--refs", "--", destination, ref],
      `Unable to verify the remote tag; it may already be published. Local tag ${tag} was retained.`,
    );
    const local = git(["rev-parse", ref], "Unable to verify the local tag.");
    if (published.split(/\s+/)[0] !== local)
      throw new Error(
        "Remote tag verification failed. Inspect the remote before taking further action.",
      );
    console.log(`Pushed and verified ${tag}.`);
  }
  if (actions) console.log(`Actions: ${actions}`);
  return { tag, prerelease, commit, actions, dryRun };
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  try {
    const { values, positionals } = parseArgs({
      options: {
        "dry-run": { type: "boolean", default: false },
        help: { type: "boolean", short: "h" },
      },
      allowPositionals: true,
    });
    const usage =
      "Usage: node scripts/publish.mjs TAG [--dry-run]\nOr: make publish TAG=v1.2.3 / make publish-check TAG=v1.2.3";
    if (values.help) {
      console.log(usage);
    } else {
      const tag = positionals[0] ?? process.env.SUBLANE_PUBLISH_TAG;
      if (positionals.length > 1 || !tag) throw new Error(usage);
      publishRelease(tag, { dryRun: values["dry-run"] });
    }
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
