"""Render a compact sheet with solid table rules for the SK-820 (180 dpi).
Runtime dependencies: python3, Pillow, and an installed Chinese TrueType font.
"""
import json
import os
import sys
from PIL import Image, ImageDraw, ImageFont

DEFAULT_WIDTH, HEIGHT = 1240, 600
FONT = '/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc'


def render_image(sheet):
    calibration = sheet.get('calibration') or {}
    width_mm = calibration.get('printWidthMm') or 175
    width = round(width_mm * 180 / 25.4)
    offset_x = round(calibration.get('offsetXmm', 0) * 180 / 25.4)
    offset_y = round(calibration.get('offsetYmm', 0) * 180 / 25.4)
    form_length = int(calibration.get('formLengthMm') or 99)
    feed_height = ((HEIGHT + offset_y + 23) // 24) * 24
    if (not 100 <= width_mm <= 210 or not 85 <= form_length <= 300
            or not 0 <= calibration.get('offsetXmm', 0) <= 30
            or not 0 <= calibration.get('offsetYmm', 0) <= 30
            or width_mm + calibration.get('offsetXmm', 0) > 210
            or feed_height > round(form_length * 180 / 25.4)):
        raise ValueError('calibration offsets exceed printable area')
    content_width = width
    x_origin = 0
    top = 84
    font_path = os.environ.get('WEIGHBRIDGE_PRINT_FONT', FONT)
    normal = ImageFont.truetype(font_path, 24)
    title_font = ImageFont.truetype(font_path, 34)
    im = Image.new('1', (width, HEIGHT), 1)
    draw = ImageDraw.Draw(im)
    draw.text((x_origin + content_width // 2, 5), sheet['title'], font=title_font, fill=0, anchor='mt')
    marks = '  '.join(sheet.get('marks') or [])
    if marks:
        draw.text((x_origin + content_width - 10, 48), marks, font=normal, fill=0, anchor='rt')
    row_height = 94
    rows = sheet['rows']
    if len(rows) != 5:
        raise ValueError('compact sheet requires five rows')
    draw.rectangle((x_origin + 1, top, x_origin + content_width - 2, top + len(rows) * row_height), outline=0, width=2)
    for row_no, row in enumerate(rows):
        if not 1 <= len(row) <= 3:
            raise ValueError('unsupported cell count')
        y = top + row_no * row_height
        if row_no:
            draw.line((x_origin + 1, y, x_origin + content_width - 2, y), fill=0, width=2)
        for col, field in enumerate(row):
            left = x_origin + round(col * (content_width - 4) / len(row)) + 2
            right = x_origin + round((col + 1) * (content_width - 4) / len(row)) + 2
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
    if draw.textlength(sheet['title'], font=title_font) > content_width - 24:
        raise ValueError('title exceeds page width')
    if draw.textlength(marks, font=normal) > content_width - 24:
        raise ValueError('marks exceed page width')
    if offset_x or offset_y:
        # Offsets add blank paper before the entire image; they must never
        # squeeze the table or crop its title, marks, borders, or contents.
        positioned = Image.new('1', (width + offset_x, HEIGHT + offset_y), 1)
        positioned.paste(im, (offset_x, offset_y))
        return positioned
    return im


def escp(im, sheet):
    # 99 mm form, 180 dpi square pixels. 24-row bands use the same ESC * 39
    # path already verified by the onsite QR test. No font alignment assumptions.
    sheet_length = int((sheet.get('calibration') or {}).get('formLengthMm') or 99)
    form = round(sheet_length * 360 / 25.4)
    out = bytearray(b'\x1b@\x1b(U\x01\x00\x0a\x1b(C\x02\x00' + bytes((form & 255, form >> 8)) + b'\x1bU\x01\x1b3\x18')
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
        sys.stdout.buffer.write(escp(image, sheet))
