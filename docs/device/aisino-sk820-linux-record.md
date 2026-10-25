# Aisino SK-820 针式打印机 Linux 适配与打印验证记录

日期：2026-10-18
范围：现场主机上的 USB 针式打印机识别、CUPS 队列配置、中文打印与二维码打印验证。

本文记录 2026-10-18 当时的驱动与打印硬件实验；当时 Agent 及业务打印尚未接入，该描述不代表当前实现。正式打印规则见[业务规格](../requirements/weighing-business-spec.md#6-单据查询与打印)，后续软件与现场证据见[当前状态](../next.md)。

## 1. 环境与设备

| 项目 | 值 |
| --- | --- |
| 主机 | HP ProDesk 400 G2 Mini |
| 系统 | Ubuntu 26.04.1 LTS（Resolute Raccoon） |
| SSH | `hp@hp-HP-ProDesk-400-G2-MINI`，本地别名 `weighbridge-hp`，密钥 `~/.ssh/id_ed25519_weighbridge` |
| CUPS | 2.4.16-1ubuntu1.3（`enabled`、`active`） |
| 打印相关包 | `cups`、`cups-client`、`cups-ppdc`、`cups-filters`、`foomatic-db-compressed-ppds` |
| 打印机 | DASCOM Co.,Ltd Aisino SK-820 Printer，USB `20d1:2010` |
| 内核设备 | `usblp` 绑定，生成 `/dev/usb/lp0`（`root:lp`，`660`） |
| CUPS 设备 URI | `usb://Aisino/SK-820?serial=Aisino%20SK-820` |

宿主机 `hp` 用户属于 `lpadmin` 组，因此队列管理不需要 sudo；安装软件包仍需要 sudo 密码，本次未执行任何安装。

## 2. 设备识别

```console
$ lsusb
Bus 001 Device 002: ID 20d1:2010 DASCOM Co.,Ltd Aisino SK-820 Printer

$ lsusb -v -d 20d1:2010 | grep -E 'bInterfaceClass|bInterfaceSubClass|bInterfaceProtocol|iProduct'
  iProduct                2 Aisino SK-820 Printer
  bInterfaceClass         7 Printer
  bInterfaceSubClass      1 Printer
  bInterfaceProtocol      2 Bidirectional

$ ls -l /dev/usb/lp0
crw-rw---- 1 root lp 180, 0 Oct 18 17:16 /dev/usb/lp0
```

接口类 `7/1/2` 表示标准 USB Printer Class（双向），说明它是内核和 CUPS 都能直接识别的标准打印机设备，不需要得实（DASCOM）的专有 USB 驱动。CUPS 的 `usb` 后端通过 libusb 直接访问设备；`usblp` 同时生成了 `/dev/usb/lp0`，可用于绕过 CUPS 的裸写验证。

## 3. CUPS 队列

系统已预装 CUPS 和 foomatic PPD 集合，`lpinfo -m` 中可用的针式驱动包括：

- `Generic ESC/P Dot Matrix Printer Foomatic/lq850`（Epson LQ-850 仿真，24 针）
- `Generic ESC/P Dot Matrix Printer Foomatic/epson`
- `drv:///sample.drv/epson24.ppd`（Epson 24-Pin Series）

未安装得实官方 Linux 驱动。SK-820 兼容 ESC/P，使用通用 24 针驱动即可。

### 3.1 驱动队列（默认）

```bash
lpadmin -p Aisino-SK820 -E \
  -v "usb://Aisino/SK-820?serial=Aisino%20SK-820" \
  -m "foomatic-db-compressed-ppds:0/ppd/foomatic-ppd/Generic-ESC_P_Dot_Matrix_Printer-lq850.ppd" \
  -D "Aisino SK-820 Dot Matrix" -L "Weighbridge" \
  -o printer-is-shared=false
cupsenable Aisino-SK820
cupsaccept Aisino-SK820
lpadmin -d Aisino-SK820
```

### 3.2 raw 队列（票据与图形）

```bash
lpadmin -p Aisino-SK820-raw -E \
  -v "usb://Aisino/SK-820?serial=Aisino%20SK-820" \
  -m raw \
  -D "Aisino SK-820 raw ESC/P" -L "Weighbridge" \
  -o printer-is-shared=false
cupsenable Aisino-SK820-raw
cupsaccept Aisino-SK820-raw
```

| 队列 | 驱动 | 用途 |
| --- | --- | --- |
| `Aisino-SK820` | Foomatic lq850（24 针 ESC/P） | 普通 ASCII/文档，系统默认队列 |
| `Aisino-SK820-raw` | raw（原样透传） | GB18030 中文、ESC/P 票据、二维码位图 |

CUPS 2.4 对 `-m raw` 与 PPD 驱动均给出弃用提示（“原始序列已被弃用”“打印机驱动已被弃用”），当前版本功能正常；未来 CUPS 大版本升级时需要复验或改为直接写 `/dev/usb/lp0`。

## 4. 验证结果

| # | 项目 | 方法 | 结果 |
| --- | --- | --- | --- |
| 1 | USB 设备识别 | `lsusb`、`/dev/usb/lp0` | 通过 |
| 2 | CUPS 发现设备 | `lpinfo -v` 返回 `usb://Aisino/SK-820?...` | 通过 |
| 3 | 驱动队列 ASCII 打印 | `printf ... \| lp -d Aisino-SK820` | 出字正常（现场确认） |
| 4 | raw 队列 GB18030 中文 | 初始化 + GB18030 文本 + 换页 | 中文正常（现场确认），内置字库可用 |
| 5 | 二维码位图打印 | ESC/P 24 针图形指令 | 通过；手机扫码可识别，精度良好（现场确认） |
| 6 | 驱动队列中文打印 | 未测试 | 不采用；票据中文走 raw |

ASCII 测试：

```bash
printf "Weighbridge SK-820 Test\nESC/P 24-pin via CUPS\n\n\n" \
  | lp -d Aisino-SK820 -o PageSize=A4 -t "self-test"
```

中文测试：

```bash
{ printf '\033@'; printf '中文测试 酒厂地磅 SK-820 1234567890\n\r'; printf '\014'; } \
  | iconv -f UTF-8 -t GB18030 \
  | lp -d Aisino-SK820-raw -o raw -t "gb18030-test"
```

结论：SK-820 内置 GB18030 字库，业务数据转 GB18030 后可直接以 ESC/P 文本打印，不需要在软件侧渲染点阵字模。CUPS 驱动队列会经 `texttopdf`/Ghostscript 转换，中文依赖系统 CJK 字库且会改变版式，票据打印不使用该路径。

## 5. 二维码打印

### 5.1 参数

| 参数 | 值 | 说明 |
| --- | --- | --- |
| 内容 | `https://api.ezios.cn:30000/sign-in?redirect=%2Fdashboard%2Foverview` | 67 字节，ASCII |
| 纠错等级 | Q | 点阵打印有墨点扩散，取较高纠错 |
| QR 版本 | 6（41×41 模块） | 67 字节在 Q 等级下超出 V5 容量 |
| 静区 | 4 模块（每边） | 含静区后 49×49 |
| 模块尺寸 | 5 个打印点 | 180 dpi 下约 0.706 mm |
| 成品尺寸 | 245×245 dots | 约 34.6 mm × 34.6 mm |
| 水平分辨率 | 180 dpi | `ESC *` 模式 39 |
| 垂直分辨率 | 180 dpi | 行距 24/180 英寸，24 点一行 |

34.6 mm 在 82 列针打的可打印宽度内，尺寸适中：模块过小会被墨迹糊连，过大则占用票面。

### 5.2 ESC/P 指令序列

| 指令 | 字节 | 作用 |
| --- | --- | --- |
| `ESC @` | `1B 40` | 初始化打印机 |
| `ESC 3 24` | `1B 33 18` | 行距设为 24/180 英寸，与 24 点图形带对齐 |
| `ESC * 39 nL nH` | `1B 2A 27 nL nH` | 24 针三倍密度位图，180 dpi；每列 3 字节，高位在上 |
| `CR LF` | `0D 0A` | 每 24 行图形带结束后走一行 |
| `ESC 2` | `1B 32` | 恢复默认行距 |
| 文本 + `FF` | GB18030 + `0C` | 打印说明文字并换页 |

位图数据按 24 行一个带切分，每带按列输出；为避开打印缓冲限制，每条 `ESC *` 最多 128 列。24 针数据每列 3 字节：第 1 字节对应第 1–8 点（bit7 为最上点），第 2 字节对应 9–16 点，第 3 字节对应 17–24 点。

### 5.3 生成与发送

生成脚本（本机临时环境，`qrcode` 安装到独立目录以避开 PEP 668）：

```bash
python3 -m pip install --target /tmp/qrpkg qrcode
```

```python
# make_qr_bin.py
import sys
sys.path.insert(0, "/tmp/qrpkg")
import qrcode
from qrcode.constants import ERROR_CORRECT_Q

URL = "https://api.ezios.cn:30000/sign-in?redirect=%2Fdashboard%2Foverview"
MODULE_DOTS = 5
DOTS_PER_INCH = 180

qr = qrcode.QRCode(error_correction=ERROR_CORRECT_Q, border=4)
qr.add_data(URL)
qr.make(fit=True)
matrix = qr.get_matrix()  # 含 4 模块静区

n = len(matrix)
pixels = [[matrix[y // MODULE_DOTS][x // MODULE_DOTS]
           for x in range(n * MODULE_DOTS)]
          for y in range(n * MODULE_DOTS)]
height, width = len(pixels), len(pixels[0])

out = bytearray(b"\x1b@")      # ESC @
out += b"\x1b3\x18"            # ESC 3 24 (1/180 inch)

BAND, CHUNK = 24, 128
for y0 in range(0, height, BAND):
    band = pixels[y0:y0 + BAND]
    if len(band) < BAND:
        band += [[False] * width for _ in range(BAND - len(band))]
    for x0 in range(0, width, CHUNK):
        cols = min(CHUNK, width - x0)
        data = bytearray()
        for x in range(x0, x0 + cols):
            b1 = b2 = b3 = 0
            for r in range(24):
                if band[r][x]:
                    if r < 8:
                        b1 |= 1 << (7 - r)
                    elif r < 16:
                        b2 |= 1 << (7 - (r - 8))
                    else:
                        b3 |= 1 << (7 - (r - 16))
            data += bytes((b1, b2, b3))
        out += b"\x1b*" + bytes((39, cols & 0xFF, cols >> 8)) + data
    out += b"\r\n"

out += b"\x1b2"  # ESC 2
out += "\r\n扫码登录 / Sign in\r\n\r\n\r\n".encode("gb18030")
out += b"\x0c"   # FF

open("/tmp/qr.bin", "wb").write(out)
```

发送：

```bash
cat /tmp/qr.bin | ssh weighbridge-hp 'lp -d Aisino-SK820-raw -o raw -t "qr-login"'
```

作业正常结束，`lpstat -o` 无残留，队列回到空闲。

## 6. 已知限制与待办

- 二维码已实际出纸并通过手机扫码验证（2026-10-18，现场确认精度良好）。若后续更换 URL、纸型或模块尺寸导致识别困难，可按以下顺序调整：模块 5→6 点、纠错 Q→M（版本 5，模块数 45）、增加 `ESC U 1` 单向打印，或适当右移避开左边距。
- 未验证连续纸/多联票据、A4 之外纸型、超长 URL、`ESC *` 其他模式以及打印速度对图形质量的影响。
- CUPS 2.4 的 raw 与 PPD 驱动均有弃用提示；升级 CUPS 前需回归。
- 本次未接入 Agent，也未实现打印任务状态、失败重试或补打；这些属于后续现场阶段。
- 安装软件包需要 sudo 密码；本次使用的包均为系统已有。

## 7. 入口与排障

```bash
lpstat -p -d                 # 队列与默认目标
lpstat -v                    # 设备 URI
lpstat -o -W all             # 全部作业
cancel -a Aisino-SK820-raw   # 清空队列
lpinfo -v                    # CUPS 可见设备
lpinfo -m | grep -i esc      # 可用 ESC/P 驱动
journalctl -u cups -n 50     # 服务日志
```

CUPS 管理界面可从本机转发：`ssh -L 631:localhost:631 weighbridge-hp`，浏览器访问 `http://localhost:631`。

设备主机本地留有一份日常使用说明：`~/Documents/weighbridge-printer/README.md`（队列选择、票据/中文/二维码用法、常用命令与排障表），与本记录同步维护。

## 8. 后续 Agent 联调

2026-10-19 已实现并在本机部署最小打印 Agent，完成业务下发至 CUPS 的一次补打，初版出纸后按现场反馈补齐实线表格并收窄至约 175 mm，用户已确认 A4 单张版面可用；五联纸及连续走纸仍未验收。历史记录中的未实现边界以当时验证范围为准，最新状态见[现场联调记录](aisino-sk820-agent-test.md)。
