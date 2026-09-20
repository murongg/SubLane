import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  mkdtempSync,
  mkdirSync,
  writeFileSync,
  rmSync,
  copyFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import { workflowURL } from "./publish.mjs";

const script = fileURLToPath(new URL("./publish.mjs", import.meta.url));
const gitEnv = {
  ...process.env,
  GIT_CONFIG_GLOBAL: "/dev/null",
  GIT_CONFIG_NOSYSTEM: "1",
  GIT_AUTHOR_NAME: "Synthetic Maintainer",
  GIT_AUTHOR_EMAIL: "maintainer@example.test",
  GIT_COMMITTER_NAME: "Synthetic Maintainer",
  GIT_COMMITTER_EMAIL: "maintainer@example.test",
};
function run(cwd, args) {
  const result = spawnSync("git", args, { cwd, env: gitEnv, encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
  return result.stdout.trim();
}
function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "sublane-publish-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const remote = join(root, "remote.git");
  const cwd = join(root, "work");
  run(root, ["init", "--bare", "--initial-branch=main", remote]);
  run(root, ["init", "--initial-branch=main", cwd]);
  writeFileSync(join(cwd, "README"), "Synthetic release fixture\n");
  mkdirSync(join(cwd, ".github/workflows"), { recursive: true });
  writeFileSync(
    join(cwd, ".github/workflows/release.yaml"),
    "name: Synthetic release\n",
  );
  run(cwd, ["add", "."]);
  run(cwd, ["commit", "-m", "Synthetic initial commit"]);
  run(cwd, ["remote", "add", "origin", remote]);
  run(cwd, ["push", "-u", "origin", "main"]);
  return { cwd, remote, root };
}
function publish(f, ...args) {
  return spawnSync(process.execPath, [script, ...args], {
    cwd: f.cwd,
    env: { ...gitEnv, SUBLANE_PUBLISH_TAG: "" },
    encoding: "utf8",
  });
}
function noTags(f) {
  assert.equal(run(f.cwd, ["tag", "--list"]), "");
  assert.equal(run(f.remote, ["tag", "--list"]), "");
}

test("publishes exactly one annotated tag and identifies prereleases automatically", (t) => {
  for (const tag of ["v1.2.3", "v1.3.0-rc.1"]) {
    const f = fixture(t);
    run(f.cwd, ["tag", "-a", "v0.1.0", "-m", "Synthetic unrelated tag"]);
    run(f.cwd, ["config", "push.followTags", "true"]);
    const commit = run(f.cwd, ["rev-parse", "HEAD"]);
    const result = publish(f, tag);
    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, tag.includes("-") ? /prerelease/i : /stable/i);
    assert.equal(run(f.remote, ["tag", "--list"]), tag);
    assert.equal(run(f.cwd, ["cat-file", "-t", `refs/tags/${tag}`]), "tag");
    assert.equal(run(f.remote, ["rev-parse", `${tag}^{commit}`]), commit);
    assert.equal(run(f.remote, ["rev-parse", "main"]), commit);
    assert.equal(run(f.cwd, ["status", "--porcelain"]), "");
  }
});

test("dry run validates without creating a tag or pushing", (t) => {
  const f = fixture(t);
  const result = publish(f, "v1.2.3-beta.1", "--dry-run");
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /dry run/i);
  assert.match(result.stdout, /prerelease/i);
  noTags(f);
});

test("requires an explicit valid tag and rejects unknown arguments", (t) => {
  const f = fixture(t);
  for (const args of [[], ["v1.2"], ["v1.2.3;echo"], ["v1.2.3", "--force"]]) {
    const result = publish(f, ...args);
    assert.notEqual(result.status, 0);
    noTags(f);
  }
});

test("blocks tracked, staged and untracked work before creating a tag", (t) => {
  for (const kind of ["tracked", "staged", "untracked"]) {
    const f = fixture(t);
    writeFileSync(
      join(f.cwd, kind === "untracked" ? "new-file" : "README"),
      "Synthetic change\n",
    );
    if (kind === "staged") run(f.cwd, ["add", "README"]);
    const result = publish(f, "v1.2.3");
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /clean|uncommitted/i);
    noTags(f);
  }
});

test("rejects feature branches and detached HEAD", (t) => {
  for (const args of [
    ["switch", "-c", "feature/synthetic"],
    ["checkout", "--detach"],
  ]) {
    const f = fixture(t);
    run(f.cwd, args);
    const result = publish(f, "v1.2.3");
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /main/i);
    noTags(f);
  }
});

