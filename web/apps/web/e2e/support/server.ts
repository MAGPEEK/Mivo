// Builds the real server binary with the embedded web app and runs it in a throwaway
// environment, so the E2E suite never touches the developer's config, data or fixed ports.
import { productId } from "@mivo/ui-tokens";
import { execFile, spawn, spawnSync, type ChildProcess } from "node:child_process";
import { existsSync } from "node:fs";
import {
  mkdir,
  mkdtemp,
  open,
  readdir,
  readFile,
  rm,
  stat,
  type FileHandle,
} from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const runFile = promisify(execFile);

const repositoryRoot = fileURLToPath(new URL("../../../../../", import.meta.url));
// Vite writes the production build here and `-tags webui` embeds it (ADR 0020).
const webBuildIndex = join(
  repositoryRoot,
  "internal",
  "adapters",
  "http",
  "webui",
  "dist",
  "index.html",
);
// A Go package path, not a product name: renaming the product does not move the code.
const serverPackage = "./cmd/mivo";

/** Only the loopback interface: the test server must not be reachable from the network. */
export const loopbackHost = "127.0.0.1";

const envPrefix = `${productId.toUpperCase()}_`;
const executableSuffix = process.platform === "win32" ? ".exe" : "";
const temporaryRootPrefix = `${productId}-e2e-`;

// A cold build compiles the whole module and its dependencies; a hung toolchain must still
// fail the run instead of blocking it.
const buildTimeoutMs = 5 * 60_000;
// Room for a few large Go compiler messages before execFile gives up on the output.
const buildOutputLimitBytes = 16 * 1024 * 1024;
const healthPath = "/api/v1/health";
const startupTimeoutMs = 30_000;
const healthPollIntervalMs = 100;
const healthRequestTimeoutMs = 1_000;
// Time the server gets per stop signal before the next, harder one.
const stopTimeoutMs = 5_000;
const logTailBytes = 4_000;
// Windows releases file handles of a killed process with a short delay.
const removeRetries = 10;
const removeRetryDelayMs = 200;
// A run that was killed before its teardown leaves its directory behind. Older ones are
// removed; younger ones may belong to a run that is still going.
const staleRootAgeMs = 6 * 60 * 60_000;

/** A running E2E server. */
export interface E2EServer {
  /** Origin the server listens on, e.g. `http://127.0.0.1:53124`. */
  readonly baseUrl: string;
  /** Stops the server and deletes its temporary directory. */
  readonly stop: () => Promise<void>;
}

/** Options of {@link startE2EServer}. */
export interface E2EServerOptions {
  /** File that receives the server's log output, kept for debugging failed runs. */
  readonly logFile: string;
}

/**
 * Builds the binary into a fresh temporary directory, starts it on a free loopback port with
 * every server and OS directory inside that directory, and waits until it reports healthy.
 *
 * The binary embeds whatever web build exists. `pnpm -C web e2e` rebuilds it first; running
 * `playwright test` directly tests the last web build.
 * @throws Error with a clear reason when Go or the web build is missing, the build fails or
 *   the server does not become healthy.
 */
export async function startE2EServer(options: E2EServerOptions): Promise<E2EServer> {
  assertGoAvailable();
  assertWebBuildExists();
  await removeStaleRoots();
  const root = await mkdtemp(join(tmpdir(), temporaryRootPrefix));
  let launched: LaunchedProcess | undefined;
  try {
    const binary = await buildServer(root);
    const port = await findFreePort();
    const running = await launch(binary, await isolatedEnvironment(root, port), options.logFile);
    launched = running;
    const baseUrl = `http://${loopbackHost}:${String(port)}`;
    await waitUntilHealthy(baseUrl, running.exited, options.logFile);
    return {
      baseUrl,
      stop: async () => {
        try {
          await running.stop();
        } finally {
          await removeDirectory(root);
        }
      },
    };
  } catch (error) {
    await cleanUpAfterFailedStart(launched, root);
    throw error;
  }
}

// The start error explains what went wrong; a cleanup error thrown in its place would hide
// it, so cleanup problems are only reported.
async function cleanUpAfterFailedStart(launched: LaunchedProcess | undefined, root: string) {
  try {
    await launched?.stop();
  } catch (stopError) {
    reportCleanupError(stopError);
  } finally {
    await removeDirectory(root).catch(reportCleanupError);
  }
}

