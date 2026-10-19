# 把二维码打上 24 针针式打印机：一次 Linux + CUPS + ESC/P 调试记录

> 面向技术博客的整理稿。文中主机名、内网地址和业务 URL 已用占位符替换；具体测试数据见同目录的[适配与验证记录](aisino-sk820-linux-record.md)。

地磅房要打印过磅单，票面上除了中文，还要印一个可扫码的二维码。手头是一台得实（DASCOM）代工的航天信息 Aisino SK-820，24 针、82 列、USB 接口，接在一台 Ubuntu 工控机上。目标很朴素：不装任何厂商驱动，让 Linux 能稳定打出中文和二维码。

下面是从“这玩意儿认不认”到“二维码能不能扫”的完整过程，以及中间踩过的坑。

## 一、先别急着找驱动

拿到 USB 打印机，第一步不是搜“Linux 驱动下载”，而是看它到底是什么设备：

```console
$ lsusb
Bus 001 Device 002: ID 20d1:2010 DASCOM Co.,Ltd Aisino SK-820 Printer

$ lsusb -v -d 20d1:2010 | grep -E 'bInterfaceClass|bInterfaceSubClass|bInterfaceProtocol'
  bInterfaceClass         7 Printer
  bInterfaceSubClass      1 Printer
  bInterfaceProtocol      2 Bidirectional
```

`7/1/2` 是标准 USB Printer Class。这个信息很关键：

- **class 7**：设备按标准打印机协议说话，内核 `usblp` 会绑定它，生成 `/dev/usb/lp0`；CUPS 的 `usb` 后端也能通过 libusb 直接用。
- **class 255/厂商自定义**：那才是“必须装专有驱动”的信号，通常还得配套厂商的 CUPS filter。

这台机器上：

```console
$ ls -l /dev/usb/lp0
crw-rw---- 1 root lp 180, 0 ... /dev/usb/lp0
```

节点存在，说明内核侧没问题。CUPS 里也能直接看到：

```console
$ lpinfo -v
direct usb://Aisino/SK-820?serial=Aisino%20SK-820
```

URI 里带序列号，重新插拔也不会变，适合写进配置。

> 小知识：CUPS 1.6 之后默认用 libusb 直连打印机，不再依赖 `usblp`。如果哪天 CUPS 报 “Unable to open device”，常见原因是 `usblp` 和 libusb 抢设备，可以 `sudo modprobe -r usblp` 或把它加入 blacklist。反过来，如果打算直接 `cat file > /dev/usb/lp0`，就要保留 `usblp`。

## 二、用通用 ESC/P 驱动打出第一行字

Ubuntu 的 `cups` + `cups-filters` + `foomatic-db-compressed-ppds` 已经带了大量针式驱动。查一下：

```bash
lpinfo -m | grep -iE 'esc.?p|dot.?matrix|lq'
```

能看到：

```text
Generic ESC/P Dot Matrix Printer Foomatic/lq850
Generic ESC/P Dot Matrix Printer Foomatic/epson
drv:///sample.drv/epson24.ppd  Epson 24-Pin Series
```

SK-820 是 24 针、兼容 ESC/P，选 `lq850`（Epson LQ-850 仿真）最贴近。建队列：

```bash
lpadmin -p Aisino-SK820 -E \
  -v "usb://Aisino/SK-820?serial=Aisino%20SK-820" \
  -m "foomatic-db-compressed-ppds:0/ppd/foomatic-ppd/Generic-ESC_P_Dot_Matrix_Printer-lq850.ppd" \
  -D "Aisino SK-820 Dot Matrix" \
  -o printer-is-shared=false

cupsenable Aisino-SK820
cupsaccept Aisino-SK820
lpadmin -d Aisino-SK820
```

注意：`lpadmin` 需要当前用户在 `lpadmin` 组里（`sudo usermod -aG lpadmin $USER`，重新登录生效），但不需要每次 sudo。检查：

```bash
id | tr ' ' '\n' | grep lpadmin
```