test("requires main to match the actual push destination", (t) => {
  for (const direction of ["ahead", "behind", "push-target"]) {
    const f = fixture(t);
    if (direction === "ahead") {
      run(f.cwd, [
        "commit",
        "--allow-empty",
        "-m",
        "Synthetic unpushed commit",
      ]);
    } else if (direction === "behind") {
      const other = join(f.root, "other");
      run(f.root, ["clone", f.remote, other]);
      run(other, ["commit", "--allow-empty", "-m", "Synthetic remote commit"]);
      run(other, ["push", "origin", "main"]);
    } else {
      const other = join(f.root, "push.git");
      run(f.root, ["init", "--bare", "--initial-branch=main", other]);
      run(f.cwd, ["remote", "set-url", "--push", "origin", other]);
    }
    const result = publish(f, "v1.2.3");
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /main/i);
    noTags(f);
  }
});

test("rejects duplicate local or remote tags without moving them", (t) => {
  for (const side of ["cwd", "remote"]) {
    const f = fixture(t);
    run(f[side], ["tag", "-a", "v1.2.3", "-m", "Synthetic existing release"]);
    const before = run(f[side], ["rev-parse", "refs/tags/v1.2.3"]);
    const result = publish(f, "v1.2.3");
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /already exists/i);
    assert.equal(run(f[side], ["rev-parse", "refs/tags/v1.2.3"]), before);
  }
});

test("rejects multiple push destinations and missing release workflows", (t) => {
  for (const kind of ["multiple", "workflow"]) {
    const f = fixture(t);
    if (kind === "multiple") {
      run(f.cwd, ["config", "--add", "remote.origin.pushurl", f.remote]);
      run(f.cwd, [
        "config",
        "--add",
        "remote.origin.pushurl",
        join(f.root, "other.git"),
      ]);
    } else {
      run(f.cwd, ["rm", ".github/workflows/release.yaml"]);
      run(f.cwd, ["commit", "-m", "Synthetic workflow removal"]);
      run(f.cwd, ["push", "origin", "main"]);
    }
    const result = publish(f, "v1.2.3");
    assert.notEqual(result.status, 0);
    assert.match(
      result.stderr,
      kind === "multiple" ? /one.*push|multiple/i : /workflow/i,
    );
    noTags(f);
  }
});

test("retains the local tag and reports a safe retry when the push fails", (t) => {
  const f = fixture(t);
  writeFileSync(join(f.remote, "hooks/pre-receive"), "#!/bin/sh\nexit 1\n", {
    mode: 0o755,
  });
  const result = publish(f, "v1.2.3");
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /retained|kept/i);
  assert.match(
    result.stderr,
    /git push .*refs\/tags\/v1\.2\.3:refs\/tags\/v1\.2\.3/,
  );
  assert.equal(run(f.cwd, ["tag", "--list"]), "v1.2.3");
  assert.equal(run(f.remote, ["tag", "--list"]), "");
});

test("make passes the tag as data rather than executing shell fragments", (t) => {
  const f = fixture(t);
  mkdirSync(join(f.cwd, "scripts"));
  for (const name of ["publish.mjs", "release.mjs"])
    copyFileSync(
      fileURLToPath(new URL(name, import.meta.url)),
      join(f.cwd, "scripts", name),
    );
  copyFileSync(
    fileURLToPath(new URL("../Makefile", import.meta.url)),
    join(f.cwd, "Makefile"),
  );
  run(f.cwd, ["add", "."]);
  run(f.cwd, ["commit", "-m", "Synthetic script integration"]);
  run(f.cwd, ["push", "origin", "main"]);
  for (const tag of [
    "v1.2.3;touch unexpected-file",
    "v1.2.3$(shell touch unexpected-file)",
  ]) {
    const result = spawnSync("make", ["publish", `TAG=${tag}`], {
      cwd: f.cwd,
      env: gitEnv,
      encoding: "utf8",
    });
    assert.notEqual(result.status, 0);
    assert.equal(run(f.cwd, ["status", "--porcelain"]), "");
    noTags(f);
  }
  const preview = spawnSync("make", ["publish-check", "TAG=v1.2.3"], {
    cwd: f.cwd,
    env: gitEnv,
    encoding: "utf8",
  });
  assert.equal(preview.status, 0, preview.stderr);
  noTags(f);
});

test("formats Actions links for GitHub remotes without exposing URL credentials", () => {
  for (const remote of [
    "git@github.com:synthetic-team/sublane.git",
    "ssh://git@github.com/synthetic-team/sublane.git",
    "https://synthetic-token@github.com/synthetic-team/sublane.git",
  ]) {
    assert.equal(
      workflowURL(remote),
      "https://github.com/synthetic-team/sublane/actions/workflows/release.yaml",
    );
  }
  assert.equal(workflowURL("/tmp/synthetic.git"), undefined);
  assert.equal(
    workflowURL("https://git.example.test/team/repository"),
    undefined,
  );
});
