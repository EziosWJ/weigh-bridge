import unittest
import render


def sheet():
    return {'title': '酒厂过磅单', 'marks': ['补打第3次'], 'rows': [
        [{'label': '单号', 'value': 'GB202610190001'}, {'label': '车牌', 'value': '晋A65432'}, {'label': '业务', 'value': '入厂卸货'}],
        [{'label': '供货方', 'value': '青岛粮食厂'}, {'label': '物料', 'value': '豌豆'}],
        [{'label': '毛重（吨）', 'value': '50'}, {'label': '皮重（吨）', 'value': '20'}, {'label': '净重（吨）', 'value': '30'}],
        [{'label': '首次称重', 'value': '2026-10-19 20:29:05'}, {'label': '二次称重', 'value': '2026-10-19 20:37:39'}],
        [{'label': '操作员', 'value': '管理员'}, {'label': '司机签字', 'value': '________________'}]]}


class TableTest(unittest.TestCase):
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
        data = render.escp(im)
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
