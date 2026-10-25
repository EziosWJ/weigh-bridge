// Build-time helper only: the installed product does not need Node.js.
import { mkdir, readFile, copyFile, cp, writeFile, rm } from 'node:fs/promises'
import { resolve, join } from 'node:path'
import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'

const root = resolve(import.meta.dirname, '..')
const version = process.argv[2] || execFileSync('git', ['rev-parse', '--short=12', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim()
if (!/^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$/.test(version)) throw new Error('版本只能包含字母、数字、点、下划线和连字符')
const name = `weighbridge-${version}-linux-amd64`
const stage = join(root, '.task', name)
await rm(stage, { recursive: true, force: true })
await mkdir(join(stage, 'bin'), { recursive: true })
await mkdir(join(stage, 'configs'), { recursive: true })
const binaries = ['weighbridge-server', 'weighbridge-migrate', 'weighbridge-agent-printer', 'weighbridge-local-admin-init', 'weighbridge-agent-provision']
const components = {
  server: ['weighbridge-server', 'weighbridge-migrate', 'weighbridge-local-admin-init', 'weighbridge-agent-provision'],
  agent: ['weighbridge-agent-printer', 'weighbridge-agent-provision'],
}
const hashes = {}
for (const binary of binaries) {
  const source = join(root, 'bin', binary)
  const data = await readFile(source)
  // Reject an accidentally packaged host/Windows binary.
  if (data.subarray(0, 4).toString('hex') !== '7f454c46' || data[4] !== 2 || data.readUInt16LE(18) !== 62) throw new Error(`${binary} 不是 Linux amd64 ELF`)
  await copyFile(source, join(stage, 'bin', binary))
  hashes[binary] = createHash('sha256').update(data).digest('hex')
}
await cp(join(root, 'server/migrations'), join(stage, 'migrations'), { recursive: true })
await copyFile(join(root, 'server/configs/config.yaml'), join(stage, 'configs/config.yaml'))
await copyFile(join(root, 'agent/deploy/install-local.sh'), join(stage, 'install.sh'))
await mkdir(join(stage, 'deploy'), { recursive: true })
await copyFile(join(root, 'deploy/mediamtx.yml.example'), join(stage, 'deploy/mediamtx.yml.example'))
await cp(join(root, 'docs'), join(stage, 'docs'), { recursive: true })
await copyFile(join(root, 'CONTEXT.md'), join(stage, 'CONTEXT.md'))
await writeFile(join(stage, 'DEPLOYMENT.md'), '# 本厂部署\n\n默认同机安装；也可用 `install.sh --component server|agent` 分机安装。Agent-only 只填 `--server-url`，不需要数据库凭据。迁移、初始化、升级、备份和回退见 [离线部署说明](docs/device/local-deployment.md)。软件与现场验收状态见 [当前状态](docs/next.md)。\n')
await writeFile(join(stage, 'manifest.json'), JSON.stringify({ version, platform: 'linux-amd64', builtAt: new Date().toISOString(), binaries: hashes, components }, null, 2)+'\n')
await mkdir(join(root, 'bin'), { recursive: true })
const archive = join(root, 'bin', `${name}.tar.gz`)
execFileSync('tar', ['-czf', archive, '-C', join(root, '.task'), name])
console.log(archive)
