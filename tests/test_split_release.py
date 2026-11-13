"""Los fragmentos de la release deben reconstruir todos los bytes originales."""
import hashlib
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'tools'))
from split_release import split


class SplitReleaseTest(unittest.TestCase):
    def test_exact_boundary_and_last_partial_chunk(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / 'release.tar'
            for index, data in enumerate((b'12345678', b'123456789')):
                source.write_bytes(data)
                output = root / str(index)
                report = split(source, output, source.name, 4)
                reconstructed = b''.join((output / p['name']).read_bytes() for p in report['parts'])
                self.assertEqual(data, reconstructed)
                self.assertEqual(report['sha256'], hashlib.sha256(data).hexdigest())
                self.assertTrue(all(p['bytes'] <= 4 for p in report['parts']))
                with self.assertRaises(ValueError):
                    split(source, output, source.name, 4)
