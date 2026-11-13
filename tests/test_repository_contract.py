"""Release and installer contracts for the single Gateway application."""
import json
from pathlib import Path
import re
import tomllib
import unittest

ROOT = Path(__file__).resolve().parents[1]


class RepositoryContractTest(unittest.TestCase):
    def test_yunohost_install_urls_have_required_suffix_and_match_publishers(self):
        # YunoHost rechaza las URL HTTPS cuyo repositorio no termina en _ynh.
        # Contrato de APP_REPO_URL en YunoHost/yunohost, src/utils/app_utils.py.
        readme = (ROOT / 'packaging/yunohost/README.md').read_text()
        urls = re.findall(r'yunohost app (?:install|upgrade headscale_gateway (?:-u|--url)) (https://github\.com/Gente-del-Condado/[^\s)]+)', readme)
        self.assertTrue(urls)
        for url in urls:
            self.assertRegex(url, r'^https://github\.com/Gente-del-Condado/[a-zA-Z0-9_.-]+_ynh$')
        repositories = {url.rsplit('/', 1)[1] for url in urls}
        self.assertEqual(len(repositories), 1)
        repository = repositories.pop()
        package_flow = (ROOT / '.github/workflows/publish-yunohost.yml').read_text()
        docs_flow = (ROOT / '.github/workflows/pages.yml').read_text()
        self.assertIn('wolfbelmi88/' + repository + '.git package', package_flow)
        self.assertIn('repository: ' + repository + '\n', docs_flow)

    def test_one_release_version_for_all_packages(self):
        version = (ROOT / 'VERSION').read_text().strip()
        manifest = json.loads((ROOT / 'custom_components/headscale_gateway/manifest.json').read_text())
        self.assertEqual(manifest['version'], version)
        app_config = (ROOT / 'packaging/homeassistant/config.yaml.in').read_text()
        self.assertEqual(re.search(r'^version: (.+)$', app_config, re.M)[1], version)
        self.assertRegex(app_config, r'(?m)^panel_admin: true$')
        ynh = tomllib.loads((ROOT / 'packaging/yunohost/manifest.toml').read_text())
        self.assertEqual(ynh['version'].split('~')[0], version)

    def test_hacs_uses_standard_repository_layout(self):
        self.assertTrue((ROOT / 'hacs.json').is_file())
        manifest = json.loads((ROOT / 'custom_components/headscale_gateway/manifest.json').read_text())
        self.assertEqual(manifest['domain'], 'headscale_gateway')
        self.assertFalse((ROOT / 'packaging/hacs/custom_components').exists())

    def test_homeassistant_open_web_ui_keeps_ingress_mount(self):
        config = (ROOT / 'packaging/homeassistant/config.yaml.in').read_text()
        entry = re.search(r'^ingress_entry: (.+)$', config, re.M)[1]
        # Supervisor app.ingress_url concatenates the entry after a trailing
        # slash. An absolute entry produces //admin/ and ServeMux redirects
        # to /admin/, losing the Home Assistant ingress mount.
        url = '/api/hassio_ingress/test/' + entry
        self.assertEqual(url, '/api/hassio_ingress/test/admin/')