然后打一页 ASCII：

```bash
printf "Hello Dot Matrix\nESC/P test\n\n\n" | lp -d Aisino-SK820 -t "self-test"
lpstat -p Aisino-SK820
```

如果纸上有字，USB、CUPS、驱动这条链路就通了。

这里会看到两条弃用警告：`原始序列已被弃用`、`打印机驱动已被弃用`。CUPS 2.4 仍然完整支持，但意味着未来大版本可能移除，升级前要回归。

## 三、中文：不要走 CUPS 的文本转换

ASCII 通了不等于中文能打。CUPS 处理纯文本的路径大致是：

```text
text/plain → texttopdf → Ghostscript → foomatic-rip/驱动 → ESC/P
```

这条路径依赖系统字体、会重新排版，对票据这种“每个字符位置都要可控”的场景并不合适；而且中文能不能出来取决于系统有没有装 CJK 字体，打出来也未必是针打内置字库的效果。

正确做法是建一个 **raw 队列**，让 CUPS 原样透传字节：

```bash
lpadmin -p Aisino-SK820-raw -E \
  -v "usb://Aisino/SK-820?serial=Aisino%20SK-820" \
  -m raw -D "Aisino SK-820 raw ESC/P"

cupsenable Aisino-SK820-raw
cupsaccept Aisino-SK820-raw
```

然后按打印机的字符编码发数据。国产针打通常内置 GB18030 字库，于是可以这样测试：

```bash
{ printf '\033@'; printf '中文测试 酒厂地磅 1234567890\n\r'; printf '\014'; } \
  | iconv -f UTF-8 -t GB18030 \
  | lp -d Aisino-SK820-raw -o raw -t "gb18030-test"
```

`\033@` 是 `ESC @`，初始化打印机；`\014` 是换页（FF）。如果中文正常出字，说明打印机内置 GB18030 字库——这是最省事的结论：业务侧只需要把 UTF-8 转成 GB18030，不需要自己渲染点阵字。

如果中文变成空白或问号，就说明没有对应字库，只能走图形方式：在软件里用点阵字体渲染成位图，再用下面的 `ESC *` 指令打印。

## 四、直接对打印机说 ESC/P

针打的可控性来自 ESC/P 指令。票据打印常用的几条：

| 指令 | 字节 | 含义 |
| --- | --- | --- |
| `ESC @` | `1B 40` | 初始化，清状态、恢复默认 |
| `ESC 3 n` | `1B 33 n` | 行距设为 n/180 英寸（24 针） |
| `ESC 2` | `1B 32` | 恢复默认行距 |
| `ESC * m nL nH` | `1B 2A ...` | 位图打印 |
| `CR LF` | `0D 0A` | 回车换行 |
| `FF` | `0C` | 换页 |

`ESC *` 的 `m` 是位图模式。24 针打印机常用 `m=39`：三倍密度、180 dpi 水平分辨率，每个图形带 24 行。数据按列组织，每列 3 字节：

```text
第 1 字节：第 1–8 点，bit7 是最上面一点
第 2 字节：第 9–16 点
第 3 字节：第 17–24 点
```

一列 3 字节，`nL + nH*256` 是列数。为了不让打印缓冲溢出，通常每条指令不超过 128 列，分块发送。

垂直方向要对齐：`ESC *` 每带固定 24 点高，所以先用 `ESC 3 24` 把行距设成 24/180 英寸，打印完一带再 `CR LF`，下一带才会严丝合缝地接上。行距不对，图形就会被压扁或拉出白缝。

## 五、二维码：尺寸不是拍脑袋定的

二维码的物理尺寸由三个量决定：

```text
模块数 × 每模块点数 ÷ 打印机 DPI × 25.4 = 毫米
```

模块数又由内容长度和纠错等级决定。一个 67 字节的 URL，在纠错等级 Q 下需要 QR Version 6，即 41×41 模块；加上每边 4 个模块的静区，总共 49×49。

