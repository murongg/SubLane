import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { once } from "node:events";
import { createServer } from "node:net";
import { createHmac } from "node:crypto";
import {
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  realpathSync,
  rmSync,
  statSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

const script = fileURLToPath(new URL("./separate-key.sh", import.meta.url));
const docker = spawnSync("which", ["docker"], {
  encoding: "utf8",
}).stdout.trim();
const available =
  docker &&
  spawnSync(docker, ["compose", "version"], { stdio: "ignore" }).status === 0;
const key = Buffer.alloc(32, 0x5a);

function fixture(t, mode = "") {
  const root = realpathSync(
    mkdtempSync(join(tmpdir(), "sublane-key-migration-")),
  );
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const dir = join(root, "deployment with spaces");
  const data = join(root, "data");
  const bin = join(root, "bin");
  for (const path of [dir, data, bin]) mkdirSync(path, { mode: 0o700 });
  const target = join(root, "private keys");
  const config = join(dir, "docker.compose.yaml");
  writeFileSync(
    join(dir, ".env"),
    "COMPOSE_PROJECT_NAME=synthetic-gateway\nSUBLANE_IMAGE=example.test/sublane:0.12.0\n",
  );
  let original = `# Preserve this operator comment and image interpolation.\nservices:\n  sublane:\n    image: \${SUBLANE_IMAGE}\n    environment:\n      SUBLANE_DATA_DIR: /data\n      SUBLANE_CREDENTIALS_KEY_FILE: \${SUBLANE_CREDENTIALS_KEY_FILE:-}\n      SUBLANE_PUBLIC_URL: https://gateway.example.test\n    volumes:\n      - ${JSON.stringify(data + ":/data")}\n    ports:\n      - '127.0.0.1:18080:8080'\n    read_only: true\n    cap_drop: [ALL]\n    healthcheck:\n      test: [CMD, /bin/true]\nvolumes:\n  unused-volume: {}\n`;
  if (mode === "env-appearance") {
    original =
      "name: synthetic-gateway\n" +
      original.replace("${SUBLANE_IMAGE}", "example.test/sublane:0.12.0");
    rmSync(join(dir, ".env"));
  }
  if (mode === "mount-drift")
    original = original.replace(
      JSON.stringify(data + ":/data"),
      JSON.stringify(root + "/other-data:/data"),
    );
  if (mode === "named-volume")
    original = original
      .replace(JSON.stringify(data + ":/data"), "data:/data")
      .replace("unused-volume: {}", "data: {}");
  if (mode === "legacy-key-mount") {
    const legacy = join(root, "legacy.key");
    writeFileSync(legacy, key, { mode: 0o600 });
    original = original.replace(
      "    ports:",
      `      - ${JSON.stringify(legacy + ":/data/credentials.key:ro")}\n    ports:`,
    );
  }
  writeFileSync(config, original, { mode: 0o640 });
  writeFileSync(join(data, "credentials.key"), key, { mode: 0o600 });
  writeFileSync(
    join(data, "sublane.db"),
    createHmac("sha256", key).update("synthetic-secret").digest("hex"),
  );
  if (mode === "mount-drift") mkdirSync(join(root, "other-data"));
  const helper = join(root, "mock.cjs");
  writeFileSync(
    helper,
    String.raw`
const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const { createHash, createHmac } = require("node:crypto");
const args = process.argv.slice(2);
const mode = process.env.MIGRATION_MODE;
const root = process.env.MIGRATION_ROOT;
const data = path.join(root, "data");
const dir = path.join(root, "deployment with spaces");
const command = process.env.MIGRATION_COMMAND || path.basename(process.argv[1]);
const statePath = path.join(root, "docker-state.json");
const state = fs.existsSync(statePath)
  ? JSON.parse(fs.readFileSync(statePath))
  : { running: true, ups: 0 };
const save = () => fs.writeFileSync(statePath, JSON.stringify(state));
const log = (value) =>
  fs.appendFileSync(
    path.join(root, "events.jsonl"),
    JSON.stringify(value) + "\n",
  );
const hash = (file) =>
  createHash("sha256").update(fs.readFileSync(file)).digest("hex");
function config(argv = args) {
  const c = spawnSync(process.env.REAL_DOCKER, argv, { encoding: "utf8" });
  if (c.status !== 0) {
    process.stderr.write(c.stderr);
    process.exit(c.status || 1);
  }
  return JSON.parse(c.stdout);
}
function parsed() {
  const end = args.findIndex((a) =>
    ["config", "ps", "exec", "up", "stop", "run"].includes(a),
  );
  return config([...args.slice(0, end), "config", "--format", "json"]);
}
function selected(c) {
  const p = c.services.sublane.environment.SUBLANE_CREDENTIALS_KEY_FILE;
  return p
    ? c.services.sublane.volumes.find(
        (v) => v.target === "/run/sublane-keys/credentials.key",
      ).source
    : path.join(data, "credentials.key");
}
if (command === "realpath") {
  const file = args.at(-1);
  if (args.includes("-e")) console.log(fs.realpathSync(file));
  else {
    let parent = path.resolve(file),
      missing = [];
    while (!fs.existsSync(parent)) {
      missing.unshift(path.basename(parent));
      parent = path.dirname(parent);
    }
    console.log(path.join(fs.realpathSync(parent), ...missing));
  }
} else if (command === "stat") {
  const file = args.at(-1),
    s = fs.statSync(file);
  const format = args[args.indexOf("-c") + 1];
  process.stdout.write(
    format
      .replaceAll("%u", "0")
      .replaceAll("%g", "0")
      .replaceAll("%a", (s.mode & 0o777).toString(8))
      .replaceAll("%s", String(s.size))
      .replaceAll("%d", String(s.dev))
      .replaceAll("%i", String(s.ino)) + "\n",
  );
} else if (command === "mkdir") {
  if (
    mode === "state-race" &&
    args.at(-1) === path.join(dir, ".key-migration")
  ) {
    fs.mkdirSync(args.at(-1));
    fs.writeFileSync(
      path.join(args.at(-1), "source.key"),
      "another synthetic recovery key",
    );
  }
  if (
    mode === "destination-race" &&
    args.at(-1) === path.join(root, "private keys")
  ) {
    fs.mkdirSync(args.at(-1));
    fs.writeFileSync(
      path.join(args.at(-1), "credentials.key"),
      "another synthetic key",
    );
  }
  const r = spawnSync(process.env.REAL_MKDIR, args, { stdio: "inherit" });
  process.exit(r.status ?? 1);
} else if (command === "mv") {
  if (
    mode === "config-apply-failure" &&
    args.at(-1) === path.join(dir, "docker.compose.yaml")
  )
    process.exit(1);
  const r = spawnSync(process.env.REAL_MV, args, { stdio: "inherit" });
  process.exit(r.status ?? 1);
} else if (command === "install") {
  const files = [];
  let permission = 0o755,
    directory = false;
  for (let i = 0; i < args.length; i++) {
    if (args[i] === "-d") directory = true;
    else if (args[i] === "-m") permission = parseInt(args[++i], 8);
    else if (args[i] === "-o" || args[i] === "-g") i++;
    else files.push(args[i]);
  }
  if (
    directory &&
    mode === "destination-race" &&
    files[0] === path.join(root, "private keys")
  ) {
    fs.mkdirSync(files[0]);
    fs.writeFileSync(
      path.join(files[0], "credentials.key"),
      "another synthetic key",
    );
  }
  if (directory) fs.mkdirSync(files[0], { recursive: true, mode: permission });
  else {
    fs.copyFileSync(files[0], files[1]);
    if (
      mode === "copy-corruption" &&
      files[1] === path.join(root, "private keys", "credentials.key")
    )
      fs.writeFileSync(files[1], Buffer.alloc(32, 0x41));
  }
  fs.chmodSync(files.at(-1), permission);
} else if (command === "chmod") {
  if (args[0] === "--reference")
    fs.chmodSync(args[2], fs.statSync(args[1]).mode & 0o777);
  else fs.chmodSync(args.at(-1), parseInt(args[0], 8));
} else if (command === "rm") {
  if (args.includes(path.join(data, "credentials.key"))) {
    log({ action: "delete-old" });
    if (mode === "delete-failure") process.exit(1);
  }
  const result = spawnSync(process.env.REAL_RM, args, { stdio: "inherit" });
  process.exit(result.status ?? 1);
} else if (command === "sublane") {
  if (process.env.MIGRATION_REAL_BINARY) {
    const r = spawnSync(process.env.MIGRATION_REAL_BINARY, args, {
      env: process.env,
      stdio: "inherit",
    });
    process.exit(r.status ?? 1);
  }
  if (args[0] === "--help") {
    console.log("--password-stdin --key-file FILE");
    process.exit(0);
  }
  const secret =
    process.env.SUBLANE_CREDENTIALS_KEY_FILE ||
    path.join(data, "credentials.key");
  const password = fs.readFileSync(0, "utf8");
  log({
    action: args[1] === "verify" ? "verify" : "backup",
    external: secret !== path.join(data, "credentials.key"),
    oldExists: fs.existsSync(path.join(data, "credentials.key")),
  });
  if (
    !password.trim() ||
    mode === "original-verification-failure" ||
    (mode === "verification-failure" &&
      secret !== path.join(data, "credentials.key"))
  )
    process.exit(1);
  if (
    createHmac("sha256", fs.readFileSync(secret))
      .update("synthetic-secret")
      .digest("hex") !== fs.readFileSync(path.join(data, "sublane.db"), "utf8")
  )
    process.exit(1);
  if (args[1] === "verify") {
    if (
      !fs
        .readFileSync(args[args.indexOf("--input") + 1], "utf8")
        .startsWith("age-encryption.org/v1\n")
    )
      process.exit(1);
  } else
    fs.writeFileSync(
      args[args.indexOf("--output") + 1],
      "age-encryption.org/v1\nsynthetic archive",
      { mode: 0o600 },
    );
  console.log(JSON.stringify({ encrypted: true }));
} else if (command === "docker") {
  if (args[0] === "info")
    console.log(
      JSON.stringify({
        ID: mode === "changed-daemon" ? "other-synthetic-engine" : "synthetic-engine",
        OSType: "linux",
        SecurityOptions: mode === "rootless" ? ["name=rootless"] : [],
      }),
    );
  else if (args[0] === "context")
    console.log(
      mode === "remote" ? "ssh://example.test" : "unix:///var/run/docker.sock",
    );
  else if (args[0] === "image") console.log("sha256:synthetic-image");
  else if (args[0] === "ps") {
    if(!state.disappeared) console.log(mode === "one-off-present" && !args.includes("label=com.docker.compose.oneoff=False") ? "synthetic-container\nsynthetic-oneoff" : "synthetic-container");
  }
  else if (args[0] === "inspect") {
    const c =
      state.config ||
      config([
        "compose",
        "--project-directory",
        dir,
        "--env-file",
        fs.existsSync(path.join(dir, ".env"))
          ? path.join(dir, ".env")
          : "/dev/null",
        "-f",
        path.join(dir, "docker.compose.yaml"),
        "config",
        "--format",
        "json",
      ]);
    if (mode === "runtime-drift" && !state.config)
      c.services.sublane.environment.SUBLANE_DATA_DIR = "/different-data";
    const mounts = [
      {
        Type: mode === "named-volume" ? "volume" : "bind",
        Name: mode === "named-volume" ? "synthetic-gateway_data" : undefined,
        Source: data,
        Destination: "/data",
        RW: true,
      },
    ];
    if(mode==="legacy-key-mount") mounts.push({Type:"bind",Source:path.join(root,"legacy.key"),Destination:"/data/credentials.key",RW:false});
    for (const v of c.services.sublane.volumes)
      if (v.target === "/run/sublane-keys/credentials.key")
        mounts.push({
          Type: "bind",
          Source: v.source,
          Destination: v.target,
          RW: !v.read_only,
        });
    console.log(
      JSON.stringify([
        {
          Id: "synthetic-container",
          Image: mode === "runtime-image-drift" ? "sha256:other-synthetic-image" : "sha256:synthetic-image",
          State: {
            Running: state.running,
            Health: { Status: state.running ? "healthy" : "unhealthy" },
          },
          Config: {
            Image: c.services.sublane.image,
            Env: Object.entries(c.services.sublane.environment).map(
              ([k, v]) => k + "=" + v,
            ),
            Labels: {
              "com.docker.compose.project": "synthetic-gateway",
              "com.docker.compose.project.working_dir": mode === "owner-drift" ? "/another-synthetic-deployment" : dir,
              "com.docker.compose.project.config_files": path.join(
                dir,
                "docker.compose.yaml",
              ),
            },
          },
          Mounts: mounts,
        },
      ]),
    );
  } else if (args[0] === "compose") {
    if (args.includes("version")) {
      console.log("v2.40.0");
      process.exit(0);
    }
    if (args.includes("config")) {
      log({action:"configuration",locked:fs.existsSync(path.join(dir,".update-lock"))});
      const c = config(args);
      if (
        mode === "semantic-drift" &&
        c.services.sublane.environment.SUBLANE_CREDENTIALS_KEY_FILE
      )
        c.services.sublane.ports[0].published = "19000";
      console.log(JSON.stringify(c));
    } else if (args.includes("ps")) { if(!state.disappeared) console.log(mode === "one-off-present" ? "synthetic-container\nsynthetic-oneoff" : "synthetic-container"); }
    else if (args.includes("stop")) {
      state.running = false;
      save();
      log({ action: "stop" });
    } else if (args.includes("up")) {
      const c = parsed(),
        external =
          !!c.services.sublane.environment.SUBLANE_CREDENTIALS_KEY_FILE;
      state.ups++;
      if(mode==="missing-after-delete"&&external&&!fs.existsSync(path.join(data,"credentials.key"))) {state.disappeared=true;state.running=false;save();process.exit(1);}
      if (
        mode === "kill-after-delete" &&
        external &&
        !fs.existsSync(path.join(data, "credentials.key"))
      ) {
        save();
        process.kill(
          Number(fs.readFileSync(path.join(root, "script.pid"), "utf8")),
          "SIGKILL",
        );
        process.exit(0);
      }
      log({
        action: "up",
        external,
        oldExists: fs.existsSync(path.join(data, "credentials.key")),
      });
      if (
        external &&
        ((mode === "startup-failure" && state.ups === 1) ||
          (mode === "post-delete-failure" &&
            !fs.existsSync(path.join(data, "credentials.key"))) ||
          mode === "rollback-failure")
      ) {
        save();
        process.exit(1);
      }
      if (!external && mode === "rollback-failure") {
        save();
        process.exit(1);
      }
      if (!fs.existsSync(selected(c))) process.exit(1);
      state.disappeared=false;
      state.config = c;
      state.running = true;
      save();
    } else if (args.includes("exec")) {
      if (args.includes("id"))
        console.log(args.at(-1) === "-u" ? process.getuid() : process.getgid());
      else if (args.includes("--help"))
        console.log("--password-stdin --key-file FILE");
      else if (args.includes("sha256sum"))
        console.log(hash(selected(parsed())) + "  key");
    } else if (args.includes("run")) {
      const c = parsed();
      const mount = args[args.indexOf("--volume") + 1];
      const scratch = mount.split(":")[0];
      const sublane = args.indexOf("sublane", args.indexOf("run"));
      const childArgs = args.slice(sublane + 1);
      const env = {
        ...process.env,
        SUBLANE_DATA_DIR: data,
        SUBLANE_CREDENTIALS_KEY_FILE: c.services.sublane.environment
          .SUBLANE_CREDENTIALS_KEY_FILE
          ? selected(c)
          : "",
        SUBLANE_CHECK_DIR: scratch,
        TMPDIR: path.join(scratch, "tmp"),
      };
      const result = spawnSync("/bin/sh", childArgs, { env, stdio: "inherit" });
      if (
        mode === "env-appearance" &&
        c.services.sublane.environment.SUBLANE_CREDENTIALS_KEY_FILE
      )
        fs.writeFileSync(path.join(dir, ".env"), "OPERATOR_CHANGE=preserve\n");
      process.exit(result.status ?? 1);
    }
  }
}

`,
  );
  for (const name of [
    "docker",
    "sublane",
    "stat",
    "install",
    "chmod",
    "rm",
    "realpath",
    "mkdir",
    "mv",
  ])
    writeFileSync(
      join(bin, name),
      `#!${process.execPath}\nprocess.env.MIGRATION_COMMAND=${JSON.stringify(name)};require(${JSON.stringify(helper)});\n`,
      { mode: 0o755 },
    );
  const bashEnv = join(root, "bash-env");
  writeFileSync(
    bashEnv,
    'printf \"%s\\n\" \"$$\" > \"$MIGRATION_ROOT/script.pid\"\n' +
      'id() { if [[ $1 == -u ]]; then printf "0\\n"; else command id "$@"; fi; }\nuname() { printf "Linux\\n"; }\nchown() { :; }\nsync() { :; }\n',
  );
  const env = {
    ...process.env,
    SUBLANE_CREDENTIALS_KEY_FILE: "",
    SUBLANE_IMAGE: "example.test/sublane:0.12.0",
    COMPOSE_PROJECT_NAME: "synthetic-gateway",
    PATH: bin + ":" + process.env.PATH,
    BASH_ENV: bashEnv,
    MIGRATION_ROOT: root,
    MIGRATION_MODE: mode,
    REAL_DOCKER: docker,
    REAL_RM: spawnSync("which", ["rm"], { encoding: "utf8" }).stdout.trim(),
    REAL_MKDIR: spawnSync("which", ["mkdir"], {
      encoding: "utf8",
    }).stdout.trim(),
    REAL_MV: spawnSync("which", ["mv"], { encoding: "utf8" }).stdout.trim(),
  };
  return {
    root,
    dir,
    data,
    target,
    config,
    original,
    env,
    events: () =>
      existsSync(join(root, "events.jsonl"))
        ? readFileSync(join(root, "events.jsonl"), "utf8")
            .trim()
            .split("\n")
            .filter(Boolean)
            .map(JSON.parse)
        : [],
  };
}
function run(f, args = []) {
  return spawnSync("bash", [script, "--dir", f.dir, ...args], {
    env: f.env,
    encoding: "utf8",
    timeout: 45000,
  });
}
function migrate(f) {
  return run(f, ["--key-dir", f.target]);
}
function preserved(f) {
  assert.deepEqual(readFileSync(join(f.data, "credentials.key")), key);
  assert.equal(readFileSync(f.config, "utf8"), f.original);
  assert.equal(existsSync(join(f.dir, ".update-lock")), false);
}

test(
  "separates a Compose key only after encrypted credential checks and supports rollback",
  { skip: !available },
  (t) => {
    const f = fixture(t);
    const result = migrate(f);
    assert.equal(
      result.status,
      0,
      result.stderr +
        result.stdout +
        (existsSync(join(f.dir, ".key-migration", "after.json"))
          ? readFileSync(join(f.dir, ".key-migration", "after.json"), "utf8")
          : ""),
    );
    const resolved = spawnSync(
      docker,
      [
        "compose",
        "--project-directory",
        f.dir,
        "--env-file",
        join(f.dir, ".env"),
        "-f",
        f.config,
        "config",
        "--format",
        "json",
      ],
      { encoding: "utf8", env: f.env },
    );
    assert.equal(resolved.status, 0, resolved.stderr);
    const mount = JSON.parse(resolved.stdout).services.sublane.volumes.find(
      (v) => v.target === "/run/sublane-keys/credentials.key",
    );
    assert.equal(mount?.source, join(f.target, "credentials.key"));
    assert.equal(mount.read_only, true);
    assert.equal(existsSync(join(f.data, "credentials.key")), false);
    assert.deepEqual(readFileSync(join(f.target, "credentials.key")), key);
    assert.equal(statSync(f.target).mode & 0o777, 0o700);
    assert.equal(
      statSync(join(f.target, "credentials.key")).mode & 0o777,
      0o600,
    );
    const events = f.events(),
      deletion = events.findIndex((e) => e.action === "delete-old");
    assert.ok(deletion > 0);
    assert.ok(
      events
        .slice(0, deletion)
        .some((e) => e.action === "verify" && e.external && e.oldExists),
    );
    assert.ok(
      events
        .slice(deletion + 1)
        .some((e) => e.action === "verify" && e.external && !e.oldExists),
    );
    assert.ok(
      readFileSync(f.config, "utf8").includes(
        "# Preserve this operator comment",
      ),
    );
    assert.ok(readFileSync(f.config, "utf8").includes("${SUBLANE_IMAGE}"));
    assert.equal(
      existsSync(join(f.dir, ".key-migration", "source.key")),
      false,
    );
    assert.equal(existsSync(join(f.dir, ".key-migration", "check")), false);
    assert.equal(result.stdout.includes(key.toString()), false);
    const rollback = run(f, ["--rollback"]);
    assert.equal(rollback.status, 0, rollback.stderr + rollback.stdout);
    preserved(f);
  },
);
for (const mode of [
  "startup-failure",
  "verification-failure",
  "post-delete-failure",
  "delete-failure",
  "missing-after-delete",
])
  test("rolls back on " + mode, { skip: !available }, (t) => {
    const f = fixture(t, mode),
      result = migrate(f);
    assert.notEqual(result.status, 0, result.stdout);
    preserved(f);
    assert.equal(
      JSON.parse(readFileSync(join(f.dir, ".key-migration", "metadata.json")))
        .phase,
      "rolled_back",
    );
    assert.equal(
      existsSync(join(f.dir, ".key-migration", "source.key")),
      false,
    );
  });
for (const mode of ["semantic-drift", "rootless", "remote"])
  test(
    "refuses " + mode + " before stopping the service",
    { skip: !available },
    (t) => {
      const f = fixture(t, mode),
        result = migrate(f);
      assert.notEqual(result.status, 0, result.stdout);
      preserved(f);
      assert.doesNotMatch(result.stderr, /No such file|illegal option/);
      assert.equal(
        f.events().some((e) => e.action === "stop"),
        false,
      );
    },
  );
test(
  "does not overwrite an existing external key or follow a linked destination",
  { skip: !available },
  (t) => {
    const f = fixture(t);
    mkdirSync(f.target);
    writeFileSync(join(f.target, "credentials.key"), "unrelated synthetic key");
    assert.notEqual(migrate(f).status, 0);
    assert.equal(
      readFileSync(join(f.target, "credentials.key"), "utf8"),
      "unrelated synthetic key",
    );
    preserved(f);
    rmSync(f.target, { recursive: true });
    symlinkSync(f.data, f.target);
    assert.notEqual(migrate(f).status, 0);
    preserved(f);
  },
);
test(
  "refuses rollback over operator configuration changes and retains recovery keys",
  { skip: !available },
  (t) => {
    const f = fixture(t);
    assert.equal(migrate(f).status, 0);
    const changed =
      readFileSync(f.config, "utf8") + "# Later operator change\n";
    writeFileSync(f.config, changed);
    const result = run(f, ["--rollback"]);
    assert.notEqual(result.status, 0);
    assert.equal(readFileSync(f.config, "utf8"), changed);
    assert.deepEqual(readFileSync(join(f.target, "credentials.key")), key);
  },
);
test(
  "retains recovery material when automatic rollback cannot restart the service",
  { skip: !available },
  (t) => {
    const f = fixture(t, "rollback-failure"),
      result = migrate(f);
    assert.notEqual(result.status, 0);
    assert.deepEqual(
      readFileSync(join(f.dir, ".key-migration", "source.key")),
      key,
    );
    assert.match(result.stderr, /rollback|recovery/i);
    assert.equal(statSync(join(f.dir, ".key-migration")).mode & 0o777, 0o700);
  },
);

test(
  "refuses a running data-directory mismatch before downtime",
  { skip: !available },
  (t) => {
    const f = fixture(t, "runtime-drift"),
      result = migrate(f);
    assert.equal(existsSync(join(f.dir, ".key-migration")), false);
    assert.notEqual(result.status, 0);
    preserved(f);
    assert.equal(
      f.events().some((e) => e.action === "stop"),
      false,
    );
  },
);
test(
  "a newly created environment file prevents old-key deletion",
  { skip: !available },
  (t) => {
    const f = fixture(t, "env-appearance"),
      result = migrate(f);
    assert.notEqual(result.status, 0);
    assert.deepEqual(readFileSync(join(f.data, "credentials.key")), key);
    assert.equal(
      f.events().some((e) => e.action === "delete-old"),
      false,
    );
    assert.equal(
      readFileSync(join(f.dir, ".env"), "utf8"),
      "OPERATOR_CHANGE=preserve\n",
    );
  },
);

test(
  "recovers from a forced kill after deleting the old key",
  { skip: !available },
  (t) => {
    const f = fixture(t, "kill-after-delete"),
      result = migrate(f);
    assert.equal(result.signal, "SIGKILL");
    assert.equal(existsSync(join(f.data, "credentials.key")), false);
    assert.equal(
      JSON.parse(readFileSync(join(f.dir, ".key-migration", "metadata.json")))
        .phase,
      "removing_old",
    );
    rmSync(join(f.dir, ".update-lock"), { recursive: true });
    f.env.MIGRATION_MODE = "";
    const recovery = run(f, ["--rollback"]);
    assert.equal(recovery.status, 0, recovery.stderr + recovery.stdout);
    preserved(f);
  },
);
test(
  "refuses keys inside data, public original keys and an existing update lock",
  { skip: !available },
  (t) => {
    const f = fixture(t);
    const nested = run(f, ["--key-dir", join(f.data, "keys")]);
    assert.notEqual(nested.status, 0);
    preserved(f);
    assert.equal(existsSync(join(f.data, "keys")), false);
    const exposed = join(f.data, "credentials.key");
    const chmod = spawnSync("chmod", ["0644", exposed]);
    assert.equal(chmod.status, 0);
    assert.notEqual(migrate(f).status, 0);
    preserved(f);
    spawnSync("chmod", ["0600", exposed]);
    mkdirSync(join(f.dir, ".update-lock"));
    assert.notEqual(migrate(f).status, 0);
    assert.equal(existsSync(join(f.dir, ".update-lock")), true);
    assert.deepEqual(readFileSync(exposed), key);
    assert.equal(readFileSync(f.config, "utf8"), f.original);
    assert.equal(
      f.events().some((e) => e.action === "stop"),
      false,
    );
  },
);

test(
  "refuses pending data-mount changes before downtime",
  { skip: !available },
  (t) => {
    const f = fixture(t, "mount-drift"),
      result = migrate(f);
    assert.notEqual(result.status, 0);
    preserved(f);
    assert.equal(
      f.events().some((e) => e.action === "stop"),
      false,
    );
  },
);
test(
  "migrates and rolls back an existing named data volume",
  { skip: !available },
  (t) => {
    const f = fixture(t, "named-volume"),
      result = migrate(f);
    assert.equal(result.status, 0, result.stderr + result.stdout);
    assert.equal(existsSync(join(f.data, "credentials.key")), false);
    const rollback = run(f, ["--rollback"]);
    assert.equal(rollback.status, 0, rollback.stderr + rollback.stdout);
    preserved(f);
  },
);

test(
  "real CLI validates encrypted TOTP data through migration and rollback",
  { skip: !available || !process.env.SUBLANE_TEST_MIGRATION_BINARY },
  async (t) => {
    const f = fixture(t);
    rmSync(join(f.data, "sublane.db"));
    f.env.MIGRATION_REAL_BINARY = process.env.SUBLANE_TEST_MIGRATION_BINARY;
    const listener = createServer();
    listener.listen(0, "127.0.0.1");
    await once(listener, "listening");
    const port = listener.address().port;
    await new Promise((resolve) => listener.close(resolve));
    const origin = "http://127.0.0.1:" + port;
    let child;
    async function stop() {
      if (!child || child.exitCode !== null) return;
      const ended = once(child, "exit");
      child.kill("SIGTERM");
      const timer = setTimeout(() => child.kill("SIGKILL"), 12000);
      await ended;
      clearTimeout(timer);
    }
    async function start(external = "") {
      child = spawn(f.env.MIGRATION_REAL_BINARY, [], {
        env: {
          ...process.env,
          SUBLANE_ADDR: "127.0.0.1:" + port,
          SUBLANE_PUBLIC_URL: origin,
          SUBLANE_DATA_DIR: f.data,
          SUBLANE_CREDENTIALS_KEY_FILE: external,
          SUBLANE_DEMO: "false",
          SUBLANE_PRICING_URL: origin + "/synthetic-prices",
          SUBLANE_PRICING_HASH_URL: origin + "/synthetic-hash",
          HTTP_PROXY: "http://127.0.0.1:1",
          HTTPS_PROXY: "http://127.0.0.1:1",
          NO_PROXY: "127.0.0.1,localhost",
        },
        stdio: "ignore",
      });
      for (let i = 0; i < 200; i++) {
        assert.equal(child.exitCode, null, "Synthetic server exited");
        try {
          if ((await fetch(origin + "/readyz")).ok) return;
        } catch {}
        await new Promise((resolve) => setTimeout(resolve, 50));
      }
      assert.fail("Synthetic server did not become ready");
    }
    const cookies = new Map();
    async function post(path, body) {
      const response = await fetch(origin + path, {
        method: "POST",
        headers: {
          Origin: origin,
          "Content-Type": "application/json",
          Cookie: [...cookies].map(([k, v]) => k + "=" + v).join("; "),
        },
        body: JSON.stringify(body),
      });
      for (const cookie of response.headers.getSetCookie()) {
        const first = cookie.split(";")[0],
          index = first.indexOf("=");
        cookies.set(first.slice(0, index), first.slice(index + 1));
      }
      assert.equal(response.ok, true, await response.clone().text());
      return response.json();
    }
    function totp(secret) {
      const bits = [...secret]
        .map((c) =>
          "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
            .indexOf(c)
            .toString(2)
            .padStart(5, "0"),
        )
        .join("");
      const bytes = Buffer.from(bits.match(/.{8}/g).map((b) => parseInt(b, 2)));
      const counter = Buffer.alloc(8);
      counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 30000)));
      const h = createHmac("sha1", bytes).update(counter).digest();
      return String(
        (h.readUInt32BE(h[19] & 15) & 0x7fffffff) % 1000000,
      ).padStart(6, "0");
    }
    try {
      await start();
      await post("/api/auth/setup", {
        username: "synthetic-owner",
        password: "synthetic-password",
        workspace_name: "Synthetic",
      });
      const enrollment = await post("/api/me/totp", {
        current_password: "synthetic-password",
        code: "",
      });
      const factors = await post("/api/me/totp/confirm", {
        code: totp(enrollment.secret),
      });
      const migration = migrate(f);
      assert.equal(migration.status, 0, migration.stderr + migration.stdout);
      await stop();
      await start(join(f.target, "credentials.key"));
      cookies.clear();
      await post("/api/auth/login", {
        username: "synthetic-owner",
        password: "synthetic-password",
        code: factors.recovery_codes[0],
      });
      const rollback = run(f, ["--rollback"]);
      assert.equal(rollback.status, 0, rollback.stderr + rollback.stdout);
      await stop();
      await start();
      cookies.clear();
      await post("/api/auth/login", {
        username: "synthetic-owner",
        password: "synthetic-password",
        code: factors.recovery_codes[1],
      });
      assert.deepEqual(readFileSync(join(f.data, "credentials.key")), key);
    } finally {
      await stop();
    }
  },
);

