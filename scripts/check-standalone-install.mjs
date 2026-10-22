// Isolated install/upgrade smoke. Never starts services or touches a real DB.
import assert from 'node:assert/strict'
import { mkdtemp, cp, readFile, writeFile, stat, access, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { execFileSync } from 'node:child_process'

const source = process.argv[2]
if (!source) throw new Error('传入已解包的统一发布目录')
const temp = await mkdtemp(join(tmpdir(), 'weighbridge-install-check-'))
try {
  const packageDir = join(temp, 'package')
  const installRoot = join(temp, 'installed')
  const unitDir = join(temp, 'units')
  await cp(resolve(source), packageDir, { recursive: true })
  const manifest = JSON.parse(await readFile(join(packageDir, 'manifest.json'), 'utf8'))
  const env = {
    ...process.env,
    WEIGHBRIDGE_DATABASE_URL: 'postgres://isolated:isolated@127.0.0.1:1/not-connected',
    WEIGHBRIDGE_SITE_ID: '2',
    WEIGHBRIDGE_SYSTEMD_UNIT_DIR: unitDir,
  }
  const install = () => execFileSync('bash', [join(packageDir, 'install.sh'), installRoot], { env, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] })
  install()
  const configPath = join(installRoot, 'configs/config.prod.yaml')
  const original = await readFile(configPath, 'utf8')
  const config = JSON.parse(original)
  assert.equal(config.node.role, 'standalone')
  assert.equal(config.node.site_id, 2)
  assert.equal(config.maintenance.data_dir, join(installRoot, 'agent'))
  assert.ok(config.jwt.secret.length >= 48)
  assert.equal((await stat(configPath)).mode & 0o777, 0o600)
  await assert.rejects(access(join(installRoot, 'current')), 'installer must not activate a release')
  const envPath = join(installRoot, 'agent/agent.env')
  const ledgerPath = join(installRoot, 'agent/state/attempt-preserve.json')
  await writeFile(envPath, 'private identity and camera credentials\n', { mode: 0o600 })
  await writeFile(ledgerPath, '{"attempt":7,"status":"unknown"}\n')
  const unit = await readFile(join(unitDir, 'weighbridge-agent-printer.service'), 'utf8')
  assert.ok(unit.includes(`--state-dir ${installRoot}/agent/state`))
  const secondVersion = `${manifest.version}-upgrade-check`
  manifest.version = secondVersion
  await writeFile(join(packageDir, 'manifest.json'), JSON.stringify(manifest))
  install()
  assert.equal(await readFile(configPath, 'utf8'), original, 'upgrade overwrote local configuration')
  assert.equal(await readFile(envPath, 'utf8'), 'private identity and camera credentials\n')
  assert.equal(await readFile(ledgerPath, 'utf8'), '{"attempt":7,"status":"unknown"}\n')
  assert.equal(await readFile(join(unitDir, 'weighbridge-agent-printer.service'), 'utf8'), unit)
  await access(join(installRoot, 'releases', secondVersion, 'weighbridge-agent-printer.service.candidate'))
  manifest.version += '-corrupt'
  manifest.binaries['weighbridge-server'] = '0'.repeat(64)
  await writeFile(join(packageDir, 'manifest.json'), JSON.stringify(manifest))
  assert.throws(install, /二进制校验失败|Command failed/)
  await assert.rejects(access(join(installRoot, 'releases', manifest.version)))
  console.log('PASS: 首次安装、非自动激活、私有配置、升级保留身份/账本/unit、损坏包拒绝；未启动服务或连接数据库。')
} finally {
  await rm(temp, { recursive: true, force: true })
}