SK-820 在 `ESC * 39` 模式下是 180 dpi。取每模块 5 个点：

```text
49 × 5 = 245 dots
245 ÷ 180 × 25.4 ≈ 34.6 mm
```

这个尺寸在 82 列针打的可打印宽度内，实测手机可以稳定识别。为什么不用 3 个点？因为针打的墨点会扩散，180 dpi 下 3 点只有 0.42 mm，相邻模块容易糊在一起；5 点约 0.71 mm，容错更好。再大就会挤占票面。

另外两个容易忽略的点：

- **静区不能省**。二维码四周的空白是识别的一部分，至少保留 4 个模块；打印时左边距不够，宁可整体右移。
- **纠错等级别一味求高**。Q 等级让版本从 5 涨到 6，模块更多、整体更大；如果尺寸受限，M 等级反而能在同样物理宽度下换来更大的单模块。

生成位图的核心逻辑（完整脚本见[验证记录](aisino-sk820-linux-record.md)）：

```python
import qrcode
from qrcode.constants import ERROR_CORRECT_Q

qr = qrcode.QRCode(error_correction=ERROR_CORRECT_Q, border=4)
qr.add_data(url)
qr.make(fit=True)
matrix = qr.get_matrix()          # 已含静区

DOTS = 5                          # 每模块 5 个打印点
pixels = [[matrix[y // DOTS][x // DOTS]
           for x in range(len(matrix) * DOTS)]
          for y in range(len(matrix) * DOTS)]
```

然后把 `pixels` 按 24 行切带，每列压成 3 字节，前面加上 `ESC * 39` 的表头，再通过 raw 队列发出去：

```bash
cat qr.bin | ssh printer-host 'lp -d Aisino-SK820-raw -o raw -t "qr"'
```

## 六、踩坑清单

1. **“没有 TTY”导致 ssh-copy-id 失败**。在自动化会话里执行 `ssh-copy-id`，密码提示会报 `ssh_askpass: exec(/usr/bin/ssh-askpass): No such file or directory`。要么在普通终端里做，要么直接在目标机 `~/.ssh/authorized_keys` 追加公钥（记得用 `>>`）。
2. **PEP 668 挡住 pip**。Ubuntu 26.04 的 Python 是 externally-managed，`pip install --user` 也会被拒。用 `pip install --target /tmp/pkg qrcode` 装到独立目录，再 `sys.path.insert`，不污染系统环境。
3. **`lpadmin` 权限**。用户加入 `lpadmin` 组后要重新登录；组权限生效后，建队列、设默认都不再需要 sudo。
4. **raw 和 PPD 驱动都会被弃用**。CUPS 2.4 的提示不影响使用，但别把 `-m raw` 当成永久 API；直写 `/dev/usb/lp0` 是退路，但要注意 `lp` 组权限和多个进程并发。
5. **换页方式**。票据是连续纸时，`FF` 可能一次走掉一整页；用若干 `LF` 更可控。A4 切纸则可以直接 `FF`。
6. **双向打印**。图形和竖线在双向模式下可能出现左右错位，必要时 `ESC U 1` 切单向，代价是速度。
7. **中文走错队列**。raw 队列只负责透传，别指望它做编码转换；驱动队列会排版，别指望它保持票据坐标。

## 七、最后的检查单

- `lsusb -v` 确认 interface class 是 7；
- `lpinfo -v` 能看到 `usb://` URI；
- 用 ASCII 测通驱动队列；
- 用 GB18030 + raw 队列测通中文；
- 图形用 `ESC * 39` + `ESC 3 24`，分块发送；
- 二维码模块 ≥5 点、静区 4 模块，打完必须用真机扫一次；
- 记录 CUPS 版本和队列配置，升级前回归。

从“要不要装驱动”到“二维码能扫”，真正需要的其实只有三样东西：确认设备类、选对 ESC/P 模式、把编码和尺寸算清楚。厂商驱动反而是最后才考虑的事。
