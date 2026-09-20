import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, writeFileSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

const enabled = process.env.SUBLANE_TEST_CHANGELOG === "1";
const script = fileURLToPath(new URL("./changelog.mjs", import.meta.url));
const config = fileURLToPath(new URL("../cliff.toml", import.meta.url));
const env = {
  ...process.env,
  GIT_CONFIG_GLOBAL: "/dev/null",
  GIT_CONFIG_NOSYSTEM: "1",
  GIT_AUTHOR_NAME: "Synthetic Maintainer",
  GIT_AUTHOR_EMAIL: "maintainer@example.test",
  GIT_COMMITTER_NAME: "Synthetic Maintainer",
  GIT_COMMITTER_EMAIL: "maintainer@example.test",
  GIT_AUTHOR_DATE: "2026-01-01T12:00:00Z",
  GIT_COMMITTER_DATE: "2026-01-01T12:00:00Z",
};
function git(cwd, ...args) {
  const result = spawnSync("git", args, { cwd, env, encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
  return result.stdout.trim();
}
function fixture(t) {
  const cwd = mkdtempSync(join(tmpdir(), "sublane-changelog-"));
  t.after(() => rmSync(cwd, { recursive: true, force: true }));
  git(cwd, "init", "--initial-branch=main");
  return cwd;
}
function commit(cwd, message) {
  git(cwd, "commit", "--allow-empty", "-m", message);
}
function generate(cwd, tag, extra = {}) {
  const output = join(cwd, "notes.md");
  const args = [script, "--config", config, "--output", output];
  if (tag) args.push("--tag", tag);
  const result = spawnSync(process.execPath, args, {
    cwd,
    env: { ...env, ...extra },
    encoding: "utf8",
  });
  return { ...result, output };
}

test(
  "Keep a Changelog groups user-facing changes and preserves breaking notes",
  { skip: !enabled },
  (t) => {
    const cwd = fixture(t);
    for (const message of [
      "feat(api): add synthetic model lookup",
      "perf(gateway): reduce synthetic queue overhead",
      "feat(api): deprecate synthetic legacy endpoint",
      "refactor(api): remove synthetic legacy endpoint",
      "fix(auth): reject synthetic expired sessions",
      "fix(security): protect synthetic token scope",
      "docs: document synthetic setup",
      "docs(api): revise synthetic contract\n\nBREAKING CHANGE: Send the synthetic option explicitly.",
      "test: add synthetic fixtures",
      "chore: update synthetic tooling",
      "refactor(api)!: replace synthetic payload\n\nBREAKING CHANGE: Send the synthetic payload as an object.",
    ])
      commit(cwd, message);
    const result = generate(cwd);
    assert.equal(result.status, 0, result.stderr);
    const notes = readFileSync(result.output, "utf8");
    assert.match(notes, /## \[Unreleased\]/);
    for (const name of [
      "Added",
      "Changed",
      "Deprecated",
      "Removed",
      "Fixed",
      "Security",
    ])
      assert.match(notes, new RegExp(`### ${name}`));
    assert.deepEqual(
      [...notes.matchAll(/^### (.+)$/gm)].map((match) => match[1]),
      ["Added", "Changed", "Deprecated", "Removed", "Fixed", "Security"],
    );
    assert.match(notes, /Add synthetic model lookup/);
    assert.match(notes, /\*\*BREAKING:\*\*/);
    assert.match(notes, /Send the synthetic payload as an object/);
    assert.match(notes, /Send the synthetic option explicitly/);
    assert.doesNotMatch(
      notes,
      /Document synthetic setup|Add synthetic fixtures|Update synthetic tooling/,
    );
    assert.equal(git(cwd, "tag", "--list"), "");
  },
);

test(
  "release notes isolate the current tag and stable notes include prerelease work",
  { skip: !enabled },
  (t) => {
    const cwd = fixture(t);
    commit(cwd, "feat: add synthetic baseline");
    git(cwd, "tag", "v0.1.0");
    commit(cwd, "feat: add synthetic prerelease capability");
    git(cwd, "tag", "v0.2.0-rc.1");
    let result = generate(cwd, "v0.2.0-rc.1");
    assert.equal(result.status, 0, result.stderr);
    let notes = readFileSync(result.output, "utf8");
    assert.match(notes, /## \[0.2.0-rc.1\]/);
    assert.match(notes, /Add synthetic prerelease capability/);
    assert.doesNotMatch(notes, /Add synthetic baseline|Unreleased/);
    commit(cwd, "fix: stabilize synthetic capability");
    git(cwd, "tag", "v0.2.0");
    result = generate(cwd, "v0.2.0");
    assert.equal(result.status, 0, result.stderr);
    notes = readFileSync(result.output, "utf8");
    assert.match(notes, /## \[0.2.0\]/);
    assert.match(notes, /Add synthetic prerelease capability/);
    assert.match(notes, /Stabilize synthetic capability/);
    assert.doesNotMatch(notes, /Add synthetic baseline|## \[0.2.0-rc.1\]/);
    assert.match(notes, /compare\/v0.1.0\.\.\.v0.2.0/);
  },
);

test(
  "failed generation and mismatched tags preserve the previous file",
  { skip: !enabled },
  (t) => {
    const cwd = fixture(t);
    commit(cwd, "feat: add synthetic baseline");
    git(cwd, "tag", "v0.1.0");
    commit(cwd, "fix: change synthetic baseline");
    const output = join(cwd, "notes.md");
    writeFileSync(output, "Synthetic previous changelog\n");
    for (const [tag, extra] of [
      ["v0.1.0", {}],
      ["v9.9.9", {}],
      [undefined, { GIT_CLIFF: join(cwd, "missing-binary") }],
    ]) {
      const result = generate(cwd, tag, extra);
      assert.notEqual(result.status, 0);
      assert.equal(
        readFileSync(output, "utf8"),
        "Synthetic previous changelog\n",
      );
    }
  },
);

test(
  "stable promotion at the RC commit and maintenance tags keep their own history",
  { skip: !enabled },
  (t) => {
    const cwd = fixture(t);
    commit(cwd, "feat: add synthetic original capability");
    git(cwd, "tag", "v1.0.0");
    commit(cwd, "feat: add synthetic upcoming capability");
    git(cwd, "tag", "v1.1.0-rc.1");
    git(cwd, "tag", "v1.1.0");
    let result = generate(cwd, "v1.1.0");
    assert.equal(result.status, 0, result.stderr);
    let notes = readFileSync(result.output, "utf8");
    assert.match(notes, /Add synthetic upcoming capability/);
    assert.doesNotMatch(notes, /## \[1.1.0-rc.1\]/);
    git(cwd, "switch", "-c", "maintenance", "v1.0.0");
    commit(cwd, "fix: repair synthetic original capability");
    git(cwd, "tag", "v1.0.1");
    result = generate(cwd, "v1.0.1");
    assert.equal(result.status, 0, result.stderr);
    notes = readFileSync(result.output, "utf8");
    assert.match(notes, /Repair synthetic original capability/);
    assert.doesNotMatch(notes, /Add synthetic upcoming capability/);
    assert.match(notes, /compare\/v1.0.0\.\.\.v1.0.1/);
  },
);
