import { spawnSync } from "node:child_process";
import { writeFileSync, renameSync, mkdirSync, rmSync } from "node:fs";
import { dirname, resolve, basename, join } from "node:path";
import { randomUUID } from "node:crypto";
import { parseArgs } from "node:util";
import { releaseMetadata } from "./release.mjs";

try {
  const { values } = parseArgs({
    options: {
      config: { type: "string", default: "cliff.toml" },
      output: { type: "string" },
      tag: { type: "string" },
      release: { type: "boolean", default: false },
    },
  });
  const tag =
    values.tag ??
    (values.release ? process.env.SUBLANE_CHANGELOG_TAG : undefined);
  if (values.release && !tag)
    throw new Error(
      "Use make release-notes TAG=v1.2.3 with an existing tag at HEAD.",
    );
  const args = [
    "--config",
    resolve(values.config),
    "--repository",
    process.cwd(),
    "--offline",
    "--no-exec",
    "--use-branch-tags",
  ];
  if (tag) {
    const { prerelease } = releaseMetadata(tag, "owner/repo");
    const head = spawnSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" });
    const target = spawnSync(
      "git",
      ["rev-parse", "--verify", `refs/tags/${tag}^{commit}`],
      { encoding: "utf8" },
    );
    if (
      head.status !== 0 ||
      target.status !== 0 ||
      head.stdout.trim() !== target.stdout.trim()
    )
      throw new Error(
        "Release notes require the selected tag to exist at HEAD.",
      );
    args.push("--current", "--tag", tag, "--strip", "header");
    // A stable release summarizes the entire cycle, including commits first shipped in RCs.
    if (!prerelease)
      args.push(
        "--tag-pattern",
        "^v(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)$",
      );
  }
  const cliff = process.env.GIT_CLIFF || "git-cliff";
  const result = spawnSync(cliff, args, {
    encoding: "utf8",
    maxBuffer: 8 << 20,
    timeout: 60000,
  });
  if (result.error || result.status !== 0)
    throw new Error(
      result.error?.code === "ENOENT"
        ? "Install git-cliff 2.14.2 or set GIT_CLIFF to its executable."
        : `git-cliff failed: ${result.stderr?.trim() || result.error?.message || "generation error"}`,
    );
  if (!result.stdout.trim())
    throw new Error(
      "git-cliff produced no changelog; the previous file was preserved.",
    );
  const output = resolve(
    values.output ?? (tag ? "dist/release-notes.md" : "CHANGELOG.md"),
  );
  mkdirSync(dirname(output), { recursive: true });
  const temporary = join(
    dirname(output),
    `.${basename(output)}.${randomUUID()}.tmp`,
  );
  // Render completely before replacing the file, so tool/configuration failures cannot erase it.
  try {
    writeFileSync(temporary, result.stdout, { flag: "wx" });
    renameSync(temporary, output);
  } finally {
    rmSync(temporary, { force: true });
  }
  console.log(`Generated ${output}`);
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
