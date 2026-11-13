"""Una carpeta de binarios antigua no debe convertirse en una versión nueva."""
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'tools'))
import package_release


class ReleaseFreshnessTest(unittest.TestCase):
    def test_rejects_other_version_or_changed_sources_before_packaging(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / 'bin').mkdir()
            (root / 'VERSION').write_text('1.4.7\n')
            for version, fingerprint in [('1.4.6', 'current'), ('1.4.7', 'old')]:
                (root / 'bin/build-manifest.json').write_text(json.dumps([
                    {'version': version, 'source_fingerprint': fingerprint}]))
                with patch.object(package_release, 'ROOT', root), patch.object(
                        package_release, 'source_fingerprint', return_value='current'):
                    with self.assertRaisesRegex(ValueError, 'otra versión o fuentes'):
                        package_release.runtime_archives(root / 'output', True)
                self.assertFalse((root / 'output').exists())
