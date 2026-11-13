"""Run against a real Home Assistant Core test environment, not import stubs."""
from pathlib import Path
from importlib.util import find_spec
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[2] ))

# 2026.11 ships the official helpers. Older supported releases use PHACC.
# The official package registers itself through its pytest entry point.
if find_spec('pytest_homeassistant') is None:
    pytest_plugins = ['pytest_homeassistant_custom_component']
