// Real packaged API + isolated PostgreSQL restart smoke. No physical devices or
// systemd units are started, and no deployment credentials are read.
import assert from 'node:assert/strict'
import { mkdtemp, readFile, writeFile, symlink, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { execFile, spawn } from 'node:child_process'
import { promisify } from 'node:util'
import { createServer } from 'node:net'
import { setTimeout as delay } from 'node:timers/promises'
import { startTestPostgres } from './test-postgres.mjs'

const execute = promisify(execFile)
const source = process.argv[2]
if (!source) throw new Error('传入已解包的统一发布目录')
const temp = await mkdtemp(join(tmpdir(), 'weighbridge-release-check-'))
const packageDir = resolve(source)
const root = join(temp, 'installed')
const baseEnv = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('APP_')))
let database, child, exited, output = ''
async function stop() {
  if (!child) return
  child.kill('SIGTERM')
  try {
    await Promise.race([exited, delay(15000, undefined, { ref: false }).then(() => { throw new Error('API 未按期退出') })])
  } finally {
    if (child.exitCode === null && child.signalCode === null) { child.kill('SIGKILL'); await exited }
    child = undefined
  }
}
try {
  database = await startTestPostgres()
  const databaseURL = new URL(database.env.APP_DATABASE__URL)
  databaseURL.username = database.env.APP_DATABASE__USERNAME
  databaseURL.password = database.env.APP_DATABASE__PASSWORD
  const env = { ...baseEnv, APP_ENV: 'prod', GIN_MODE: 'release' }
  await execute('bash', [join(packageDir, 'install.sh'), root], { env: {
    ...env, WEIGHBRIDGE_DATABASE_URL: databaseURL.href,
    WEIGHBRIDGE_SITE_ID: '2', WEIGHBRIDGE_SYSTEMD_UNIT_DIR: join(temp, 'units'),
  }, timeout: 30000 })
  const manifest = JSON.parse(await readFile(join(packageDir, 'manifest.json'), 'utf8'))
  const release = join(root, 'releases', manifest.version)
  const configPath = join(root, 'configs/config.prod.yaml')
  const config = JSON.parse(await readFile(configPath, 'utf8'))
  const listener = createServer()
  await new Promise((resolve, reject) => { listener.once('error', reject); listener.listen(0, '127.0.0.1', resolve) })
  const port = listener.address().port
  await new Promise(resolve => listener.close(resolve))
  config.http.address = `127.0.0.1:${port}`
  await writeFile(configPath, JSON.stringify(config), { mode: 0o600 })
  await execute(join(release, 'bin/weighbridge-migrate'), ['up', '--kind', 'all'], { cwd: release, env, timeout: 60000 })
  await execute(join(release, 'bin/weighbridge-standalone-init'), [
    '--config-dir', join(root, 'configs'), '--site-code', 'RELEASE02', '--site-name', '发布验证二厂',
    '--scale-code', 'SCALE02', '--scale-name', '发布验证地磅', '--agent-id', 'release-agent-2',
    '--agent-env-path', join(root, 'agent/agent.env'),
  ], { cwd: release, env: { ...env, WEIGHBRIDGE_ADMIN_PASSWORD: 'isolated-release-admin-password' }, timeout: 30000 })
  await symlink(join('releases', manifest.version), join(root, 'current'))
  const agentIdentity = await readFile(join(root, 'agent/agent.env'), 'utf8')
  await writeFile(join(root, 'agent/state/unknown-attempt.json'), '{"status":"unknown"}\n')
  await writeFile(join(root, 'data/agent-images/preserved-evidence.txt'), 'preserved image evidence\n')
  const baseURL = `http://127.0.0.1:${port}`
  const get = path => fetch(baseURL + path, { signal: AbortSignal.timeout(3000) })
  for (let cycle = 0; cycle < 2; cycle++) {
    output = ''
    child = spawn(join(root, 'current/bin/weighbridge-server'), [], { cwd: root, env, stdio: ['ignore', 'pipe', 'pipe'] })
    child.stdout.on('data', data => { output = (output + data).slice(-16000) })
    child.stderr.on('data', data => { output = (output + data).slice(-16000) })
    exited = new Promise(resolve => { child.once('exit', resolve); child.once('error', resolve) })
    let ready = false
    for (let attempt = 0; attempt < 60; attempt++) {
      if (child.exitCode !== null) throw new Error(`API 启动失败：${output}`)
      try { if ((await get('/ready')).ok) { ready = true; break } } catch { /* bounded startup polling */ }
      await delay(200)
    }
    assert.ok(ready, `API 就绪失败：${output}`)
    assert.ok((await get('/health')).ok)
    const page = await get('/system/agent-maintenance')
    assert.ok(page.headers.get('content-type')?.includes('text/html'))
    assert.ok((await page.text()).includes('id="root"'), 'packaged API did not serve the embedded SPA')
    const response = await fetch(baseURL+'/api/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username: 'admin', password: 'isolated-release-admin-password' }), signal: AbortSignal.timeout(3000) })
    assert.equal(response.status, 200)
    const login = await response.json()
    const token = login.data.tokenValue
    assert.ok(token, 'missing local login token')
    const status = await fetch(baseURL+'/api/node/status', { headers: { Authorization: token }, signal: AbortSignal.timeout(3000) })
    const node = await status.json()
    assert.equal(node.data.role, 'standalone')
    assert.equal(node.data.siteId, 2)
    assert.equal(node.data.initialized, false)
    assert.deepEqual(node.data.sites, [])
    await stop()
    assert.equal(await readFile(join(root, 'agent/agent.env'), 'utf8'), agentIdentity)
    assert.equal(await readFile(join(root, 'agent/state/unknown-attempt.json'), 'utf8'), '{"status":"unknown"}\n')
    assert.equal(await readFile(join(root, 'data/agent-images/preserved-evidence.txt'), 'utf8'), 'preserved image evidence\n')
  }
  console.log('PASS: 统一包显式迁移/自治初始化、真实 API 内嵌页面、本地重登、health/ready、重启保留现场身份/证据/未知账本；未启动真实设备或中心。')
} finally {
  await stop()
  await database?.close()
  await rm(temp, { recursive: true, force: true })
}