function reportCleanupError(error: unknown): void {
  console.warn("E2E server cleanup failed:", error);
}

function removeDirectory(directory: string): Promise<void> {
  return rm(directory, {
    recursive: true,
    force: true,
    maxRetries: removeRetries,
    retryDelay: removeRetryDelayMs,
  });
}

async function removeStaleRoots(): Promise<void> {
  const parent = tmpdir();
  const names = await readdir(parent).catch((): string[] => []);
  const cutoff = Date.now() - staleRootAgeMs;
  const stale = names.filter((name) => name.startsWith(temporaryRootPrefix));
  await Promise.all(
    stale.map(async (name) => {
      const directory = join(parent, name);
      const info = await stat(directory).catch(() => undefined);
      if (info?.isDirectory() === true && info.mtimeMs < cutoff) {
        await removeDirectory(directory).catch(reportCleanupError);
      }
    }),
  );
}

function assertGoAvailable(): void {
  const result = spawnSync("go", ["version"], { encoding: "utf8" });
  if (result.error !== undefined || result.status !== 0) {
    throw new Error(
      "Go was not found on PATH. The E2E suite builds the server with `go build -tags webui`; " +
        "install the Go version from go.mod and open a new shell.",
      { cause: result.error },
    );
  }
}

function assertWebBuildExists(): void {
  if (!existsSync(webBuildIndex)) {
    throw new Error(
      `No web build at ${webBuildIndex}. Run \`pnpm -C web e2e\`, which builds the web app ` +
        "first; `playwright test` alone embeds the last web build.",
    );
  }
}

async function buildServer(root: string): Promise<string> {
  const binary = join(root, "bin", `${productId}${executableSuffix}`);
  const args = ["build", "-tags", "webui", "-o", binary, serverPackage];
  const command = `go ${args.join(" ")}`;
  try {
    await runFile("go", args, {
      cwd: repositoryRoot,
      maxBuffer: buildOutputLimitBytes,
      timeout: buildTimeoutMs,
    });
  } catch (error) {
    if (error instanceof Error && "killed" in error && error.killed === true) {
      throw new Error(`${command} did not finish within ${String(buildTimeoutMs)} ms.`, {
        cause: error,
      });
    }
    const output = error instanceof Error && "stderr" in error ? String(error.stderr) : "";
    throw new Error(`${command} failed:\n${output}`, { cause: error });
  }
  return binary;
}

// The OS picks the port; closing the probe frees it for the server a moment later. Another
// process could claim it in between, which then fails loudly as "exited before healthy".
async function findFreePort(): Promise<number> {
  const probe = createServer();
  await new Promise<void>((resolve, reject) => {
    probe.once("error", reject);
    probe.listen(0, loopbackHost, resolve);
  });
  const address = probe.address();
  await new Promise<void>((resolve) => {
    probe.close(() => {
      resolve();
    });
  });
  if (address === null || typeof address === "string") {
    throw new Error("Could not determine a free TCP port.");
  }
  return address.port;
}

/**
 * The inherited environment without any server setting, plus server and OS base directories
 * that all point into `root`. OS directories are replaced as well, so a default the server
 * derives from them can never reach the real profile.
 */
async function isolatedEnvironment(root: string, port: number): Promise<NodeJS.ProcessEnv> {
  const home = join(root, "home");
  const temporary = join(root, "tmp");
  const directories: Record<string, string> = {
    LOCALAPPDATA: join(home, "AppData", "Local"),
    APPDATA: join(home, "AppData", "Roaming"),
    USERPROFILE: home,
    HOME: home,
    XDG_CONFIG_HOME: join(home, ".config"),
    XDG_DATA_HOME: join(home, ".local", "share"),
    XDG_CACHE_HOME: join(home, ".cache"),
    TEMP: temporary,
    TMP: temporary,
    TMPDIR: temporary,
    [`${envPrefix}CONFIG_DIR`]: join(root, "config"),
    [`${envPrefix}PATHS_DATA`]: join(root, "data"),
    [`${envPrefix}PATHS_CACHE`]: join(root, "cache"),
    [`${envPrefix}PATHS_TRANSCODE`]: join(root, "transcode"),
  };
  await Promise.all(Object.values(directories).map((path) => mkdir(path, { recursive: true })));
  const overrides: Record<string, string> = {
    ...directories,
    [`${envPrefix}SERVER_HOST`]: loopbackHost,
    [`${envPrefix}SERVER_PORT`]: String(port),
  };
  return { ...inheritedEnvironment(Object.keys(overrides)), ...overrides };
}

