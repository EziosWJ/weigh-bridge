"""合成规则模型；不连接项目服务，不能代替集成验收。"""
import unittest


def candidates(events, now):
    groups = []
    seen = set()
    for event in sorted(events, key=lambda e: (e['received'], e['id'])):
        if event['id'] in seen:
            continue
        seen.add(event['id'])
        if (not event['plate'] or not event['enabled']
                or not 0 <= now - event['received'] < 60
                or not 0 <= event['received'] - event['recognized'] <= 60):
            continue
        group = next((g for g in reversed(groups) if g['plate'] == event['plate']
                      and event['received'] - g['last'] <= 15), None)
        if group is None:
            group = {'plate': event['plate'], 'last': event['received'], 'sources': []}
            groups.append(group)
        group['last'] = event['received']
        group['sources'].append(event['camera'])
    return sorted(groups, key=lambda g: g['last'], reverse=True)[:5]


def event(id, camera, plate, received, **kw):
    return dict(id=id, camera=camera, plate=plate, received=received,
                recognized=kw.get('recognized', received), enabled=kw.get('enabled', True),
                images=kw.get('images', True))


class Workbench:
    def __init__(self, watermark=0):
        self.watermark = watermark
        self.owner = 'empty'
        self.plate = ''
        self.locked = False
        self.focused = False

    def receive(self, id, plate):
        if id > self.watermark and self.owner == 'empty' and not self.locked and not self.focused:
            self.owner, self.plate = 'suggested', plate

    def edit(self, plate):
        self.owner, self.plate = 'manual', plate

    def next_vehicle(self, watermark):
        self.__init__(watermark)


class CandidateModelTest(unittest.TestCase):
    def test_one_and_two_cameras_and_retry(self):
        a = event  # Event IDs are server record IDs, not manufacturer IDs.
        first = a(1, 'A', 'A123', 90)
        self.assertEqual(candidates([first, first, a(2, 'B', 'A123', 100)], 100),
                         [dict(plate='A123', last=100, sources=['A', 'B'])])
        self.assertEqual(len(candidates([first], 100)), 1)

    def test_conflict_and_window(self):
        rows = [event(1, 'A', 'A123', 80), event(2, 'B', 'B123', 81),
                event(3, 'B', 'A123', 96)]
        self.assertEqual(len(candidates(rows, 100)), 3)

    def test_expiry_delayed_missing_and_disabled(self):
        rows = [event(1, 'A', 'A', 40), event(2, 'A', 'B', 100, recognized=39),
                event(3, 'A', 'C', 100, images=False), event(4, 'A', 'D', 100, enabled=False),
                event(5, 'A', 'E', 100, recognized=101)]
        self.assertEqual([g['plate'] for g in candidates(rows, 100)], ['C'])

    def test_top_five(self):
        rows = [event(i, 'A', str(i), 90 + i) for i in range(1, 8)]
        self.assertEqual([g['plate'] for g in candidates(rows, 100)], ['7', '6', '5', '4', '3'])

    def test_workbench_protection_and_next_vehicle_watermark(self):
        workbench = Workbench(10)
        workbench.receive(11, 'A123')
        workbench.receive(12, 'B123')
        self.assertEqual(workbench.plate, 'A123')
        workbench.edit('A124')
        workbench.receive(13, 'C123')
        self.assertEqual(workbench.plate, 'A124')
        workbench.next_vehicle(13)
        workbench.receive(12, 'B123')
        self.assertEqual(workbench.plate, '')
        workbench.focused = True
        workbench.receive(14, 'C123')
        self.assertEqual(workbench.plate, '')
        workbench.focused = False
        workbench.locked = True
        workbench.receive(14, 'C123')
        self.assertEqual(workbench.plate, '')
        workbench.locked = False
        workbench.receive(14, 'C123')
        self.assertEqual(workbench.plate, 'C123')


if __name__ == '__main__':
    unittest.main()
