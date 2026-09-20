import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  statSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

const script = fileURLToPath(new URL("./install.sh", import.meta.url));
const compose = "services:\n  sublane:\n    image: ${SUBLANE_IMAGE}\n";
const checksum = createHash("sha256").update(compose).digest("hex");

function fixture(t, mode = "", api = {}) {
  const root = mkdtempSync(join(tmpdir(), "sublane-installer-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const bin = join(root, "bin");
  mkdirSync(bin);
  const mock = `#!${process.execPath}
const fs = require('node:fs');
const path = require('node:path');
const command = path.basename(process.argv[1]);
const args = process.argv.slice(2);
const mode = process.env.INSTALL_TEST_MODE;
fs.appendFileSync(process.env.INSTALL_TEST_LOG, JSON.stringify({command, args, image: process.env.SUBLANE_IMAGE, port: process.env.SUBLANE_PORT}) + '\\n');
if (command === 'curl') {
  if (mode === 'download-failure') process.exit(22);
  const url = args.find(value => value.startsWith('https://'));
  const output = args[args.indexOf('--output') + 1];
  if (url.startsWith('https://api.github.com/')) {
    const api = JSON.parse(process.env.INSTALL_TEST_API);
    const latest = url.endsWith('/releases/latest');
    const status = latest ? (api.latestStatus ?? 404) : (api.listStatus ?? 200);
    const page = new URL(url).searchParams.get('page') ?? '1';
    const body = latest ? (api.latest ?? {message: 'Not Found'}) : (api.pages?.[page] ?? [
      {tag_name:'v0.1.0-rc.1',draft:false,prerelease:true,published_at:'2030-01-01T00:00:00Z'}
    ]);
    fs.writeFileSync(output, JSON.stringify(body));
    process.stdout.write(String(status));
    process.exit(0);
  }
  const body = url.endsWith('/SHA256SUMS')
    ? (mode === 'bad-checksum' ? '0'.repeat(64) : ${JSON.stringify(checksum)}) + '  docker.compose.yaml\\n'
    : ${JSON.stringify(compose)};
  fs.writeFileSync(output, body);
} else if (command === 'docker') {
  if (args[0] === 'info' && mode === 'no-engine') process.exit(1);
  if (args.includes('pull') && mode === 'pull-failure') process.exit(1);
  if (args.includes('up') && mode === 'unhealthy') process.exit(1);
}
`;
  for (const name of ["curl", "docker"])
    writeFileSync(join(bin, name), mock, { mode: 0o755 });
  const log = join(root, "commands.jsonl");
  return {
    root,
    target: join(root, "sublane"),
    env: {
      ...process.env,
      PATH: `${bin}:${process.env.PATH}`,
      INSTALL_TEST_LOG: log,
      INSTALL_TEST_MODE: mode,
      INSTALL_TEST_API: JSON.stringify(api),
    },
    calls: () =>
      existsSync(log)
        ? readFileSync(log, "utf8").trim().split("\n").map(JSON.parse)
        : [],
  };
}

function install(f, args = [], piped = false) {
  return spawnSync("bash", piped ? ["-s", "--", ...args] : [script, ...args], {
    cwd: f.root,
    env: f.env,
    input: piped ? readFileSync(script) : undefined,
    encoding: "utf8",
    timeout: 15000,
  });
}

test("installs a pinned release with checksums and waits for healthy Compose startup", (t) => {
  const f = fixture(t);
  const result = install(f);
  assert.equal(result.status, 0, result.stderr);
  assert.match(
    readFileSync(join(f.target, ".env"), "utf8"),
    /SUBLANE_IMAGE=ghcr.io\/murongg\/sublane:0\.1\.0-rc\.1/,
  );
  assert.equal(
    readFileSync(join(f.target, "docker.compose.yaml"), "utf8"),
    compose,
  );
  assert.equal(statSync(join(f.target, ".env")).mode & 0o777, 0o600);
  const calls = f.calls();
  assert.equal(calls.filter((call) => call.command === "curl").length, 4);
  const up = calls.find((call) => call.args.includes("up"));
  assert.ok(up.args.includes("--wait"));
  assert.ok(up.args.includes("--no-build"));
  assert.ok(up.args.includes("--project-name"));
  assert.ok(
    calls.findIndex((call) => call.args.includes("pull")) < calls.indexOf(up),
  );
  assert.match(result.stdout, /http:\/\/127\.0\.0\.1:8080/);
});

test("supports piped execution, explicit versions, paths with spaces and custom ports", (t) => {
  const f = fixture(t);
  const target = join(f.root, "team gateway");
  const result = install(
    f,
    ["--version", "v1.2.3-beta.2", "--dir", target, "--port", "18851"],
    true,
  );
  assert.equal(result.status, 0, result.stderr);
  const env = readFileSync(join(target, ".env"), "utf8");
  assert.match(env, /SUBLANE_IMAGE=ghcr.io\/murongg\/sublane:1\.2\.3-beta\.2/);
  assert.match(env, /SUBLANE_PORT=18851/);
  assert.match(env, /COMPOSE_PROJECT_NAME=sublane-[a-z0-9-]+/);
  assert.match(result.stdout, /127\.0\.0\.1:18851/);
  assert.equal(
    f
      .calls()
      .some((call) =>
        call.args.some((arg) => arg.startsWith("https://api.github.com/")),
      ),
    false,
  );
});

test("rejects invalid arguments before downloading or starting containers", (t) => {
  for (const args of [
    ["--version", "1.2.3;touch unexpected"],
    ["--version", "1.2"],
    ["--version", "v"],
    ["--version", "01.2.3"],
    ["--version", "1.2.3-rc.01"],
    ["--port", "0"],
    ["--port", "65536"],
    ["--port", "8080\nOTHER=value"],
    ["--dir", "line\nbreak"],
    ["--version"],
    ["--force"],
  ]) {
    const f = fixture(t);
    const result = install(f, args);
    assert.notEqual(result.status, 0, args.join(" "));
    assert.match(result.stderr, /Error:/);
    assert.equal(f.calls().length, 0);
    assert.equal(existsSync(f.target), false);
  }
});

test("preserves existing installations and refuses symlinks", (t) => {
  for (const symlink of [false, true]) {
    const f = fixture(t);
    const original = symlink ? join(f.root, "original") : f.target;
    mkdirSync(original);
    writeFileSync(join(original, ".env"), "SYNTHETIC_SETTING=keep\n");
    if (symlink) symlinkSync(original, f.target);
    const result = install(f);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /exists/i);
    assert.equal(
      readFileSync(join(original, ".env"), "utf8"),
      "SYNTHETIC_SETTING=keep\n",
    );
    assert.equal(f.calls().length, 0);
  }
});

test("download, checksum, engine and pull failures leave no installation", (t) => {
  for (const [mode, message] of [
    ["download-failure", /Could not (download|fetch)/],
    ["bad-checksum", /checksum mismatch/],
    ["no-engine", /Docker is not running/],
    ["pull-failure", /Image pull failed/],
  ]) {
    const f = fixture(t, mode);
    const result = install(f);
    assert.notEqual(result.status, 0, mode);
    assert.match(result.stderr, message);
    assert.equal(existsSync(f.target), false, mode);
    assert.equal(
      f.calls().some((call) => call.args.includes("up")),
      false,
    );
  }
});

test("a startup failure retains configuration and never removes container data", (t) => {
  const f = fixture(t, "unhealthy");
  const result = install(f);
  assert.notEqual(result.status, 0);
  assert.equal(existsSync(join(f.target, ".env")), true);
  assert.match(result.stderr, /retained|preserved/i);
  assert.equal(
    f
      .calls()
      .some((call) => call.args.includes("down") || call.args.includes("rm")),
    false,
  );
  assert.doesNotMatch(result.stdout, /Installation complete/);
});

test("explicit installer settings take precedence over inherited Compose interpolation", (t) => {
  const f = fixture(t);
  Object.assign(f.env, {
    SUBLANE_IMAGE: "example.test/unrelated:image",
    SUBLANE_PORT: "9099",
    COMPOSE_PROJECT_NAME: "existing-synthetic-project",
    COMPOSE_FILE: "/synthetic/unrelated.yaml",
  });
  const result = install(f);
  assert.equal(result.status, 0, result.stderr);
  const calls = f
    .calls()
    .filter((call) => call.args.includes("--project-name"));
  assert.ok(calls.length >= 3);
  for (const call of calls) {
    assert.equal(call.image, "ghcr.io/murongg/sublane:0.1.0-rc.1");
    assert.equal(call.port, "8080");
    assert.notEqual(
      call.args[call.args.indexOf("--project-name") + 1],
      "existing-synthetic-project",
    );
    assert.ok(call.args.includes("--env-file"));
    assert.ok(call.args.includes("-f"));
  }
});

test("help performs no deployment actions", (t) => {
  const f = fixture(t);
  const result = install(f, ["--help"]);
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /--version/);
  assert.equal(f.calls().length, 0);
  assert.equal(existsSync(f.target), false);
});

