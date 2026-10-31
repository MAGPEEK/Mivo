// Runs the developer tools the E2E setup needs (Go, FFmpeg) with clear errors instead of
// timeouts when a tool is missing, hangs or fails.
import { execFile, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const runFile = promisify(execFile);

/** Root of the repository; Go commands run here. */
export const repositoryRoot = fileURLToPath(new URL("../../../../../", import.meta.url));

// Room for a few large compiler or FFmpeg messages before execFile gives up on the output.
const outputLimitBytes = 16 * 1024 * 1024;
// Node's error code when a child process writes more than maxBuffer.
const outputLimitErrorCode = "ERR_CHILD_PROCESS_STDIO_MAXBUFFER";

/**
 * Throws `message` when `command args` cannot be started or fails, e.g. because the tool is
 * not on PATH. Meant for quick version queries before the real work starts.
 */
export function assertCommandAvailable(
  command: string,
  args: readonly string[],
  message: string,
): void {
  const result = spawnSync(command, args, { encoding: "utf8" });
  if (result.error !== undefined || result.status !== 0) {
    throw new Error(message, { cause: result.error });
  }
}

/** Options of {@link runCommand}. */
export interface RunCommandOptions {
  /** A hung tool must still fail the run instead of blocking it. */
  readonly timeoutMs: number;
  /** Working directory; defaults to the repository root. */
  readonly cwd?: string;
}

/**
 * Runs `command args` to completion.
 * @throws Error naming the command with its error output, an exceeded timeout or an exceeded
 *   output limit.
 */
export async function runCommand(
  command: string,
  args: readonly string[],
  options: RunCommandOptions,
): Promise<void> {
  try {
    await runFile(command, args, {
      cwd: options.cwd ?? repositoryRoot,
      maxBuffer: outputLimitBytes,
      timeout: options.timeoutMs,
    });
  } catch (error) {
    throw commandError(`${command} ${args.join(" ")}`, options.timeoutMs, error);
  }
}

// An exceeded maxBuffer is only recognisable by its error code; `killed` marks the timeout.
function commandError(commandLine: string, timeoutMs: number, error: unknown): Error {
  if (!(error instanceof Error)) {
    return new Error(`${commandLine} failed.`, { cause: error });
  }
  if ("code" in error && error.code === outputLimitErrorCode) {
    const limit = String(outputLimitBytes);
    return new Error(`${commandLine} wrote more than ${limit} bytes of output.`, { cause: error });
  }
  if ("killed" in error && error.killed === true) {
    return new Error(`${commandLine} did not finish within ${String(timeoutMs)} ms.`, {
      cause: error,
    });
  }
  const output = "stderr" in error ? String(error.stderr) : "";
  return new Error(`${commandLine} failed:\n${output}`, { cause: error });
}
