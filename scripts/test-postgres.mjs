// Isolated database for browser acceptance tests. Never reads deployment credentials.
import { execFile } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { promisify } from 'node:util';
import { setTimeout as delay } from 'node:timers/promises';

const execute = promisify(execFile);
const docker = (...args) => execute('docker', args, { timeout: 60_000 });

export async function startTestPostgres() {
  const name = `weighbridge-browser-${randomUUID()}`;
  const password = randomUUID();
  const close = async () => { await docker('rm', '--force', name); };
  try {
    await docker('run', '--detach', '--rm', '--name', name,
      '--env', 'POSTGRES_DB=integration', '--env', 'POSTGRES_USER=integration',
      '--env', `POSTGRES_PASSWORD=${password}`, '--publish', '127.0.0.1::5432', 'postgres:17-alpine');
    const { stdout } = await docker('port', name, '5432/tcp');
    const port = stdout.trim().split(':').at(-1);
    let ready = false;
    // Wait for the final TCP server, after image initialization has finished.
    for (let attempt = 0; attempt < 45; attempt++) {
      try {
        await docker('exec', '--env', `PGPASSWORD=${password}`, name,
          'psql', '-h', '127.0.0.1', '-U', 'integration', '-d', 'integration', '-tAc', 'SELECT 1');
        ready = true;
        break;
      } catch { await delay(1000); }
    }
    if (!ready) throw new Error('Temporary PostgreSQL did not become ready within 45 attempts');
    return {
      env: {
        APP_CONFIG_PROFILE: '', APP_DATABASE__DRIVER: 'postgres',
        APP_DATABASE__URL: `postgres://127.0.0.1:${port}/integration?sslmode=disable`,
        APP_DATABASE__USERNAME: 'integration', APP_DATABASE__PASSWORD: password,
      },
      close,
    };
  } catch (error) {
    await close().catch(() => {});
    throw error;
  }
}
