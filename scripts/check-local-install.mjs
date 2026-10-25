// Isolated install/upgrade smoke. Never starts services or touches a real DB.
import assert from 'node:assert/strict'
import { mkdtemp, cp, readFile, writeFile, stat, access, rm, mkdir, readdir } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { execFileSync } from 'node:child_process'

const source = process.argv[2]
if (!source) throw new Error('传入已解包的统一发布目录')
const temp = await mkdtemp(join(tmpdir(), 'weighbridge-install-check-'))
const packageDir = join(temp, 'package')
await cp(resolve(source), packageDir, { recursive: true })
const manifestPath = join(packageDir, 'manifest.json')
const baseManifest = JSON.parse(await readFile(manifestPath, 'utf8'))
let version = baseManifest.version
const databaseURL = 'postgres://isolated:isolated@127.0.0.1:1/not-connected'

function install(root, args = [], { database = true, env = {} } = {}) {
  const childEnv = { ...process.env, WEIGHBRIDGE_SYSTEMD_UNIT_DIR: `${root}-units`, ...env }
  if (database) childEnv.WEIGHBRIDGE_DATABASE_URL = databaseURL
  else delete childEnv.WEIGHBRIDGE_DATABASE_URL
  delete childEnv.WEIGHBRIDGE_DATABASE_USERNAME
  delete childEnv.WEIGHBRIDGE_DATABASE_PASSWORD
  delete childEnv.WEIGHBRIDGE_SITE_ID
  return execFileSync('bash', [join(packageDir, 'install.sh'), ...args, root], { env: childEnv, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] })
}

async function bumpVersion(suffix) {
  version = `${baseManifest.version}-${suffix}`
  const manifest = { ...baseManifest, version }
  await writeFile(manifestPath, JSON.stringify(manifest))
  return version
}

