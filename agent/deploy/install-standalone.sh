#!/usr/bin/env bash
set -euo pipefail
umask 077

if [[ ${1:-} == --help ]]; then
  cat <<'HELP'
用法：bash install.sh <安装目录绝对路径>
使用本厂专用非 root 用户。首次安装先设置 WEIGHBRIDGE_DATABASE_URL、
WEIGHBRIDGE_SITE_ID（1..255，须两厂预先分配不同值）。
脚本只安装发布文件、生成首次配置和用户 unit，不迁移、不启用、不启动服务。
配置、数据库、图片和 Agent 账本保留。后续步骤见 DEPLOYMENT.md。
HELP
  exit 0
fi
if [[ $(id -u) == 0 ]]; then echo '请用本厂专用非 root 用户安装。' >&2; exit 1; fi
install_root=${1:?请提供安装目录绝对路径，详见 --help}
# Restrict generated systemd paths to avoid interpreting specifiers/whitespace.
if [[ ! $install_root =~ ^/[A-Za-z0-9_./-]+$ || $install_root == / || $install_root == *'/../'* || $install_root == */.. ]]; then
  echo '安装路径须为绝对路径且仅含字母、数字、下划线、点、连字符或斜线。' >&2; exit 1
fi
package_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
command -v python3 >/dev/null
command -v flock >/dev/null
python3 - "$package_dir" "$install_root" <<'PY'
import hashlib, json, os, pathlib, secrets, shutil, sys
from urllib.parse import urlsplit, urlunsplit, unquote
source, root = map(pathlib.Path, sys.argv[1:])
manifest = json.loads((source/'manifest.json').read_text())
version = manifest['version']
if not version or any(c not in 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-' for c in version):
    raise SystemExit('无效发布版本')
for binary, checksum in manifest['binaries'].items():
    if pathlib.Path(binary).name != binary or hashlib.sha256((source/'bin'/binary).read_bytes()).hexdigest() != checksum:
        raise SystemExit('二进制校验失败')
config_path = root/'configs/config.prod.yaml'
if not config_path.exists():
    database = os.environ.get('WEIGHBRIDGE_DATABASE_URL', '')
    try: site_id = int(os.environ.get('WEIGHBRIDGE_SITE_ID', '0'))
    except ValueError: site_id = 0
    if not database.startswith(('postgres://','postgresql://')) or not 1 <= site_id <= 255:
        raise SystemExit('首次安装必须提供本地 PostgreSQL URL和已确认的 SITE_ID（1..255）')
    parsed_database = urlsplit(database)
    database_username = os.environ.get('WEIGHBRIDGE_DATABASE_USERNAME', unquote(parsed_database.username or ''))
    database_password = os.environ.get('WEIGHBRIDGE_DATABASE_PASSWORD', unquote(parsed_database.password or ''))
    if not database_username or not database_password:
        raise SystemExit('数据库 URL 须含账号密码，或设置 WEIGHBRIDGE_DATABASE_USERNAME/PASSWORD')
    database = urlunsplit(parsed_database._replace(netloc=parsed_database.netloc.rsplit('@',1)[-1]))
for part in ('releases', 'configs', 'data/agent-images', 'data/uploads', 'agent/state'):
    (root/part).mkdir(parents=True, exist_ok=True, mode=0o700)
release = root/'releases'/version
if release.exists():
    raise SystemExit('此版本已安装；为每次构建分配唯一版本以保留回退文件')
release.mkdir(mode=0o700)
for part in ('bin', 'migrations'):
    shutil.copytree(source/part, release/part)
os.symlink(root/'configs', release/'configs')
shutil.copy2(source/'manifest.json', release/'manifest.json')
for binary in (release/'bin').iterdir(): binary.chmod(0o700)
if not (root/'configs/config.yaml').exists(): shutil.copy2(source/'configs/config.yaml', root/'configs/config.yaml')
if not config_path.exists():
    cfg = {
      'http': {'address':'0.0.0.0:18900'},
      'database': {'driver':'postgres','url':database,'username':database_username,'password':database_password},
      'jwt': {'secret':secrets.token_urlsafe(48)},
      'node': {'role':'standalone','site_id':site_id},
      'weighing': {'image_dir':str(root/'data/agent-images')},
      'file': {'storage_root':str(root/'data/uploads')},
      'maintenance': {'enabled':True},
    }
    # JSON is a YAML subset; never interpolate credentials through the shell.
    with config_path.open('x') as f: json.dump(cfg,f,ensure_ascii=False,indent=2)
    config_path.chmod(0o600)
# Stage only; the operator switches current after backup, migration and preflight.
print('已安装版本：'+str(release))
print('完成 DEPLOYMENT.md 的备份、迁移及初始化后，再切换 current 并启动。')
unit_dir = pathlib.Path(os.environ.get('WEIGHBRIDGE_SYSTEMD_UNIT_DIR', str(pathlib.Path.home()/'.config/systemd/user')))
unit_dir.mkdir(parents=True,exist_ok=True)
units = {
 'weighbridge-business.service': f'''[Unit]
Description=Weighbridge local business and embedded Web
After=network.target
[Service]
Type=simple
WorkingDirectory={root}
Environment=APP_ENV=prod
ExecStart={root}/current/bin/weighbridge-server
Restart=on-failure
RestartSec=3
TimeoutStopSec=20
UMask=0077
NoNewPrivileges=true
[Install]
WantedBy=default.target
''',
 'weighbridge-agent-printer.service': f'''[Unit]
Description=Weighbridge local scale camera and printer
After=network-online.target
[Service]
Type=simple
WorkingDirectory={root}/agent
ExecStart=/usr/bin/flock -n {root}/agent/agent.lock {root}/current/bin/weighbridge-agent-printer --env-file {root}/agent/agent.env --state-dir {root}/agent/state
Restart=on-failure
RestartSec=3
TimeoutStopSec=10
UMask=0077
NoNewPrivileges=true
[Install]
WantedBy=default.target
'''
}
for name, text in units.items():
    # Do not replace an existing unit/drop-in during a package upgrade.
    target = unit_dir/name
    if target.exists():
        (release/(name+'.candidate')).write_text(text)
        print('保留已有 unit；请核对候选文件：'+str(release/(name+'.candidate')))
    else: target.write_text(text)
PY
