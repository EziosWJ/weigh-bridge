import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const backend = join(root, "server");
const [mode, ...args] = process.argv.slice(2);

function command(name, args) {
  // npm/Task on Windows may be command shims; no user arguments reach a shell.
  return spawnSync(name, args, {
    cwd: backend,
    encoding: "utf8",
    timeout: 10000,
    windowsHide: true,
    shell: process.platform === "win32" && name === "npm",
  });
}

function run() {
  if (!["init", "doctor"].includes(mode)) {
    console.error("用法：task init [-- --database postgres] 或 task doctor");
    return 1;
  }
  if (args.includes("--help") || args.includes("-h")) {
    console.log(`task ${mode} [-- --database postgres]`);
    return 0;
  }

  let failed = false;
  let canCompile = true;
  function check(ok, label, remedy) {
    console.log(`${ok ? "[通过]" : "[失败]"} ${label}${ok ? "" : `：${remedy}`}`);
    if (!ok) failed = true;
  }

  const go = command("go", ["version"]);
  canCompile = go.status === 0;
  check(canCompile, "Go", "安装 Go 1.26 或更高版本，并加入 PATH");

  if (mode === "doctor") {
    check(true, "Node.js");
    check(command("npm", ["--version"]).status === 0, "npm", "安装 npm 并加入 PATH");
    check(command("task", ["--version"]).status === 0, "Task", "安装 Task 并加入 PATH");
    check(existsSync(join(root, "web", "node_modules", "vite", "package.json")),
      "前端依赖", "在项目根目录执行 npm --prefix web ci");
  }

  if (canCompile) {
    const result = spawnSync("go", ["run", "./cmd/dev-setup", mode, ...args], {
      cwd: backend,
      stdio: "inherit",
      env: process.env,
    });
    if (result.error || result.status !== 0) failed = true;
  }
  return failed ? 1 : 0;
}

process.exitCode = run();
