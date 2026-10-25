#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
  cat <<'HELP'
用法：bash install.sh [选项] <安装目录绝对路径>

按组件安装本厂运行文件，默认同机安装 Server 与 Agent：
  --component all|server|agent   安装组件，默认 all
  --server-url URL               Agent 主机可访问的业务服务地址；all 缺省 http://127.0.0.1:18900，
                                 agent-only 必须提供
  --print-queue NAME             Agent 默认 CUPS 队列，默认 Aisino-SK820-raw
  --serial-port PATH             Agent 仪表串口路径，默认留空待网页配置
  --serial-baud N                Agent 仪表波特率，默认 9600
  --update-units                 核对后显式替换有差异的 systemd unit，并备份旧 unit/drop-in
  --update-env                   升级时显式应用清理后的 agent.env（原文件先备份）
  --skip-dependency-check        跳过 CUPS/串口/FFmpeg 运行时检查（仅用于隔离测试）
  --help                         显示本说明

Server 组件首次安装需要 WEIGHBRIDGE_DATABASE_URL，账号密码可在 URL 中，
或单独设置 WEIGHBRIDGE_DATABASE_USERNAME/PASSWORD。Agent-only 只读取
--server-url，不读取、拷贝或要求数据库凭据。

脚本只安装发布文件、生成首次运行配置和用户 unit，不迁移、不启用、不启动服务。
已有配置、业务库、图片、Agent 账本和自定义 unit 不被静默覆盖。
HELP
}

component=all
server_url=
print_queue=Aisino-SK820-raw
serial_port=
serial_baud=9600
update_units=0
update_env=0
skip_dependency_check=0
server_url_explicit=0
print_queue_explicit=0
serial_port_explicit=0
serial_baud_explicit=0
install_root=

while (($#)); do
  case "$1" in
    --help|-h) usage; exit 0 ;;
    --component) component=${2:?--component 需要参数}; shift 2 ;;
    --component=*) component=${1#*=}; shift ;;
    --server-url) server_url=${2:?--server-url 需要参数}; server_url_explicit=1; shift 2 ;;
    --server-url=*) server_url=${1#*=}; server_url_explicit=1; shift ;;
    --print-queue) print_queue=${2:?--print-queue 需要参数}; print_queue_explicit=1; shift 2 ;;
    --print-queue=*) print_queue=${1#*=}; print_queue_explicit=1; shift ;;
    --serial-port) serial_port=${2-}; serial_port_explicit=1; shift 2 ;;
    --serial-port=*) serial_port=${1#*=}; serial_port_explicit=1; shift ;;
    --serial-baud) serial_baud=${2:?--serial-baud 需要参数}; serial_baud_explicit=1; shift 2 ;;
    --serial-baud=*) serial_baud=${1#*=}; serial_baud_explicit=1; shift ;;
    --update-units) update_units=1; shift ;;
    --update-env) update_env=1; shift ;;
    --skip-dependency-check) skip_dependency_check=1; shift ;;
    --) shift; install_root=${1-}; shift || true ;;
    -*) echo "未知选项：$1（使用 --help 查看用法）" >&2; exit 2 ;;
    *)
      if [[ -n $install_root ]]; then echo '只能提供一个安装目录。' >&2; exit 2; fi
      install_root=$1; shift ;;
  esac
done

case "$component" in all|server|agent) ;; *) echo '--component 只能是 all、server 或 agent。' >&2; exit 2 ;; esac
if [[ ! $serial_baud =~ ^[0-9]+$ ]] || (( serial_baud < 1200 || serial_baud > 115200 )); then
  echo '--serial-baud 须为 1200～115200 的整数。' >&2; exit 2
fi
if [[ $component == agent && -z ${server_url} ]]; then
  echo 'Agent-only 安装必须通过 --server-url 提供 Agent 主机可访问的业务服务地址。' >&2; exit 2
