import assert from "node:assert/strict";
import { test } from "node:test";
import { cpSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";

function fixture(t, { tools = ["go", "gcc", "task", "npm"], cgo = "1", helperStatus = 0, dependencies = true } = {}) {
  const root = mkdtempSync(join(tmpdir(), "dev-setup-test-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  for (const dir of ["scripts", "server", "tools"]) mkdirSync(join(root, dir));
  cpSync(new URL("./dev-setup.mjs", import.meta.url), join(root, "scripts", "dev-setup.mjs"));
  if (dependencies) {
    mkdirSync(join(root, "web", "node_modules", "vite"), { recursive: true });
    writeFileSync(join(root, "web", "node_modules", "vite", "package.json"), "{}");
  }
  for (const tool of tools) {
    const body = tool === "go"
      ? `if [ "$1" = "env" ]; then printf '%s' '{"CGO_ENABLED":"${cgo}","CC":"gcc"}'; elif [ "$1" = "run" ]; then printf '%s\\n' 'helper invoked'; exit ${helperStatus}; else printf '%s\\n' 'version'; fi`
      : "exit 0";
    writeFileSync(join(root, "tools", tool), `#!/bin/sh\n${body}\n`, { mode: 0o755 });
  }
  return (...args) => spawnSync(process.execPath, [join(root, "scripts", "dev-setup.mjs"), ...args], {
    encoding: "utf8",
    env: { ...process.env, PATH: join(root, "tools") },
    timeout: 10000,
  });
}

// Executable shell fixtures model missing tools without touching the host PATH.
const unixOnly = { skip: process.platform === "win32" };

test("doctor reports missing Go before compiling and diagnoses other tools", unixOnly, (t) => {
  const run = fixture(t, { tools: [], dependencies: false });
  const result = run("doctor");
  assert.equal(result.status, 1);
  for (const tool of ["Go", "npm", "Task", "前端依赖"]) assert.match(result.stdout, new RegExp(`\\[失败\\] ${tool}`));
  assert.doesNotMatch(result.stdout, /helper invoked/);
});

test("doctor works without CGO or a C compiler", unixOnly, (t) => {
  const result = fixture(t, { tools: ["go", "task", "npm"], cgo: "0" })("doctor");
  assert.equal(result.status, 0);
  assert.doesNotMatch(result.stdout, /\[失败\]/);
  assert.match(result.stdout, /helper invoked/);
});

test("doctor checks configuration even if frontend dependencies are missing", unixOnly, (t) => {
  const result = fixture(t, { dependencies: false })("doctor");
  assert.equal(result.status, 1);
  assert.match(result.stdout, /npm --prefix web ci/);
  assert.match(result.stdout, /helper invoked/);
});

test("bootstrap propagates helper failure and successful initialization", unixOnly, (t) => {
  assert.equal(fixture(t, { helperStatus: 1 })("doctor").status, 1);
  const result = fixture(t)("init", "--database", "postgres");
  assert.equal(result.status, 0);
  assert.match(result.stdout, /helper invoked/);
});

test("help works without Go and invalid mode fails", unixOnly, (t) => {
  const run = fixture(t, { tools: [] });
  assert.equal(run("init", "--help").status, 0);
  assert.equal(run("unknown").status, 1);
});