test(
  "claims the external directory exclusively against another migration",
  { skip: !available },
  (t) => {
    const f = fixture(t, "destination-race"),
      result = migrate(f);
    assert.notEqual(result.status, 0);
    preserved(f);
    assert.equal(
      readFileSync(join(f.target, "credentials.key"), "utf8"),
      "another synthetic key",
    );
    assert.equal(
      f.events().some((e) => e.action === "stop"),
      false,
    );
  },
);
test(
  "refuses a linked parent into data and unsafe key-directory characters",
  { skip: !available },
  (t) => {
    const f = fixture(t),
      alias = join(f.root, "alias");
    symlinkSync(f.data, alias);
    assert.notEqual(run(f, ["--key-dir", join(alias, "keys")]).status, 0);
    preserved(f);
    for (const name of [
      "bad$key",
      "bad:key",
      "bad\\key",
      'bad"key',
      "bad\nkey",
    ]) {
      const result = run(f, ["--key-dir", join(f.root, name)]);
      assert.notEqual(result.status, 0, name);
      preserved(f);
    }
  },
);

test(
  "can roll back when publishing the new configuration fails",
  { skip: !available },
  (t) => {
    const f = fixture(t, "config-apply-failure"),
      result = migrate(f);
    assert.notEqual(result.status, 0);
    preserved(f);
    assert.equal(
      JSON.parse(readFileSync(join(f.dir, ".key-migration", "metadata.json")))
        .phase,
      "rolled_back",
    );
    assert.equal(
      existsSync(join(f.dir, ".key-migration", "source.key")),
      false,
    );
  },
);
test(
  "a corrupted external copy fails validation before downtime",
  { skip: !available },
  (t) => {
    const f = fixture(t, "copy-corruption"),
      result = migrate(f);
    assert.notEqual(result.status, 0);
    preserved(f);
    assert.equal(
      f.events().some((e) => e.action === "stop"),
      false,
    );
  },
);
test(
  "rollback refuses to overwrite a different key installed by an operator",
  { skip: !available },
  (t) => {
    const f = fixture(t);
    assert.equal(migrate(f).status, 0);
    const other = Buffer.alloc(32, 0x41);
    writeFileSync(join(f.data, "credentials.key"), other, { mode: 0o600 });
    const before = readFileSync(f.config, "utf8");
    const result = run(f, ["--rollback"]);
    assert.notEqual(result.status, 0);
    assert.deepEqual(readFileSync(join(f.data, "credentials.key")), other);
    assert.equal(readFileSync(f.config, "utf8"), before);
    assert.deepEqual(readFileSync(join(f.target, "credentials.key")), key);
  },
);

