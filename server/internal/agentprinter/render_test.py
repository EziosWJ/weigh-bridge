import unittest
import hashlib
import math
import render


def sheet():
    return {'title': '酒厂过磅单', 'marks': ['补打第3次'], 'rows': [
        [{'label': '单号', 'value': 'GB202610190001'}, {'label': '车牌', 'value': '晋A65432'}, {'label': '业务', 'value': '入厂卸货'}],
        [{'label': '供货方', 'value': '青岛粮食厂'}, {'label': '物料', 'value': '豌豆'}],
        [{'label': '毛重（吨）', 'value': '50'}, {'label': '皮重（吨）', 'value': '20'}, {'label': '净重（吨）', 'value': '30'}],
        [{'label': '首次称重', 'value': '2026-10-19 20:29:05'}, {'label': '二次称重', 'value': '2026-10-19 20:37:39'}],
        [{'label': '操作员', 'value': '管理员'}, {'label': '司机签字', 'value': '________________'}]]}


def continuous_sheet():
    s = sheet()
    s.update(template='weighing-compact-v2', widthMm=241, heightMm=93)
    first = s['rows'].pop(0)
    first[0]['value'] = 'GB-NEW-9a6735f61d3b803e078773473802c348'
    s['rows'][0][1]['value'] = '联调煤炭 / 一级'
    s['rows'] = [[first[0]], first[1:]] + s['rows']
    return s


