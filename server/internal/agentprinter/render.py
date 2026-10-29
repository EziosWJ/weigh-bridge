"""Render a compact sheet with solid table rules for the SK-820 (180 dpi).
Runtime dependencies: python3, Pillow, and an installed Chinese TrueType font.
"""
import json
import math
import os
import sys
from PIL import Image, ImageDraw, ImageFont

DEFAULT_WIDTH, HEIGHT = 1240, 600
FONT = '/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc'


def rounded(value):
    # Match Go math.Round: half away from zero, including negative offsets.
    return math.floor(value + 0.5) if value >= 0 else math.ceil(value - 0.5)


def geometry(sheet):
    c = sheet.get('calibration') or {}
    width_mm = c.get('printWidthMm', 175)
    scale = c.get('printScale') or 1
    form_length = c.get('formLengthMm', 93 if sheet.get('template') == 'weighing-compact-v2' else 99)
    x, y = c.get('offsetXmm', 0), c.get('offsetYmm', 0)
    if not all(math.isfinite(v) for v in (width_mm, scale, form_length, x, y)):
        raise ValueError('calibration must be finite')
    if (not 100 <= width_mm <= 210 or not .5 <= scale <= 1.5
            or not 85 <= form_length <= 300 or not 0 <= x <= 30 or not -30 <= y <= 30):
        raise ValueError('calibration outside allowed range')
    width = rounded(width_mm * 180 / 25.4)
    base_height = 576 if sheet.get('template') == 'weighing-compact-v2' else HEIGHT
    scaled_width, scaled_height = rounded(width * scale), rounded(base_height * scale)
    dx, dy = rounded(x * 180 / 25.4), rounded(y * 180 / 25.4)
    if sheet.get('template') == 'weighing-compact-v2':
        # The tractor strips sit outside the centered 210 mm print window.
        # Calibrated offsets translate from that centered position.
        dx += max(0, (rounded(210 * 180 / 25.4) - scaled_width) // 2)
        dy += max(0, (rounded(form_length * 180 / 25.4) - scaled_height) // 2)
    if dy < 0:
        raise ValueError('vertical offset exceeds centered top margin')
    feed_height = ((scaled_height + dy + 23) // 24) * 24
    form_units = rounded(form_length * 360 / 25.4)
    if scaled_width + dx > rounded(210 * 180 / 25.4) or feed_height * 2 > form_units:
        raise ValueError('calibration offsets exceed printable area')
    return width, scaled_width, scaled_height, dx, dy, form_units


def calibration_pattern(im, font_path):
    # Rulers are added AFTER scaling: each tick always represents a physical mm.
    draw = ImageDraw.Draw(im)
    w, h = im.size
    draw.rectangle((0, 0, w - 1, h - 1), outline=0)
    for x in (w // 2,):
        draw.line((x, 0, x, 2), fill=0)
        draw.line((x, h - 3, x, h - 1), fill=0)
    for y in (h // 2,):
        draw.line((0, y, 2, y), fill=0)
        draw.line((w - 3, y, w - 1, y), fill=0)
    font = ImageFont.truetype(font_path, 12)
    origin, baseline = 18, h - 2
    draw.line((origin, baseline, w - 18, baseline), fill=0)
    for mm in range(int((w - 36) * 25.4 / 180) + 1):
        x = origin + rounded(mm * 180 / 25.4)
        length = 7 if mm % 10 == 0 else 3
        draw.line((x, baseline - length, x, baseline), fill=0)
        if mm % 10 == 0:
            draw.text((x, baseline - 9), str(mm), font=font, fill=0, anchor='mb')


def render_image(sheet):
    width, scaled_width, scaled_height, offset_x, offset_y, _ = geometry(sheet)
    content_width = width
    x_origin = 0
    current = sheet.get('template') == 'weighing-compact-v2'
    top = 90 if current else 84
    font_path = os.environ.get('WEIGHBRIDGE_PRINT_FONT', FONT)
    normal = ImageFont.truetype(font_path, 28 if current else 24)
    title_font = ImageFont.truetype(font_path, 40 if current else 34)
    im = Image.new('1', (width, 576 if current else HEIGHT), 1)
    draw = ImageDraw.Draw(im)
    draw.text((x_origin + content_width // 2, 5), sheet['title'], font=title_font, fill=0, anchor='mt')
    marks = '  '.join(sheet.get('marks') or [])
    if marks:
        draw.text((x_origin + content_width - 10, 48), marks, font=normal, fill=0, anchor='rt')
    rows = sheet['rows']
    expected = 5 if sheet.get('calibrationPattern') or not current else 6
    if len(rows) != expected:
        raise ValueError('unsupported compact sheet row count')
    heights = [54, 66, 72, 120, 70, 100] if current and expected == 6 else [94] * 5
    draw.rectangle((x_origin + 1, top, x_origin + content_width - 2, top + sum(heights)), outline=0, width=2)
    for row_no, row in enumerate(rows):
        if not 1 <= len(row) <= 3:
            raise ValueError('unsupported cell count')
        y = top + sum(heights[:row_no])
        row_height = heights[row_no]
        if row_no:
            draw.line((x_origin + 1, y, x_origin + content_width - 2, y), fill=0, width=2)
        for col, field in enumerate(row):
            left = x_origin + round(col * (content_width - 4) / len(row)) + 2
            right = x_origin + round((col + 1) * (content_width - 4) / len(row)) + 2
            if col:
                draw.line((left, y, left, y + row_height), fill=0, width=2)
            if current and not sheet.get('calibrationPattern') and row_no == 3:
                label_font = ImageFont.truetype(font_path, 26)
                weight_font = ImageFont.truetype(font_path, 48 if col == 2 else 44)
                if draw.textlength(field['value'], font=weight_font) > right - left - 24:
                    raise ValueError('weight exceeds cell width')
                center = (left + right) // 2
                draw.text((center, y + 10), field['label'], font=label_font, fill=0, anchor='mt')
                draw.text((center, y + 49), field['value'], font=weight_font, fill=0, anchor='mt', stroke_width=1)
                continue
            text = field['label'] + ': ' + field['value']
            lines, line = [], ''
            for char in text:
                if draw.textlength(line + char, font=normal) > right - left - 24:
                    lines.append(line)
                    line = ''
                line += char
            lines.append(line)
            if len(lines) > 2 or current and len(lines) * 34 > row_height - 4:
                raise ValueError('field exceeds two-line cell; refusing truncation')
            for number, line in enumerate(lines):
                draw.text((left + 12, y + (row_height - len(lines) * 34) // 2 + number * 34), line, font=normal, fill=0)
    if draw.textlength(sheet['title'], font=title_font) > content_width - 24:
        raise ValueError('title exceeds page width')
    if draw.textlength(marks, font=normal) > content_width - 24:
        raise ValueError('marks exceed page width')
    if (scaled_width, scaled_height) != im.size:
        im = im.resize((scaled_width, scaled_height), Image.Resampling.NEAREST)
    if sheet.get('calibrationPattern'):
        calibration_pattern(im, font_path)
    if offset_x or offset_y:
        # Positive vertical offset adds blank paper above the image (moves it
        # down); negative offset reduces the centered top margin (moves it up).
        # It must never crop the title, marks, borders, or table contents.
        positioned = Image.new('1', (im.width + offset_x, im.height + offset_y), 1)
        positioned.paste(im, (offset_x, offset_y))
        return positioned
    return im


def escp(im, sheet):
    # Effective calibrated form length, 180 dpi square pixels. 24-row bands use the same ESC * 39
    # path already verified by the onsite QR test. No font alignment assumptions.
    _, _, _, _, _, form = geometry(sheet)
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
