import { cpSync, mkdirSync, rmSync, statSync } from "node:fs";
import { fileURLToPath } from "node:url";

const source = fileURLToPath(new URL("../web/dist/", import.meta.url));
const target = fileURLToPath(new URL("../server/internal/webui/dist/", import.meta.url));

// Validate the build before replacing the previous embedded assets.
statSync(new URL("../web/dist/index.html", import.meta.url));
rmSync(target, { recursive: true, force: true });
cpSync(source, target, { recursive: true });
mkdirSync(new URL("../bin/", import.meta.url), { recursive: true });
