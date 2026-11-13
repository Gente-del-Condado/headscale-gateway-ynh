"""La firma de publicación nunca admite archivos compartidos ni rutas externas."""
import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'tools'))
from android_signing import load_environment


class AndroidSigningTest(unittest.TestCase):
    def test_private_profile_and_rejection_of_shared_credentials(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for name, text in [('key.p12', 'fixture'), ('password', 'fake-test-password'),
                               ('profile.json', json.dumps({'format': 1, 'keystore': 'key.p12',
                                                           'password_file': 'password', 'alias': 'test'}))]:
                path = root / name
                path.write_text(text)
                path.chmod(0o600)
            env = load_environment(root / 'profile.json')
            self.assertEqual(env['HGW_ANDROID_KEY_ALIAS'], 'test')
            (root / 'password').chmod(0o644)
            with self.assertRaisesRegex(ValueError, 'privado'):
                load_environment(root / 'profile.json')

    def test_profile_cannot_reference_parent_files_or_symlinks(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            profile = root / 'profile.json'
            profile.write_text(json.dumps({'format': 1, 'keystore': '../outside.p12',
                                           'password_file': 'password', 'alias': 'test'}))
            profile.chmod(0o600)
            with self.assertRaisesRegex(ValueError, 'relativos'):
                load_environment(profile)
            link = root / 'linked.json'
            link.symlink_to(profile)
            with self.assertRaisesRegex(ValueError, 'sin enlaces'):
                load_environment(link)