test(
  "does not clean recovery material when it failed to claim the state directory",
  { skip: !available },
  (t) => {
    const f = fixture(t, "state-race"),
      result = migrate(f);
    assert.notEqual(result.status, 0);
    preserved(f);
    assert.equal(
      readFileSync(join(f.dir, ".key-migration", "source.key"), "utf8"),
      "another synthetic recovery key",
    );
  },
);
test(
  "refuses unknown recovery formats before changing the service",
  { skip: !available },
  (t) => {
    const f = fixture(t);
    assert.equal(migrate(f).status, 0);
    const file = join(f.dir, ".key-migration", "metadata.json"),
      metadata = JSON.parse(readFileSync(file));
    metadata.version = 2;
    writeFileSync(file, JSON.stringify(metadata));
    const before = readFileSync(f.config, "utf8");
    const result = run(f, ["--rollback"]);
    assert.notEqual(result.status, 0);
    assert.equal(readFileSync(f.config, "utf8"), before);
    assert.equal(existsSync(join(f.data, "credentials.key")), false);
  },
);

test("key-path containment includes the filesystem root without accepting sibling prefixes", () => {
  // Exercise the pure shell predicate without executing privileged migration setup.
  const definition = readFileSync(script, "utf8").match(
    /^inside\(\) \{.*\}$/m,
  )?.[0];
  assert.ok(definition);
  for (const [child, parent, status] of [
    ["/srv/keys", "/", 0],
    ["/", "/", 0],
    ["relative", "/", 1],
    ["/data/keys", "/data", 0],
    ["/data", "/data", 0],
    ["/database/keys", "/data", 1],
  ]) {
    const result = spawnSync("bash", [
      "-c",
      definition + '\ninside "$1" "$2"',
      "containment",
      child,
      parent,
    ]);
    assert.equal(result.status, status, `${child} within ${parent}`);
  }
});