test("a truncated piped download does not begin installation", (t) => {
  const f = fixture(t);
  const source = readFileSync(script, "utf8");
  const result = spawnSync("bash", ["-s"], {
    cwd: f.root,
    env: f.env,
    input: source.slice(0, source.indexOf('  compose "$install_stage" pull')),
    encoding: "utf8",
  });
  assert.notEqual(result.status, 0);
  assert.equal(f.calls().length, 0);
  assert.equal(existsSync(f.target), false);
});

test("prefers the latest stable release without consulting prereleases", (t) => {
  const f = fixture(t, "", {
    latestStatus: 200,
    latest: { tag_name: "v1.2.3", draft: false, prerelease: false },
  });
  const result = install(f);
  assert.equal(result.status, 0, result.stderr);
  assert.match(
    readFileSync(join(f.target, ".env"), "utf8"),
    /SUBLANE_IMAGE=ghcr.io\/murongg\/sublane:1\.2\.3\n/,
  );
  assert.equal(
    f
      .calls()
      .filter((call) =>
        call.args.some((arg) => arg.startsWith("https://api.github.com/")),
      ).length,
    1,
  );
});

test("without stable releases, selects the most recently published prerelease across pages", (t) => {
  const older = {
    tag_name: "v2.0.0-rc.1",
    draft: false,
    prerelease: true,
    published_at: "2030-01-01T00:00:00Z",
  };
  const f = fixture(t, "", {
    pages: {
      1: Array.from({ length: 100 }, () => older),
      2: [
        {
          ...older,
          tag_name: "v2.0.0-rc.3",
          draft: true,
          published_at: "2030-04-01T00:00:00Z",
        },
        {
          ...older,
          tag_name: "v2.0.0",
          prerelease: false,
          published_at: "2030-03-01T00:00:00Z",
        },
        {
          ...older,
          tag_name: "v2.0.0-rc.2",
          published_at: "2030-02-01T00:00:00Z",
        },
      ],
    },
  });
  const result = install(f);
  assert.equal(result.status, 0, result.stderr);
  assert.match(
    readFileSync(join(f.target, ".env"), "utf8"),
    /SUBLANE_IMAGE=ghcr.io\/murongg\/sublane:2\.0\.0-rc\.2\n/,
  );
});

test("API errors and missing releases stop without silently choosing another channel", (t) => {
  for (const api of [
    { latestStatus: 403 },
    { latestStatus: 500 },
    { latestStatus: 200, latest: {} },
    { listStatus: 503 },
    { pages: { 1: [] } },
  ]) {
    const f = fixture(t, "", api);
    const result = install(f);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /Error:/);
    assert.equal(existsSync(f.target), false);
    assert.equal(
      f
        .calls()
        .some((call) =>
          call.args.some((arg) => arg.includes("/releases/download/")),
        ),
      false,
    );
  }
});
