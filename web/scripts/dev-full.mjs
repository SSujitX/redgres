import { spawn, spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const DEV_HOST = "127.0.0.1";
const DEV_PORT = 8989;
const DEV_ORIGIN = `http://${DEV_HOST}:${DEV_PORT}`;

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const webDirectory = path.resolve(scriptDirectory, "..");
const repositoryDirectory = path.resolve(webDirectory, "..");
const assetDirectory = path.join(repositoryDirectory, "internal", "web", "dist", "app");
const goCommand = process.platform === "win32" ? "go.exe" : "go";
const viteEntry = path.join(webDirectory, "node_modules", "vite", "bin", "vite.js");
const npmCliFromNode = path.join(path.dirname(process.execPath), "node_modules", "npm", "bin", "npm-cli.js");

const npmRunner = (() => {
  if (process.platform === "win32" && process.env.npm_execpath) {
    return { command: process.execPath, args: [process.env.npm_execpath] };
  }
  if (process.platform === "win32" && existsSync(npmCliFromNode)) {
    return { command: process.execPath, args: [npmCliFromNode] };
  }
  return { command: process.platform === "win32" ? "npm.cmd" : "npm", args: [] };
})();

function runSetup(command, args, cwd) {
  const result = spawnSync(command, args, {
    cwd,
    stdio: "inherit",
    windowsHide: true,
    shell: false,
  });
  if (result.error) {
    console.error(`Unable to start ${command}: ${result.error.message}`);
    process.exit(1);
  }
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

if (!existsSync(viteEntry)) {
  console.log("Frontend dependencies are missing; running npm ci...");
  runSetup(npmRunner.command, [...npmRunner.args, "ci"], webDirectory);
}

console.log("Building the frontend...");
runSetup(npmRunner.command, [...npmRunner.args, "run", "build"], webDirectory);

const developmentEnvironment = {
  ...process.env,
  REDGRES_ENVIRONMENT: "development",
  REDGRES_ADDRESS: `${DEV_HOST}:${DEV_PORT}`,
  REDGRES_BASE_URL: DEV_ORIGIN,
  REDGRES_COOKIE_SECURE: "false",
  REDGRES_DEV_ASSET_DIR: assetDirectory,
};

const childOptions = {
  stdio: "inherit",
  windowsHide: true,
  detached: process.platform !== "win32",
};

console.log(`Starting Redgres at ${DEV_ORIGIN}`);
console.log("Frontend rebuilds automatically; refresh the browser after UI edits.");

const backend = spawn(goCommand, ["run", "./cmd/redgres", "serve"], {
  ...childOptions,
  cwd: repositoryDirectory,
  env: developmentEnvironment,
});

const frontendWatch = spawn(process.execPath, [viteEntry, "build", "--watch"], {
  ...childOptions,
  cwd: webDirectory,
  env: process.env,
});

let stopping = false;

function terminate(child) {
  if (!child.pid || child.exitCode !== null) {
    return;
  }
  try {
    if (process.platform === "win32") {
      spawnSync("taskkill", ["/pid", String(child.pid), "/t", "/f"], {
        stdio: "ignore",
        windowsHide: true,
      });
    } else {
      process.kill(-child.pid, "SIGTERM");
    }
  } catch {
    // The process may have exited between the state check and termination.
  }
}

function shutdown(exitCode) {
  if (stopping) {
    return;
  }
  stopping = true;
  terminate(frontendWatch);
  terminate(backend);
  process.exit(exitCode);
}

function handleError(name, error) {
  console.error(`${name} failed to start: ${error.message}`);
  shutdown(1);
}

function handleExit(name, code, signal) {
  if (stopping) {
    return;
  }
  const outcome = signal ? `signal ${signal}` : `code ${code ?? 1}`;
  console.error(`${name} stopped (${outcome}); stopping the development stack.`);
  shutdown(code ?? 1);
}

backend.on("error", (error) => handleError("Redgres", error));
frontendWatch.on("error", (error) => handleError("Vite build watch", error));
backend.on("exit", (code, signal) => handleExit("Redgres", code, signal));
frontendWatch.on("exit", (code, signal) => handleExit("Vite build watch", code, signal));
process.on("SIGINT", () => shutdown(0));
process.on("SIGTERM", () => shutdown(0));
