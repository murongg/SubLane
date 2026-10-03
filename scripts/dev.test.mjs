import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createServer } from "node:net";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

const script = fileURLToPath(new URL("./dev.mjs", import.meta.url));

async function fixture(t, { failBackend = false, address } = {}) {
  if (!address) {
    const listener = createServer();
    listener.listen(0, "127.0.0.1");
    await once(listener, "listening");
    address = `127.0.0.1:${listener.address().port}`;
    await new Promise((resolve) => listener.close(resolve));
  }
  const cwd = mkdtempSync(join(tmpdir(), "sublane-dev-test-"));
  const bin = join(cwd, "commands");
  const log = join(cwd, "events.jsonl");
  mkdirSync(bin);
  const worker = `
    const fs = require("node:fs");
    const role = process.argv[1];
    const record = (event) => fs.appendFileSync(${JSON.stringify(log)}, JSON.stringify({ role, event, pid: process.pid }) + "\\n");
    record("started");
    process.on("SIGTERM", () => { record("stopped"); process.exit(0); });
    setInterval(() => {}, 1000);
  `;
  for (const role of ["make", "pnpm"]) {
    writeFileSync(join(bin, role), `#!/usr/bin/env node
      const fs = require("node:fs");
      const record = (event) => fs.appendFileSync(${JSON.stringify(log)}, JSON.stringify({ role: ${JSON.stringify(role)}, event, pid: process.pid, args: process.argv.slice(2), address: process.env.SUBLANE_ADDR }) + "\\n");
      process.on("SIGTERM", () => { record("stopped"); process.exit(0); });
      record("started");
      ${role === "make" ? `require("node:child_process").spawn(process.execPath, ["-e", ${JSON.stringify(worker)}, "backend"]);` : ""}
      ${role === "make" && failBackend ? "setTimeout(() => process.exit(17), 300);" : ""}
      setInterval(() => {}, 1000);
    `, { mode: 0o755 });
  }
  // A direct Go build bypasses the watcher and must fail without touching real output.
  writeFileSync(join(bin, "go"), "#!/usr/bin/env node\nprocess.exit(23);\n", { mode: 0o755 });
  const child = spawn(process.execPath, [script, "--port=5197"], {
    cwd,
    env: { ...process.env, SUBLANE_ADDR: address, PATH: `${bin}:${process.env.PATH}` },
    stdio: ["ignore", "ignore", "pipe"],
  });
  let stderr = "";
  child.stderr.on("data", (chunk) => { stderr += chunk; });
  const exited = once(child, "exit");
  const events = () => existsSync(log) ? readFileSync(log, "utf8").trim().split("\n").filter(Boolean).map(JSON.parse) : [];
  t.after(async () => {
    if (child.exitCode === null && child.signalCode === null) {
      child.kill("SIGTERM");
      await exited;
    }
    for (const event of events().filter((entry) => entry.event === "started")) {
      try { process.kill(event.pid, "SIGKILL"); } catch (error) { if (error.code !== "ESRCH") throw error; }
    }
    rmSync(cwd, { recursive: true, force: true });
  });
  return { child, exited, events, address, stderr: () => stderr };
}

async function waitFor(predicate) {
  for (let attempt = 0; attempt < 200; attempt++) {
    if (predicate()) return;
    await delay(25);
  }
  assert.fail("Timed out waiting for development processes");
}

test("starts the backend watcher with Vite and stops every descendant on interrupt", { skip: process.platform === "win32", timeout: 10000 }, async (t) => {
  const f = await fixture(t);
  await waitFor(() => f.events().filter((event) => event.event === "started").length === 3 || f.child.exitCode !== null);
  assert.equal(f.child.exitCode, null, "the development launcher must stay running");
  assert.deepEqual(f.events().find((event) => event.role === "make").args, ["dev-api"]);
  assert.deepEqual(f.events().find((event) => event.role === "pnpm").args, ["--dir", "web", "dev", "--port=5197"]);
  assert.equal(f.events().find((event) => event.role === "make").address, f.address);
  assert.equal(f.events().find((event) => event.role === "pnpm").address, f.address);
  f.child.kill("SIGINT");
  assert.deepEqual(await f.exited, [0, null]);
  await waitFor(() => f.events().filter((event) => event.event === "stopped").length === 3);
});

test("a failed backend watcher stops Vite and returns its failure status", { skip: process.platform === "win32", timeout: 10000 }, async (t) => {
  const f = await fixture(t, { failBackend: true });
  assert.deepEqual(await f.exited, [17, null]);
  await waitFor(() => f.events().some((event) => event.role === "pnpm" && event.event === "stopped"));
});

test("an occupied backend port fails before starting Air or Vite", { timeout: 10000 }, async (t) => {
  const listener = createServer();
  listener.listen(0, "127.0.0.1");
  await once(listener, "listening");
  t.after(() => new Promise((resolve) => listener.close(resolve)));
  const address = `127.0.0.1:${listener.address().port}`;
  const f = await fixture(t, { address });
  await waitFor(() => f.child.exitCode !== null || f.events().length > 0);
  assert.equal(f.child.exitCode, 1, "the occupied port must stop the launcher before spawning children");
  assert.deepEqual(await f.exited, [1, null]);
  assert.deepEqual(f.events(), []);
  assert.match(f.stderr(), /already in use/);
  assert.ok(f.stderr().includes(address));
  assert.match(f.stderr(), /SUBLANE_ADDR/);
});
