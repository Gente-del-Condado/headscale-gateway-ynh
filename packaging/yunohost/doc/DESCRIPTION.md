Headscale Gateway provides private HTTPS access to applications and services on
your server or local network through an existing Headscale network. It combines
a VPN node with a reverse proxy, DNS and certificate management, access settings,
configuration profiles and encrypted backups in one web interface.

The YunoHost package integrates administrator access with the portal's single
sign-on and supports multiple instances, upgrades, URL changes, backups and
restoration. It does not open an additional public inbound port.

An existing Headscale coordination server is required. The portal's HTTPS
domain and the private domain used by the Gateway are independent settings.

The original code is licensed under AGPL-3.0-only. The public package contains
the complete server build sources and Go dependencies in `payload/source.tar.gz`,
with build instructions, checksums and third-party notices.
