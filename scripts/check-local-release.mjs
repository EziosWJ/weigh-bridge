// Real packaged API + isolated PostgreSQL restart smoke. No physical devices or
// systemd units are started, and no deployment credentials are read.
import assert from 'node:assert/strict'
import { mkdtemp, readFile, writeFile, symlink, rm, stat } from 'node:fs/promises'
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
  delete env.WEIGHBRIDGE_SITE_ID
  await execute('bash', [join(packageDir, 'install.sh'), '--skip-dependency-check', root], { env: {
    ...env, WEIGHBRIDGE_DATABASE_URL: databaseURL.href,
    WEIGHBRIDGE_SYSTEMD_UNIT_DIR: join(temp, 'units'),
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
  await assert.rejects(execute(join(release, 'bin/weighbridge-server'), [], { cwd: root, env, timeout: 5000 }), error => /local-admin-init|本地管理员口令/.test(`${error.message} ${error.stderr || ''}`))
  const adminEnv = { ...env, WEIGHBRIDGE_ADMIN_PASSWORD: 'isolated-release-admin-password' }
  await execute(join(release, 'bin/weighbridge-local-admin-init'), ['--config-dir', join(root, 'configs')], { cwd: release, env: adminEnv, timeout: 30000 })
  await assert.rejects(execute(join(release, 'bin/weighbridge-local-admin-init'), ['--config-dir', join(root, 'configs')], { cwd: release, env: adminEnv, timeout: 30000 }), error => /already initialized|refusing to overwrite/.test(`${error.message} ${error.stderr || ''}`))
  await symlink(join('releases', manifest.version), join(root, 'current'))
  let agentIdentity
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
    const local = await fetch(baseURL+'/api/weighing/local-status', { headers: { Authorization: token }, signal: AbortSignal.timeout(3000) })
    assert.equal(local.status, 200)
    assert.deepEqual((await local.json()).data, { siteId: 1, configured: true, initialized: true }, 'API startup prepares the internal identity without a setup form')
    if (cycle === 0) {
      const agentEnvPath = join(root, 'agent/agent.env')
      // The installer creates a keyless same-host default. Remove it so this
      // cycle verifies the provisioning contract from a blank Agent host.
      await rm(agentEnvPath)
      await assert.rejects(execute(join(release, 'bin/weighbridge-agent-provision'), ['--config-dir', join(root, 'configs'), '--agent-env-path', agentEnvPath], { cwd: release, env, timeout: 30000 }), error => /--server-url/.test(`${error.message} ${error.stderr || ''}`), '首次配对必须显式提供连接地址')
      const provision = await execute(join(release, 'bin/weighbridge-agent-provision'), ['--config-dir', join(root, 'configs'), '--agent-env-path', agentEnvPath, '--server-url', baseURL], { cwd: release, env, timeout: 30000 })
      assert.ok(!provision.stdout.includes('agentKey'))
      let agentEnv = await readFile(agentEnvPath, 'utf8')
      for (const deprecated of ['APP_WEIGHING__AGENT_KEY', 'WEIGHBRIDGE_AGENT_ID', 'WEIGHBRIDGE_SCALE_ID', 'WEIGHBRIDGE_SITE_ID', 'WEIGHBRIDGE_CAMERA_']) {
        assert.ok(!agentEnv.includes(deprecated), `provisioned agent.env contains ${deprecated}`)
      }
      assert.match(agentEnv, new RegExp(`WEIGHBRIDGE_SERVER_URL="${baseURL.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}"`))
      agentEnv = agentEnv.replace('WEIGHBRIDGE_SERIAL_PORT=""', 'WEIGHBRIDGE_SERIAL_PORT="/dev/ttyUSB9"') + 'WEIGHBRIDGE_PRINT_WIDTH_MM=179\nAPP_WEIGHING__AGENT_KEY="legacy-key"\nWEIGHBRIDGE_CAMERA_FRONT_URL="http://camera.invalid/front"\nWEIGHBRIDGE_CAMERA_REAR_URL="http://camera.invalid/rear"\nCUSTOM_DEVICE_NOTE=preserve-me\n'
      await writeFile(agentEnvPath, agentEnv, { mode: 0o600 })
      await assert.rejects(execute(join(release, 'bin/weighbridge-agent-provision'), ['--config-dir', join(root, 'configs'), '--agent-env-path', agentEnvPath], { cwd: release, env, timeout: 30000 }), error => /explicitly pass|--update-connection/.test(`${error.message} ${error.stderr || ''}`))
      const updated = await execute(join(release, 'bin/weighbridge-agent-provision'), ['--config-dir', join(root, 'configs'), '--agent-env-path', agentEnvPath, '--update-connection'], { cwd: release, env, timeout: 30000 })
      assert.ok(!updated.stdout.includes('agentKey'))
      const preserved = await readFile(agentEnvPath, 'utf8')
      assert.match(preserved, /WEIGHBRIDGE_SERIAL_PORT="\/dev\/ttyUSB9"/)
      assert.match(preserved, /WEIGHBRIDGE_PRINT_WIDTH_MM=179/)
      assert.match(preserved, /CUSTOM_DEVICE_NOTE=preserve-me/)
      assert.ok(!preserved.includes('APP_WEIGHING__AGENT_KEY'))
      assert.ok(!preserved.includes('WEIGHBRIDGE_CAMERA_'))
      assert.ok(!preserved.includes('WEIGHBRIDGE_AGENT_ID'))
      const cameraBackup = JSON.parse(await readFile(agentEnvPath + '.legacy-camera.json', 'utf8'))
      assert.ok(cameraBackup.entries.some(entry => entry.value === 'http://camera.invalid/front'))
      assert.equal((await stat(agentEnvPath)).mode & 0o777, 0o600)
      const replaced = await execute(join(release, 'bin/weighbridge-agent-provision'), ['--config-dir', join(root, 'configs'), '--agent-env-path', agentEnvPath, '--update-connection', '--server-url', 'http://127.0.0.1:18901'], { cwd: release, env, timeout: 30000 })
      assert.ok(!replaced.stdout.includes('agentKey'))
      assert.match(await readFile(agentEnvPath, 'utf8'), /WEIGHBRIDGE_SERVER_URL="http:\/\/127\.0\.0\.1:18901"/)
      assert.match(await readFile(agentEnvPath, 'utf8'), /WEIGHBRIDGE_SERIAL_PORT="\/dev\/ttyUSB9"/)
      agentIdentity = await readFile(agentEnvPath, 'utf8')
    }
    await stop()
    if (agentIdentity !== undefined) assert.equal(await readFile(join(root, 'agent/agent.env'), 'utf8'), agentIdentity)
    assert.equal(await readFile(join(root, 'agent/state/unknown-attempt.json'), 'utf8'), '{"status":"unknown"}\n')
    assert.equal(await readFile(join(root, 'data/agent-images/preserved-evidence.txt'), 'utf8'), 'preserved image evidence\n')
  }
  console.log('PASS: 本地管理员一次性初始化、启动自动建立本厂身份、Agent 配对与受控保留设备配置、真实 API 内嵌页面、health/ready、重启保留现场身份/证据/未知账本；未启动真实设备。')
} finally {
  await stop()
  await database?.close()
  await rm(temp, { recursive: true, force: true })
}
