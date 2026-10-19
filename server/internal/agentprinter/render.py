"""Render a compact sheet with solid table rules for the SK-820 (180 dpi).
Runtime dependencies: python3, Pillow, and an installed Chinese TrueType font.
"""
import json
import os
import sys
from PIL import Image, ImageDraw, ImageFont

WIDTH, HEIGHT = 1240, 600
FONT = '/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc'


def render_image(sheet):
    font_path = os.environ.get('WEIGHBRIDGE_PRINT_FONT', FONT)
    normal = ImageFont.truetype(font_path, 24)
    title_font = ImageFont.truetype(font_path, 34)
    im = Image.new('1', (WIDTH, HEIGHT), 1)
    draw = ImageDraw.Draw(im)
    draw.text((WIDTH // 2, 5), sheet['title'], font=title_font, fill=0, anchor='mt')
    marks = '  '.join(sheet.get('marks') or [])
    if marks:
        draw.text((WIDTH - 10, 48), marks, font=normal, fill=0, anchor='rt')
    top, row_height = 84, 94
    rows = sheet['rows']
    if len(rows) != 5:
        raise ValueError('compact sheet requires five rows')
    draw.rectangle((1, top, WIDTH - 2, top + len(rows) * row_height), outline=0, width=2)
    for row_no, row in enumerate(rows):
        if not 1 <= len(row) <= 3:
            raise ValueError('unsupported cell count')
        y = top + row_no * row_height
        if row_no:
            draw.line((1, y, WIDTH - 2, y), fill=0, width=2)
        for col, field in enumerate(row):
            left = round(col * (WIDTH - 4) / len(row)) + 2
            right = round((col + 1) * (WIDTH - 4) / len(row)) + 2
            if col:
                draw.line((left, y, left, y + row_height), fill=0, width=2)
            text = field['label'] + ': ' + field['value']
            lines, line = [], ''
            for char in text:
                if draw.textlength(line + char, font=normal) > right - left - 24:
                    lines.append(line)
                    line = ''
                line += char
            lines.append(line)
            if len(lines) > 2:
                raise ValueError('field exceeds two-line cell; refusing truncation')
            for number, line in enumerate(lines):
                draw.text((left + 12, y + (row_height - len(lines) * 34) // 2 + number * 34), line, font=normal, fill=0)
    if draw.textlength(sheet['title'], font=title_font) > WIDTH - 24:
        raise ValueError('title exceeds page width')
    if draw.textlength(marks, font=normal) > WIDTH - 24:
        raise ValueError('marks exceed page width')
    return im


def escp(im):
    # 99 mm form, 180 dpi square pixels. 24-row bands use the same ESC * 39
    # path already verified by the onsite QR test. No font alignment assumptions.
    out = bytearray(b'\x1b@\x1b(U\x01\x00\x0a\x1b(C\x02\x00\x7b\x05\x1bU\x01\x1b3\x18')
    pixels = im.load()
    for y in range(0, im.height, 24):
        for x in range(0, im.width, 128):
            count = min(128, im.width - x)
            out += b'\x1b*' + bytes((39, count & 255, count >> 8))
            for column in range(x, x + count):
                for byte in range(3):
                    value = 0
                    for bit in range(8):
                        row = y + byte * 8 + bit
                        if row < im.height and pixels[column, row] == 0:
                            value |= 1 << (7 - bit)
                    out.append(value)
        out += b'\r\n'
    out += b'\x1b2\x0c'
    return bytes(out)


if __name__ == '__main__':
    sheet = json.load(sys.stdin)
    image = render_image(sheet)
    if len(sys.argv) == 2:
        image.save(sys.argv[1])
    else:
        sys.stdout.buffer.write(escp(image))