fi
if [[ $component == all && -z ${server_url} ]]; then server_url=http://127.0.0.1:18900; fi
if [[ -n ${server_url} ]]; then
  python3 - "$server_url" <<'PYURL' || { echo '--server-url 须为 http(s) 地址，且不含账号、片段或换行。' >&2; exit 2; }
import sys
from urllib.parse import urlsplit
u = urlsplit(sys.argv[1])
if u.scheme not in ('http', 'https') or not u.hostname or u.username or u.password or u.fragment or any(c in sys.argv[1] for c in '\r\n\0'):
    raise SystemExit(1)
PYURL
fi
if [[ -n $print_queue && ! $print_queue =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$ ]]; then
  echo '--print-queue 只能包含字母、数字、点、下划线和连字符，且以字母或数字开头。' >&2; exit 2
fi
if [[ -n $serial_port && ( ! $serial_port =~ ^/dev/[A-Za-z0-9_./-]+$ || $serial_port == *..* || $serial_port == */.. ) ]]; then
  echo '--serial-port 须为 /dev 下的设备路径，且不能包含 ..。' >&2; exit 2
fi
if [[ $(id -u) == 0 ]]; then echo '请用本厂专用非 root 用户安装。' >&2; exit 1; fi
install_root=${install_root:?请提供安装目录绝对路径，详见 --help}
# Restrict generated systemd paths to avoid interpreting specifiers/whitespace.
if [[ ! $install_root =~ ^/[A-Za-z0-9_./-]+$ || $install_root == / || $install_root == *'/../'* || $install_root == */.. ]]; then
  echo '安装路径须为绝对路径且仅含字母、数字、下划线、点、连字符或斜线。' >&2; exit 1
fi
package_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
command -v python3 >/dev/null
command -v flock >/dev/null