test(
  "configuration reads are locked against installer updates",
  { skip: !available },
  (t) => {
    const f = fixture(t),
      result = migrate(f);
    assert.equal(result.status, 0, result.stderr + result.stdout);
    assert.ok(
      f
        .events()
        .filter((e) => e.action === "configuration")
        .every((e) => e.locked),
    );
  },
);
test(
  "refuses an ambiguous separately mounted legacy key before journaling",
  { skip: !available },
  (t) => {
    const f = fixture(t, "legacy-key-mount"),
      result = migrate(f);
    assert.notEqual(result.status, 0);
    preserved(f);
    assert.equal(existsSync(join(f.dir, ".key-migration")), false);
    assert.equal(
      f.events().some((e) => e.action === "stop"),
      false,
    );
  },
);
test(
  "rollback refuses ambient interpolation drift before stopping",
  { skip: !available },
  (t) => {
    const f = fixture(t);
    assert.equal(migrate(f).status, 0);
    const before = readFileSync(f.config, "utf8");
    const offset = f.events().length;
    f.env.SUBLANE_IMAGE = "example.test/sublane:changed";
    const result = run(f, ["--rollback"]);
    assert.notEqual(result.status, 0);
    assert.equal(readFileSync(f.config, "utf8"), before);
    assert.equal(existsSync(join(f.data, "credentials.key")), false);
    assert.equal(
      f
        .events()
        .slice(offset)
        .some((e) => e.action === "stop"),
      false,
    );
  },
);
test(
  "failed original verification and explicit rollback leave the untouched service running",
  { skip: !available },
  (t) => {
    const f = fixture(t, "original-verification-failure"),
      result = migrate(f);
    assert.notEqual(result.status, 0);
    preserved(f);
    assert.notEqual(run(f, ["--rollback"]).status, 0);
    preserved(f);
    assert.equal(
      f.events().some((e) => e.action === "stop"),
      false,
    );
  },
);