// Windows treats variable names case-insensitively, so "LocalAppData" must not survive next
// to the override "LOCALAPPDATA".
function inheritedEnvironment(overriddenNames: readonly string[]): NodeJS.ProcessEnv {
  const replaced = new Set(overriddenNames.map((name) => name.toUpperCase()));
  return Object.fromEntries(
    Object.entries(process.env).filter(([name]) => {
      const upperName = name.toUpperCase();
      return !upperName.startsWith(envPrefix) && !replaced.has(upperName);
    }),
  );
}

interface LaunchedProcess {
  /** Resolves with a description of how the process ended. */
  readonly exited: Promise<string>;
  /** Stops the process, forcefully if needed, and closes its log. */
  readonly stop: () => Promise<void>;
}

async function launch(
  binary: string,
  environment: NodeJS.ProcessEnv,
  logFile: string,
): Promise<LaunchedProcess> {
  const log = await open(logFile, "w");
  const child = spawn(binary, [], { env: environment, stdio: ["ignore", log.fd, log.fd] });
  const exited = new Promise<string>((resolve) => {
    child.once("error", (error) => {
      resolve(`could not start: ${error.message}`);
    });
    child.once("exit", (code, signal) => {
      resolve(`exited with ${code === null ? `signal ${String(signal)}` : `code ${String(code)}`}`);
    });
  });
  // The teardown never runs when the runner ends abruptly, e.g. on a crash or process.exit.
  // An "exit" listener may only do synchronous work, which is enough to kill the server.
  const killOnRunnerExit = () => {
    child.kill();
  };
  process.once("exit", killOnRunnerExit);
  void exited.then(() => {
    process.off("exit", killOnRunnerExit);
  });
  return { exited, stop: () => stopProcess(child, exited, log) };
}

async function stopProcess(child: ChildProcess, exited: Promise<string>, log: FileHandle) {
  try {
    const stopped =
      (await signalAndWait(child, exited, "SIGTERM")) ||
      (await signalAndWait(child, exited, "SIGKILL"));
    if (!stopped) {
      throw new Error(
        `The server (pid ${String(child.pid)}) did not stop after SIGTERM and SIGKILL.`,
      );
    }
  } finally {
    await log.close();
  }
}

/** Sends `signal` unless the process has ended and reports whether it ends in time. */
async function signalAndWait(
  child: ChildProcess,
  exited: Promise<string>,
  signal: NodeJS.Signals,
): Promise<boolean> {
  if (child.exitCode === null && child.signalCode === null) {
    child.kill(signal);
  }
  // Unreferenced, so a pending timer never keeps the runner alive after the server ended.
  const timedOut = delay(stopTimeoutMs, false, { ref: false });
  return Promise.race([exited.then(() => true), timedOut]);
}

async function waitUntilHealthy(baseUrl: string, exited: Promise<string>, logFile: string) {
  let exitReason: string | undefined;
  void exited.then((reason) => {
    exitReason = reason;
  });
  const deadline = Date.now() + startupTimeoutMs;
  while (Date.now() < deadline) {
    if (exitReason !== undefined) {
      throw await startupError(`The server ${exitReason} before it became healthy.`, logFile);
    }
    if (await isHealthy(baseUrl)) {
      return;
    }
    await delay(healthPollIntervalMs);
  }
  throw await startupError(
    `The server did not answer ${baseUrl}${healthPath} within ${String(startupTimeoutMs)} ms.`,
    logFile,
  );
}

async function isHealthy(baseUrl: string): Promise<boolean> {
  try {
    const response = await fetch(`${baseUrl}${healthPath}`, {
      signal: AbortSignal.timeout(healthRequestTimeoutMs),
    });
    // Only the status matters; an unread body would keep the connection busy.
    await response.body?.cancel();
    return response.ok;
  } catch {
    return false;
  }
}

async function startupError(reason: string, logFile: string): Promise<Error> {
  const log = await readFile(logFile, "utf8").catch(() => "");
  return new Error(`${reason}\nLast server output (${logFile}):\n${log.slice(-logTailBytes)}`);
}
