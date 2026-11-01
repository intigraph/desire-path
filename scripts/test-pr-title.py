#!/usr/bin/env python3
"""Offline event/CI controls: refuse private tracker titles without a writer."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class TitleTests(unittest.TestCase):
    def test_event_boundary(self):
        for title, accepted in [
            ('fix: keep public #42', True), ('feat(cli)!: keep model-v2', True),
            ('fix: not-aegis-abc123', True), ('not conventional', False),
            ('fix: keep aegis-abc123.2suffix and aegis-abc123.label', True),
            ('fix: keep [aegis-abc123]', False), ('fix: hq-abc123.2.10', False),
            ('fix: qp-abcdefghij', False), ('fix: gassy-abc123', False),
            ('fix: line\nsecond', False), (None, False),
        ]:
            with self.subTest(title=title), tempfile.TemporaryDirectory() as tmp:
                event = Path(tmp) / 'event.json'
                event.write_text(json.dumps({'pull_request': {'title': title}}))
                env = dict(os.environ, GITHUB_EVENT_NAME='pull_request', GITHUB_EVENT_PATH=str(event))
                run = subprocess.run([sys.executable, str(ROOT / 'scripts/check-pr-title.py')],
                                     env=env, capture_output=True, text=True)
                self.assertEqual(run.returncode == 0, accepted)
                self.assertNotIn('Traceback', run.stderr)
                self.assertEqual(json.loads(event.read_text())['pull_request']['title'], title)

    def test_missing_event_refuses_and_push_skips(self):
        for event, expected in [('pull_request', 1), ('push', 0)]:
            env = dict(os.environ, GITHUB_EVENT_NAME=event, GITHUB_EVENT_PATH='/nonexistent/event.json')
            run = subprocess.run([sys.executable, str(ROOT / 'scripts/check-pr-title.py')], env=env)
            self.assertEqual(run.returncode, expected)

    def test_title_edit_runs_gate_in_existing_test_job(self):
        ci = (ROOT / '.github/workflows/ci.yml').read_text()
        self.assertIn('types: [opened, synchronize, reopened, edited, ready_for_review]', ci)
        job = ci.split('  test:', 1)[1].split('\n  repository-pendant-source:', 1)[0]
        self.assertIn('python3 scripts/check-pr-title.py', job)
        self.assertIn('python3 scripts/test-pr-title.py', job)


if __name__ == '__main__':
    unittest.main()