test(
  "rollback refuses a different Docker daemon before stopping",
  { skip: !available },
  (t) => {
    const f = fixture(t);
    assert.equal(migrate(f).status, 0);
    const before = readFileSync(f.config, "utf8"),
      offset = f.events().length;
    f.env.MIGRATION_MODE = "changed-daemon";
    const result = run(f, ["--rollback"]);
    assert.notEqual(result.status, 0);
    assert.equal(readFileSync(f.config, "utf8"), before);
    assert.equal(existsSync(join(f.data, "credentials.key")), false);
    assert.equal(
      f
        .events()
        .slice(offset)
        .some((e) => e.action === "stop"),
      false,
    );
  },
);

for (const mode of ["runtime-image-drift", "owner-drift"])
  test(
    "rollback preserves a runtime whose identity changed: " + mode,
    { skip: !available },
    (t) => {
      const f = fixture(t);
      assert.equal(migrate(f).status, 0);
      const before = readFileSync(f.config, "utf8"),
        offset = f.events().length;
      f.env.MIGRATION_MODE = mode;
      const result = run(f, ["--rollback"]);
      assert.notEqual(result.status, 0);
      assert.equal(readFileSync(f.config, "utf8"), before);
      assert.equal(existsSync(join(f.data, "credentials.key")), false);
      assert.equal(
        f
          .events()
          .slice(offset)
          .some((e) => e.action === "stop"),
        false,
      );
    },
  );

test(
  "ignores one-off verification containers when selecting the service",
  { skip: !available },
  (t) => {
    const f = fixture(t, "one-off-present"),
      result = migrate(f);
    assert.equal(result.status, 0, result.stderr + result.stdout);
    const rollback = run(f, ["--rollback"]);
    assert.equal(rollback.status, 0, rollback.stderr + rollback.stdout);
    preserved(f);
  },
);
