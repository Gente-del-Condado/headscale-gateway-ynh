#!/bin/bash
# shellcheck disable=SC1091,SC2154,SC2034
# YunoHost inyecta los ajustes y las funciones auxiliares en el entorno del script.

hgw_check_sso() {
    local version
    version="$(dpkg-query -W -f='${Version}' ssowat)"
    dpkg --compare-versions "$version" ge 12.1.1 || ynh_die "Actualiza SSOwat a la versión 12.1.1 o posterior antes de habilitar el acceso desde el portal"
    getent group admins >/dev/null || ynh_die "El grupo admins de YunoHost no está disponible mediante NSS"
}

hgw_install_binary() {
    local architecture
    architecture="$(dpkg --print-architecture)"
    case "$architecture" in amd64|arm64) ;; *) ynh_die "Arquitectura no compatible" ;; esac
    (cd ../payload && sha256sum --check SHA256SUMS)
    install -m 0755 "../payload/headscale-gateway-linux-$architecture" "$install_dir/headscale-gateway.next"
    mv -f "$install_dir/headscale-gateway.next" "$install_dir/headscale-gateway"
    cp -a ../payload/notices "$install_dir/"
}

hgw_configure() {
    local args=(--app "$app" --install-dir "$install_dir" --data-dir "$data_dir"
        --port "$port" --lan-port "$port_lan" --path "$path"
        --proxy-include "/etc/nginx/conf.d/$domain.d/$app-proxy-auth.inc")
    python3 ../conf/configure.py "${args[@]}"
    chown root:root "/etc/nginx/conf.d/$domain.d/$app-proxy-auth.inc"
    chmod 0600 "/etc/nginx/conf.d/$domain.d/$app-proxy-auth.inc"
    chown --no-dereference "$app:$app" "$install_dir/startup.json"
    chown -R "$app:$app" "$data_dir"
    chmod 0700 "$data_dir" "$data_dir/gateway"
    if [[ -f "$data_dir/initial-admin.json" ]]; then
        chmod 0600 "$data_dir/initial-admin.json"
    fi
}

hgw_register_service() {
    yunohost service add "$app" --description="Gateway HTTPS privado para Headscale" --log="$data_dir/gateway/logs/gateway.log"
}