try {
  // ---- Same-host install (default) -------------------------------------------------
  const root = join(temp, 'installed-all')
  const unitDir = `${root}-units`
  const output = install(root, ['--skip-dependency-check', '--server-url', 'http://127.0.0.1:18900'])
  assert.match(output, /已安装组件：all/)
  const configPath = join(root, 'configs/config.prod.yaml')
  const config = JSON.parse(await readFile(configPath, 'utf8'))
  assert.equal('node' in config, false)
  assert.ok(config.jwt.secret.length >= 48)
  assert.equal((await stat(configPath)).mode & 0o777, 0o600)
  assert.equal(config.database.url, 'postgres://127.0.0.1:1/not-connected')
  assert.equal((await stat(join(root, 'data/agent-images'))).isDirectory(), true)
  assert.equal((await stat(join(root, 'data/uploads'))).isDirectory(), true)
  await assert.rejects(access(join(root, 'current')), 'installer must not activate a release')
  const envPath = join(root, 'agent/agent.env')
  const envContent = await readFile(envPath, 'utf8')
  assert.match(envContent, /WEIGHBRIDGE_SERVER_URL="http:\/\/127\.0\.0\.1:18900"/)
  assert.match(envContent, /WEIGHBRIDGE_PRINT_QUEUE="Aisino-SK820-raw"/)
  for (const deprecated of ['APP_WEIGHING__AGENT_KEY', 'WEIGHBRIDGE_AGENT_ID', 'WEIGHBRIDGE_SCALE_ID', 'WEIGHBRIDGE_CAMERA_']) {
    assert.ok(!envContent.includes(deprecated), `fresh agent.env contains ${deprecated}`)
  }
  assert.equal((await stat(envPath)).mode & 0o777, 0o600)
  const agentUnit = await readFile(join(unitDir, 'weighbridge-agent-printer.service'), 'utf8')
  assert.ok(agentUnit.includes(`--state-dir ${root}/agent/state`))
  assert.ok(!agentUnit.includes('weighbridge-agent-maintenance'))
  await access(join(unitDir, 'weighbridge-business.service'))
  const releaseBins = await readFile(join(root, 'releases', version, 'bin', 'weighbridge-server'))
  assert.ok(releaseBins.length > 0)

  // ---- Upgrade preserves configuration, ledger and customized units -----------------
  await writeFile(envPath, `# field settings
WEIGHBRIDGE_SERVER_URL="http://127.0.0.1:18900"
APP_WEIGHING__AGENT_KEY="legacy-secret"
WEIGHBRIDGE_AGENT_ID="agent-01"
WEIGHBRIDGE_SCALE_ID=1
WEIGHBRIDGE_PRINT_QUEUE="Site-Queue"
WEIGHBRIDGE_PRINT_WIDTH_MM=179
WEIGHBRIDGE_FORM_LENGTH_MM=93.13
WEIGHBRIDGE_SERIAL_PORT="/dev/ttyUSB9"
WEIGHBRIDGE_SERIAL_BAUD=19200
WEIGHBRIDGE_SERIAL_ENABLED=true
WEIGHBRIDGE_CAMERA_FRONT_URL="rtsp://operator:camera-secret@10.0.0.10/front"
WEIGHBRIDGE_CAMERA_REAR_URL="rtsp://10.0.0.10/rear"
CUSTOM_DEVICE_NOTE=preserve-me
`, { mode: 0o600 })
  const ledgerPath = join(root, 'agent/state/attempt-preserve.json')
  await writeFile(ledgerPath, '{"attempt":7,"status":"unknown"}\n')
  const originalConfig = await readFile(configPath, 'utf8')
  const customUnit = `[Service]\nExecStart=/opt/legacy/agent --server http://old --agent-id agent-01 --scale-id 1 --queue Site-Queue\nEnvironment=WEIGHBRIDGE_CAMERA_FRONT_URL=rtsp://legacy/front\n`
  await writeFile(join(unitDir, 'weighbridge-agent-printer.service'), customUnit)
  await mkdir(join(unitDir, 'weighbridge-agent-printer.service.d'), { recursive: true })
  await writeFile(join(unitDir, 'weighbridge-agent-printer.service.d', 'legacy.conf'), '[Service]\nEnvironment=APP_WEIGHING__AGENT_KEY=legacy\n')
  await bumpVersion('upgrade-check')
  const upgradeOutput = install(root, ['--skip-dependency-check'])
  assert.equal(await readFile(configPath, 'utf8'), originalConfig, 'upgrade overwrote local configuration')
  assert.equal(await readFile(ledgerPath, 'utf8'), '{"attempt":7,"status":"unknown"}\n')
  assert.equal(await readFile(join(unitDir, 'weighbridge-agent-printer.service'), 'utf8'), customUnit, 'unconfirmed unit was replaced')
  assert.match(upgradeOutput, /--update-units/)
  assert.match(upgradeOutput, /--update-env/)
  await access(join(root, 'releases', version, 'weighbridge-agent-printer.service.candidate'))
  const candidateEnvPath = join(root, 'agent/agent.env.upgraded')
  const candidateEnv = await readFile(candidateEnvPath, 'utf8')
  assert.ok(!candidateEnv.includes('APP_WEIGHING__AGENT_KEY'))
  assert.ok(!candidateEnv.includes('WEIGHBRIDGE_CAMERA_'))
  assert.match(candidateEnv, /WEIGHBRIDGE_PRINT_QUEUE="Site-Queue"/)
  assert.match(candidateEnv, /WEIGHBRIDGE_PRINT_WIDTH_MM=179/)
  assert.match(candidateEnv, /WEIGHBRIDGE_FORM_LENGTH_MM=93.13/)
  assert.match(candidateEnv, /WEIGHBRIDGE_SERIAL_PORT="\/dev\/ttyUSB9"/)
  assert.match(candidateEnv, /WEIGHBRIDGE_SERIAL_BAUD=19200/)
  assert.match(candidateEnv, /CUSTOM_DEVICE_NOTE=preserve-me/)
  const candidateCamera = JSON.parse(await readFile(`${candidateEnvPath}.legacy-camera.json`, 'utf8'))
  assert.ok(candidateCamera.entries.some(entry => entry.value === 'rtsp://operator:camera-secret@10.0.0.10/front'))

  // Controlled migration is explicit, makes a backup, and stops legacy drop-ins.
  await bumpVersion('migrate-check')
  const migrateOutput = install(root, ['--skip-dependency-check', '--update-units', '--update-env', '--server-url', 'http://127.0.0.1:18901'])
  const migratedEnv = await readFile(envPath, 'utf8')
  assert.ok(!migratedEnv.includes('APP_WEIGHING__AGENT_KEY'))
  assert.ok(!migratedEnv.includes('WEIGHBRIDGE_CAMERA_'))
  assert.match(migratedEnv, /WEIGHBRIDGE_SERVER_URL="http:\/\/127\.0\.0\.1:18901"/)
  assert.match(migratedEnv, /WEIGHBRIDGE_PRINT_QUEUE="Site-Queue"/)
  assert.match(migratedEnv, /WEIGHBRIDGE_SERIAL_PORT="\/dev\/ttyUSB9"/)
  assert.match(migratedEnv, /WEIGHBRIDGE_SERIAL_BAUD=19200/)
  assert.match(migratedEnv, /CUSTOM_DEVICE_NOTE=preserve-me/)
  assert.equal((await stat(envPath)).mode & 0o777, 0o600)
  const backupNames = await readdir(join(root, 'agent/backups'))
  assert.equal(backupNames.length, 1)
  const backup = await readFile(join(root, 'agent/backups', backupNames[0]), 'utf8')
  assert.match(backup, /legacy-secret/)
  await access(join(root, 'agent/agent.env.legacy-camera.json'))
  const activeUnit = await readFile(join(unitDir, 'weighbridge-agent-printer.service'), 'utf8')
  assert.ok(activeUnit.includes('--env-file'))
  assert.ok(!activeUnit.includes('--agent-id'))
  await assert.rejects(access(join(unitDir, 'weighbridge-agent-printer.service.d', 'legacy.conf')), 'legacy drop-in must be disabled')
  assert.match(migrateOutput, /已停用并备份旧 drop-in/)
  assert.equal(await readFile(ledgerPath, 'utf8'), '{"attempt":7,"status":"unknown"}\n')

  // ---- Server-only ------------------------------------------------------------------
  const serverRoot = join(temp, 'installed-server')
  const serverOutput = install(serverRoot, ['--component', 'server', '--skip-dependency-check'])
  assert.match(serverOutput, /已安装组件：server/)
  await access(join(serverRoot, 'configs/config.prod.yaml'))
  await access(join(`${serverRoot}-units`, 'weighbridge-business.service'))
  await assert.rejects(access(join(`${serverRoot}-units`, 'weighbridge-agent-printer.service')))
  await assert.rejects(access(join(serverRoot, 'agent/agent.env')))
  await access(join(serverRoot, 'releases', version, 'bin/weighbridge-server'))
  await assert.rejects(access(join(serverRoot, 'releases', version, 'bin/weighbridge-agent-printer')))
  await access(join(serverRoot, 'releases', version, 'migrations'))

  // ---- Agent-only: no database credentials and no server configuration --------------
  const agentRoot = join(temp, 'installed-agent')
  const agentOutput = install(agentRoot, ['--component', 'agent', '--skip-dependency-check', '--server-url', 'http://10.0.0.5:18900'], { database: false })
  assert.match(agentOutput, /已安装组件：agent/)
  assert.ok((await readFile(join(agentRoot, 'agent/agent.env'), 'utf8')).includes('http://10.0.0.5:18900'))
  assert.equal((await stat(join(agentRoot, 'agent/agent.env'))).mode & 0o777, 0o600)
  await access(join(`${agentRoot}-units`, 'weighbridge-agent-printer.service'))
  await assert.rejects(access(join(`${agentRoot}-units`, 'weighbridge-business.service')))
  await assert.rejects(access(join(agentRoot, 'configs/config.prod.yaml')))
  await assert.rejects(access(join(agentRoot, 'releases', version, 'migrations')))
  await access(join(agentRoot, 'releases', version, 'bin/weighbridge-agent-printer'))
  await access(join(agentRoot, 'releases', version, 'bin/weighbridge-agent-provision'))
  assert.throws(() => install(join(temp, 'agent-no-url'), ['--component', 'agent', '--skip-dependency-check'], { database: false }), /--server-url|Command failed/)

  // ---- Damaged package is rejected without staging a release -----------------------
  await bumpVersion('corrupt')
  const corruptManifest = { ...baseManifest, version, binaries: { ...baseManifest.binaries, 'weighbridge-server': '0'.repeat(64) } }
  await writeFile(manifestPath, JSON.stringify(corruptManifest))
  const corruptRoot = join(temp, 'installed-corrupt')
  assert.throws(() => install(corruptRoot, ['--skip-dependency-check'], { database: false }), /二进制校验失败|Command failed/)
  await assert.rejects(access(join(corruptRoot, 'releases', version)))

  console.log('PASS: 同机/Server-only/Agent-only 安装、Agent-only 无数据库、旧 agent.env 清理与摄像头备份、unit/drop-in 核对与受控迁移、私有配置与账本保留、损坏包拒绝；未启动服务或连接数据库。')
} finally {
  await rm(temp, { recursive: true, force: true })
}