check_dependencies() {
  if (( skip_dependency_check )); then
    echo '已跳过运行时依赖检查（隔离测试）。' >&2
    return
  fi
  if [[ $component == server || $component == all ]]; then
    if command -v ffmpeg >/dev/null; then
      echo "FFmpeg：$(command -v ffmpeg)" >&2
    else
      echo '警告：Server 主机 PATH 中未找到 ffmpeg；RTSP 摄像头抓拍将不可用，HTTP 抓拍不受影响。' >&2
    fi
  fi
  if [[ $component == agent || $component == all ]]; then
    local missing=()
    for tool in lp lpstat cancel python3; do command -v "$tool" >/dev/null || missing+=("$tool"); done
    if (( ${#missing[@]} )); then
      echo "Agent 打印依赖缺失：${missing[*]}。请先安装 CUPS 客户端工具与 Python 3，或使用 --skip-dependency-check 仅检查文件布局。" >&2
      exit 1
    fi
    if ! python3 -c 'import PIL' >/dev/null 2>&1; then
      echo 'Agent 打印依赖缺失：Python Pillow。请安装 Pillow 后重试。' >&2
      exit 1
    fi
    if [[ ! -r /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc ]]; then
      echo '提示：未找到 /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc；打印渲染需要可用的中文字体。' >&2
    fi
    if [[ -n $print_queue ]]; then
      if ! lpstat -a 2>/dev/null | awk '{print $1}' | grep -Fxq -- "$print_queue"; then
        echo "CUPS 队列 ${print_queue} 不存在。请先在 Agent 主机创建并测试队列，或用 --print-queue 指定实际队列。" >&2
        exit 1
      fi
    fi
    if [[ -n $serial_port ]]; then
      if [[ -e $serial_port ]]; then
        if [[ ! -r $serial_port || ! -w $serial_port ]]; then
          echo "串口 ${serial_port} 对当前安装账号（UID $(id -u)）不可读写。请将运行账号加入实际串口组（常见为 dialout 或 uucp），重新登录会话后重试。" >&2
          exit 1
        fi
        if ! id -nG | tr ' ' '\n' | grep -Eq '^(dialout|uucp)$'; then
          echo "提示：当前账号不在 dialout/uucp 组；如设备重插后权限变化，请检查实际串口组。" >&2
        fi
      else
        echo "提示：串口 ${serial_port} 当前不存在；安装继续，设备接入后在维护页核对实际路径。" >&2
      fi
    fi
  fi
}

check_dependencies

python3 - "$package_dir" "$install_root" "$component" "$server_url" "$print_queue" "$serial_port" "$serial_baud" "$update_units" "$update_env" "$server_url_explicit" "$print_queue_explicit" "$serial_port_explicit" "$serial_baud_explicit" <<'PY'
import datetime, hashlib, json, os, pathlib, re, secrets, shutil, subprocess, sys
from urllib.parse import urlsplit, urlunsplit, unquote

source, root, component, server_url, print_queue, serial_port, serial_baud, update_units, update_env, server_url_explicit, print_queue_explicit, serial_port_explicit, serial_baud_explicit = sys.argv[1:]
root = pathlib.Path(root)
source = pathlib.Path(source)
component = str(component)
update_units = update_units == '1'
update_env = update_env == '1'
server_url_explicit = server_url_explicit == '1'
print_queue_explicit = print_queue_explicit == '1'
serial_port_explicit = serial_port_explicit == '1'
serial_baud_explicit = serial_baud_explicit == '1'
manifest = json.loads((source/'manifest.json').read_text())
version = manifest['version']
if not version or any(c not in 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-' for c in version):
    raise SystemExit('无效发布版本')
for binary, checksum in manifest['binaries'].items():
    if pathlib.Path(binary).name != binary or hashlib.sha256((source/'bin'/binary).read_bytes()).hexdigest() != checksum:
        raise SystemExit('二进制校验失败')

components = manifest.get('components') or {}
if components:
    if component == 'all':
        binaries = list(dict.fromkeys(components.get('server', []) + components.get('agent', [])))
    else:
        binaries = components.get(component)
    if not binaries:
        raise SystemExit(f'发布包不包含组件：{component}')
else:
    binaries = list(manifest['binaries'])
    if component == 'server':
        binaries = [b for b in binaries if b not in ('weighbridge-agent-printer',)]
    elif component == 'agent':
        binaries = [b for b in binaries if b in ('weighbridge-agent-printer', 'weighbridge-agent-provision')]

has_server = component in ('all', 'server')
has_agent = component in ('all', 'agent')
config_path = root/'configs/config.prod.yaml'
base_config_path = root/'configs/config.yaml'
database = ''
if has_server and not config_path.exists():
    database = os.environ.get('WEIGHBRIDGE_DATABASE_URL', '')
    if not database.startswith(('postgres://','postgresql://')):
        raise SystemExit('Server 组件首次安装必须提供本地 PostgreSQL URL')
    parsed_database = urlsplit(database)
    database_username = os.environ.get('WEIGHBRIDGE_DATABASE_USERNAME', unquote(parsed_database.username or ''))
    database_password = os.environ.get('WEIGHBRIDGE_DATABASE_PASSWORD', unquote(parsed_database.password or ''))
    if not database_username or not database_password:
        raise SystemExit('数据库 URL 须含账号密码，或设置 WEIGHBRIDGE_DATABASE_USERNAME/PASSWORD')
    database_plain = urlunsplit(parsed_database._replace(netloc=parsed_database.netloc.rsplit('@',1)[-1]))
for part in (['releases'] + (['configs', 'data/agent-images', 'data/uploads'] if has_server else []) + (['agent/state'] if has_agent else [])):
    (root/part).mkdir(parents=True, exist_ok=True, mode=0o700)
release = root/'releases'/version
if release.exists():
    raise SystemExit('此版本已安装；为每次构建分配唯一版本以保留回退文件')
release.mkdir(mode=0o700)
(release/'bin').mkdir(mode=0o700)
for binary in binaries:
    shutil.copy2(source/'bin'/binary, release/'bin'/binary)
    (release/'bin'/binary).chmod(0o700)
if has_server:
    shutil.copytree(source/'migrations', release/'migrations')
    os.symlink(root/'configs', release/'configs')
shutil.copy2(source/'manifest.json', release/'manifest.json')

if has_server and not base_config_path.exists():
    shutil.copy2(source/'configs/config.yaml', base_config_path)
if has_server and not config_path.exists():
    cfg = {
      'http': {'address':'0.0.0.0:18900'},
      'database': {'driver':'postgres','url':database_plain,'username':database_username,'password':database_password},
      'jwt': {'secret':secrets.token_urlsafe(48)},
      'weighing': {'image_dir':str(root/'data/agent-images')},
      'file': {'storage_root':str(root/'data/uploads')},
      'maintenance': {'enabled':True},
    }
    # JSON is a YAML subset; never interpolate credentials through the shell.
    with config_path.open('x') as f: json.dump(cfg,f,ensure_ascii=False,indent=2)
    config_path.chmod(0o600)

unit_dir = pathlib.Path(os.environ.get('WEIGHBRIDGE_SYSTEMD_UNIT_DIR', str(pathlib.Path.home()/'.config/systemd/user')))
unit_dir.mkdir(parents=True, exist_ok=True)
server_unit = f'''[Unit]
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
'''
agent_unit = f'''[Unit]
Description=Weighbridge local scale and printer Agent
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
legacy_patterns = [
    r'--server(?=\s|=|$)', r'--agent-id(?=\s|=|$)', r'--scale-id(?=\s|=|$)',
    r'--queue(?=\s|=|$)', r'--config-dir(?=\s|=|$)',
    r'APP_WEIGHING__AGENT_KEY', r'WEIGHBRIDGE_AGENT_KEY', r'WEIGHBRIDGE_AGENT_ID',
    r'WEIGHBRIDGE_SCALE_ID', r'WEIGHBRIDGE_SITE_', r'WEIGHBRIDGE_SCALE_', r'WEIGHBRIDGE_CAMERA_',
    r'weighbridge-agent-maintenance',
]
def legacy_tokens(text):
    return sorted({m.group(0) for pattern in legacy_patterns for m in re.finditer(pattern, text)})

def write_unit(name, text):
    target = unit_dir/name
    dropin_dir = unit_dir/(name+'.d')
    dropins = sorted(dropin_dir.glob('*.conf')) if dropin_dir.is_dir() else []
    dropin_tokens = {p: legacy_tokens(p.read_text(errors='replace')) for p in dropins}
    legacy_dropins = {p: tokens for p, tokens in dropin_tokens.items() if tokens}
    current = target.read_text() if target.exists() else None
    if current is None:
        temporary = unit_dir/('.'+name+'.tmp')
        temporary.write_text(text)
        temporary.chmod(0o644)
        os.replace(temporary, target)
        print('已写入 unit：'+str(target))
        if not legacy_dropins:
            return
        for path, found in legacy_dropins.items():
            print('  drop-in 含旧参数：'+str(path)+' ('+', '.join(found)+')')
        if not update_units:
            print('  保留现有 drop-in；核对后可用 --update-units 备份并停用旧参数。')
            return
    else:
        changed = current != text
        if changed:
            candidate = release/(name+'.candidate')
            candidate.write_text(text)
            print('unit 与当前模板不同，候选文件：'+str(candidate))
            tokens = legacy_tokens(current)
            if tokens:
                print('  现有 unit 含旧启动参数：'+', '.join(tokens))
        for path, found in legacy_dropins.items():
            print('  drop-in 含旧参数：'+str(path)+' ('+', '.join(found)+')')
        if not changed and not legacy_dropins:
            print('unit 已是最新：'+str(target))
            return
        if not update_units:
            print('  保留现有 unit/drop-in；核对后可用 --update-units 备份并替换或停用旧参数。')
            return
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    backup = root/'backups/units'/stamp
    backup.mkdir(parents=True, exist_ok=True, mode=0o700)
    if current is not None and current != text:
        shutil.copy2(target, backup/name)
        temporary = unit_dir/('.'+name+'.tmp')
        temporary.write_text(text)
        temporary.chmod(0o644)
        os.replace(temporary, target)
        print('已备份旧 unit 并替换：'+str(backup/name))
    for path, found in legacy_dropins.items():
        relative = path.relative_to(unit_dir)
        destination = backup/relative
        destination.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        shutil.copy2(path, destination)
        path.unlink()
        print('已停用并备份旧 drop-in：'+str(path))

if has_server:
    write_unit('weighbridge-business.service', server_unit)
if has_agent:
    write_unit('weighbridge-agent-printer.service', agent_unit)
    retired = unit_dir/'weighbridge-agent-maintenance.service'
    if retired.exists():
        found = legacy_tokens(retired.read_text(errors='replace'))
        print('发现旧维护服务 unit：'+str(retired)+((' ('+', '.join(found)+')') if found else ''))
        print('  统一维护已并入 Agent 主进程；核对后请停止并 disable 该旧服务。')
        if update_units:
            stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
            backup = root/'backups/units'/stamp
            backup.mkdir(parents=True, exist_ok=True, mode=0o700)
            shutil.copy2(retired, backup/retired.name)
            retired.unlink()
            retired_dropins = unit_dir/(retired.name+'.d')
            if retired_dropins.is_dir():
                shutil.copytree(retired_dropins, backup/(retired.name+'.d'))
                shutil.rmtree(retired_dropins)
            print('已备份并停用旧维护服务 unit；请 daemon-reload 后确认旧进程退出。')

if has_agent:
    provision = release/'bin/weighbridge-agent-provision'
    if not provision.exists():
        raise SystemExit('Agent 组件缺少 weighbridge-agent-provision，无法生成连接配置')
    env_path = root/'agent/agent.env'
    args = [str(provision), '--agent-env-path']
    if not env_path.exists():
        args += [str(env_path), '--server-url', server_url, '--print-queue', print_queue, '--serial-port', serial_port, '--serial-baud', serial_baud]
        subprocess.run(args, check=True)
        print('已生成 Agent 连接配置：'+str(env_path))
    else:
        upgraded = root/'agent/agent.env.upgraded'
        shutil.copy2(env_path, upgraded)
        upgraded.chmod(0o600)
        existing_backup = root/'agent/agent.env.legacy-camera.json'
        upgraded_backup = root/'agent/agent.env.upgraded.legacy-camera.json'
        if existing_backup.exists() and not upgraded_backup.exists():
            shutil.copy2(existing_backup, upgraded_backup)
            upgraded_backup.chmod(0o600)
        args += [str(upgraded), '--update-connection']
        if server_url_explicit:
            args += ['--server-url', server_url]
        if print_queue_explicit:
            args += ['--print-queue', print_queue]
        if serial_port_explicit:
            args += ['--serial-port', serial_port]
        if serial_baud_explicit:
            args += ['--serial-baud', serial_baud]
        subprocess.run(args, check=True)
        if upgraded.read_text() == env_path.read_text():
            print('Agent 连接配置无需迁移：'+str(env_path))
            upgraded.unlink()
            if upgraded_backup.exists() and not existing_backup.exists():
                os.replace(upgraded_backup, existing_backup)
        elif not update_env:
            print('Agent 配置含旧字段，已生成迁移候选：'+str(upgraded))
            if upgraded_backup.exists():
                print('旧摄像头配置备份：'+str(upgraded_backup))
            print('  核对后可用 --update-env 备份原文件并应用。')
        else:
            stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
            backup = root/'agent/backups'/('agent.env.'+stamp)
            backup.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
            shutil.copy2(env_path, backup)
            os.replace(upgraded, env_path)
            if upgraded_backup.exists():
                os.replace(upgraded_backup, existing_backup)
            print('已备份并迁移 Agent 连接配置：'+str(env_path)+'（备份 '+str(backup)+'）')

print('已安装组件：'+component+'，版本：'+version)
if has_server:
    print('下一步：备份后执行 releases/'+version+'/bin/weighbridge-migrate，初始化管理员，核对 current 与 unit，再启动业务服务。')
if has_agent:
    print('下一步：核对 CUPS 队列、串口权限与 agent.env，再启用并启动 Agent 服务。')
PY
