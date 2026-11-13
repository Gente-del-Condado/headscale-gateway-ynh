# Disposable integration fixture, not the YunoHost installation runtime.
FROM python:3.12-slim-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends nginx libnginx-mod-http-lua \
    && rm -rf /var/lib/apt/lists/*
COPY payload/headscale-gateway-linux-amd64 /usr/local/bin/headscale-gateway
COPY conf/configure.py conf/hgw_bootstrap.py conf/nginx.conf /test-package/conf/
COPY manifest.toml /test-package/manifest.toml
ENTRYPOINT ["python3", "/check_yunohost_sso.py"]
