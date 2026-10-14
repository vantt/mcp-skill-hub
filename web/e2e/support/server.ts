import { execFileSync, spawn } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHubSkill, gitIdentity, makeCLIRunner } from './seed-workspace';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export interface RunningServer {
  url: string;
  origin: string;
  ws: string;
  stop: () => Promise<void>;
}

export interface StartServerOptions {
  workspace?: string;
  // Environment for the spawned server; defaults to an isolated environment whose HOME,
  // XDG_CONFIG_HOME and per-user Skill Hub config live in a temporary folder.
  env?: NodeJS.ProcessEnv;
}

// isolatedServerEnv keeps every skillhub process of the suite away from the real user
// profile: `skillhub init` records the hub path in the per-user config.
function isolatedServerEnv(root: string): NodeJS.ProcessEnv {
  const home = path.join(root, 'home');
  fs.mkdirSync(home, { recursive: true });
  return {
    ...process.env,
    ...gitIdentity,
    HOME: home,
    USERPROFILE: home,
    XDG_CONFIG_HOME: path.join(root, 'config'),
    APPDATA: path.join(root, 'appdata'),
    LOCALAPPDATA: path.join(root, 'localappdata'),
    SKILLHUB_USER_CONFIG_DIR: path.join(root, 'skillhub-user-config'),
  };
}

export async function startServer(options?: StartServerOptions): Promise<RunningServer> {
  const binaryPath = path.resolve(__dirname, '../../.e2e/skillhub');
  if (!fs.existsSync(binaryPath)) {
    throw new Error(`skillhub binary not found at ${binaryPath}. Run make web-e2e to build.`);
  }

  const customWorkspace = options?.workspace;
  const ws = customWorkspace || fs.mkdtempSync(path.join(os.tmpdir(), 'skillhub-e2e-'));
  const envRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'skillhub-e2e-env-'));
  const env = options?.env ? { ...gitIdentity, ...options.env } : isolatedServerEnv(envRoot);

  if (!customWorkspace) {
    // 1. Initialize the hub in its own folder: init acts on the current folder.
    execFileSync(binaryPath, ['init'], { cwd: ws, env, stdio: 'pipe' });

    // 2. One draft skill to look at.
    createHubSkill(makeCLIRunner(ws, env), {
      id: 'smoke-skill',
      name: 'Smoke Skill',
      description: 'Smoke test skill.',
      body: '# Smoke Skill\n\nSmoke test instructions.\n',
    });
  }

  // 3. Spawn serve web
  const proc = spawn(
    binaryPath,
    ['serve', 'web', '--addr', '127.0.0.1:0', '--no-open', '--workspace', ws],
    { stdio: ['ignore', 'pipe', 'pipe'], env },
  );

  const serverInfo = await new Promise<{ url: string; origin: string }>((resolve, reject) => {
    let out = '';
    const timer = setTimeout(() => {
      proc.kill('SIGKILL');
      reject(new Error(`Timed out waiting for serve startup URL. Output:\n${out}`));
    }, 15000);

    proc.stdout.on('data', (chunk) => {
      out += chunk.toString();
      const m = out.match(/http:\/\/127\.0\.0\.1:(\d+)\/#token=([0-9a-fA-F]+)/);
      if (m && m[1]) {
        clearTimeout(timer);
        const port = m[1];
        resolve({
          url: m[0],
          origin: `http://127.0.0.1:${port}`,
        });
      }
    });

    proc.on('error', (err) => {
      clearTimeout(timer);
      reject(err);
    });

    proc.on('exit', (code) => {
      clearTimeout(timer);
      reject(new Error(`Process exited prematurely with code ${code}. Output:\n${out}`));
    });
  });

  const stop = async () => {
    if (!proc.killed) {
      proc.kill('SIGTERM');
      await new Promise<void>((resolve) => {
        proc.on('exit', () => resolve());
        setTimeout(() => {
          try {
            proc.kill('SIGKILL');
          } catch {
            // Process may already be dead.
          }
          resolve();
        }, 3000);
      });
    }
    if (!customWorkspace) {
      try {
        fs.rmSync(ws, { recursive: true, force: true });
      } catch {
        // Ignore cleanup error on temp dir.
      }
    }
    try {
      fs.rmSync(envRoot, { recursive: true, force: true });
    } catch {
      // Ignore cleanup error on temp dir.
    }
  };

  return {
    url: serverInfo.url,
    origin: serverInfo.origin,
    ws,
    stop,
  };
}