class ContinuousTableTest(unittest.TestCase):
    def test_single_section_centering_and_feed(self):
        s = continuous_sheet()
        im = render.render_image(s)
        width, sw, sh, dx, dy, form = render.geometry(s)
        self.assertEqual(width, 1240)
        self.assertLessEqual(abs(dx - (render.rounded(210 * 180 / 25.4) - sw) / 2), 1)
        self.assertLessEqual(abs(dy - (render.rounded(93 * 180 / 25.4) - sh) / 2), 1)
        self.assertEqual(im.size, (sw + dx, sh + dy))
        self.assertLessEqual(math.ceil(im.height / 24) * 48, form)
        data = render.escp(im, s)
        start = data.index(b'\x1b(C\x02\x00') + 5
        self.assertEqual(int.from_bytes(data[start:start + 2], 'little'), render.rounded(93 * 360 / 25.4))
        # Six complete rows, including the full-width long number and tall signature.
        for y in [90, 144, 210, 282, 402, 472, 572]:
            self.assertTrue(all(im.getpixel((x + dx, y + dy)) == 0 for x in range(1, sw - 1)))

    def test_offsets_translate_centered_content(self):
        s = continuous_sheet()
        base = render.render_image(s)
        s['calibration'] = {'offsetXmm': 2, 'offsetYmm': 1}
        im = render.render_image(s)
        dx, dy = render.rounded(2 * 180 / 25.4), render.rounded(180 / 25.4)
        self.assertEqual(im.crop((dx, dy, im.width, im.height)).tobytes(), base.tobytes())
        s['calibration'] = {'offsetYmm': 10}
        with self.assertRaises(ValueError):
            render.render_image(s)

    def test_negative_vertical_offset_moves_up_without_cropping(self):
        s = continuous_sheet()
        base = render.render_image(s)
        _, sw, sh, base_x, base_y, _ = render.geometry(s)
        base_content = base.crop((base_x, base_y, base_x + sw, base_y + sh))
        s['calibration'] = {'offsetYmm': -5}
        im = render.render_image(s)
        _, _, _, x, y, _ = render.geometry(s)
        self.assertLess(y, base_y)
        self.assertEqual(im.crop((x, y, x + sw, y + sh)).tobytes(), base_content.tobytes())
        s['calibration'] = {'offsetYmm': -6}
        with self.assertRaises(ValueError):
            render.render_image(s)

    def test_calibration_sample_uses_same_centered_geometry(self):
        s = continuous_sheet()
        s['calibrationPattern'] = True
        s['rows'] = [[{'label': '校准', 'value': '93 mm'}]] * 5
        im = render.render_image(s)
        _, sw, sh, dx, dy, _ = render.geometry(s)
        self.assertEqual(im.getpixel((dx + sw // 2, dy)), 0)
        self.assertEqual(im.getpixel((dx + sw // 2, dy + sh - 1)), 0)


class TableTest(unittest.TestCase):
    def test_fractional_form_length_survives_escp_encoding(self):
        s = sheet()
        s['calibration'] = {'formLengthMm': 93.13}
        im = render.render_image(s)
        data = render.escp(im, s)
        start = data.index(b'\x1b(C\x02\x00') + 5
        units = int.from_bytes(data[start:start + 2], 'little')
        self.assertEqual(units, math.floor(93.13 * 360 / 25.4 + .5))
        self.assertLessEqual(abs(units * 25.4 / 360 - 93.13), 25.4 / 720)
        self.assertNotEqual(units, round(93 * 360 / 25.4))
        self.assertTrue(data.endswith(b'\x1b2\x0c'))

    def test_scaling_preserves_full_image_then_offsets_in_physical_mm(self):
        s = sheet()
        base = render.render_image(s)
        s['calibration'] = {'printScale': 1.02, 'offsetXmm': 4,
                            'offsetYmm': 1, 'formLengthMm': 93.13}
        im = render.render_image(s)
        dx, dy = render.rounded(4 * 180 / 25.4), render.rounded(180 / 25.4)
        scaled = base.resize((render.rounded(base.width * 1.02), 612), render.Image.Resampling.NEAREST)
        self.assertEqual(im.size, (scaled.width + dx, scaled.height + dy))
        self.assertEqual(im.crop((dx, dy, im.width, im.height)).tobytes(), scaled.tobytes())

    def test_scaled_bands_and_invalid_numbers_are_rejected(self):
        for c in ({'printScale': 1.3}, {'formLengthMm': 85, 'offsetYmm': 1},
                  {'formLengthMm': 93.13, 'printScale': 1.1},
                  {'printScale': .49}, {'formLengthMm': 0},
                  {'formLengthMm': float('nan')}, {'printScale': float('inf')}):
            s = sheet()
            s['calibration'] = c
            with self.subTest(calibration=c), self.assertRaises(ValueError):
                render.render_image(s)

    def test_calibration_ruler_keeps_mm_spacing_after_scaling(self):
        s = sheet()
        s['calibration'] = {'printScale': .8}
        s['calibrationPattern'] = True
        im = render.render_image(s)
        self.assertEqual(im.getpixel((im.width // 2, 0)), 0)
        for mm in (0, 10, 20, 30):
            x = 18 + render.rounded(mm * 180 / 25.4)
            self.assertEqual(im.getpixel((x, im.height - 9)), 0)
        # Unchanged business rendering has no ruler at these positions.
        s['calibrationPattern'] = False
        plain = render.render_image(s)
        self.assertNotEqual(im.tobytes(), plain.tobytes())

    def test_confirmed_defaults_preserve_raster_and_escp(self):
        s = sheet()
        im = render.render_image(s)
        self.assertEqual(im.size, (1240, 600))
        self.assertEqual(hashlib.sha256(im.tobytes()).hexdigest(), '318a40b77b57e937dbdafae360772f45d1e388eaf72e7dfa26099a5be8a16a20')
        self.assertEqual(hashlib.sha256(render.escp(im, s)).hexdigest(), '3bb2586721e2b6541bd5a62a78ddc1a8aead508249a9180b86a7bee08d2a9e6f')

    def test_calibration_changes_output_and_refuses_short_form(self):
        base = sheet()
        tuned = sheet()
        tuned['calibration'] = {'printWidthMm': 170, 'offsetXmm': 4, 'offsetYmm': 3, 'formLengthMm': 110}
        im = render.render_image(tuned)
        self.assertEqual(im.width, round(170 * 180 / 25.4) + round(4 * 180 / 25.4))
        self.assertNotEqual(im.tobytes(), render.render_image(base).tobytes())
        self.assertNotEqual(render.escp(im, tuned), render.escp(render.render_image(base), base))
        tuned['calibration']['formLengthMm'] = 50
        with self.assertRaises(ValueError):
            render.render_image(tuned)

    def test_offsets_translate_without_squeezing_or_cropping(self):
        base = render.render_image(sheet())
        tuned = sheet()
        tuned['calibration'] = {'printWidthMm': 175, 'offsetXmm': 5,
                                'offsetYmm': 3, 'formLengthMm': 110}
        im = render.render_image(tuned)
        dx, dy = round(5 * 180 / 25.4), round(3 * 180 / 25.4)
        self.assertEqual(im.size, (base.width + dx, base.height + dy))
        self.assertEqual(im.crop((dx, dy, dx + base.width, dy + base.height)).tobytes(), base.tobytes())
        for invalid in ({'offsetXmm': -1}, {'offsetYmm': -1},
                        {'printWidthMm': 210, 'offsetXmm': 1},
                        {'offsetYmm': 1, 'formLengthMm': 86},
                        {'offsetYmm': 30, 'formLengthMm': 99}):
            tuned['calibration'] = invalid
            with self.assertRaises(ValueError):
                render.render_image(tuned)
    def test_printed_width_reserves_paper_margins(self):
        im = render.render_image(sheet())
        self.assertLessEqual(im.width / 180 * 25.4, 175, "printed table exceeds safe 175mm width")

    def test_solid_table_rules(self):
        im = render.render_image(sheet())
        for y in [84, 178, 272, 366, 460, 554]:
            self.assertTrue(all(im.getpixel((x, y)) == 0 for x in range(1, im.width - 1)))
        for y in range(84, 555):
            self.assertEqual(im.getpixel((1, y)), 0)
            self.assertEqual(im.getpixel((im.width - 2, y)), 0)
        self.assertEqual(im.getpixel((round((im.width - 4) / 3) + 2, 100)), 0)
        self.assertEqual(im.getpixel((round((im.width - 4) / 2) + 2, 200)), 0)

    def test_overflow_is_not_silently_clipped(self):
        s = sheet()
        s['rows'][0][0]['value'] = '长' * 200
        with self.assertRaises(ValueError):
            render.render_image(s)

    def test_escp_graphics_match_pixels(self):
        im = render.render_image(sheet())
        data = render.escp(im, {})
        offset = data.index(b'\x1b*')
        for y in range(0, im.height, 24):
            for x in range(0, im.width, 128):
                self.assertEqual(data[offset:offset+3], b'\x1b*\x27')
                count = data[offset+3] + 256 * data[offset+4]
                self.assertEqual(count, min(128, im.width-x))
                offset += 5
                for col in range(x, x+count):
                    for byte in range(3):
                        value = data[offset]
                        offset += 1
                        for bit in range(8):
                            row = y + byte*8+bit
                            if row < im.height:
                                self.assertEqual(bool(value & (1 << (7-bit))), im.getpixel((col,row)) == 0)
            self.assertEqual(data[offset:offset+2], b'\r\n')
            offset += 2
        self.assertEqual(data[offset:], b'\x1b2\x0c')


if __name__ == '__main__':
    unittest.main()
