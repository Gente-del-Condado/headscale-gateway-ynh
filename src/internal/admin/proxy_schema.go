package admin

// ProxyField describe una opción soportada por ProxyOptions, no una opción
// inventada por el panel. Los valores ausentes heredan los de la plantilla.
// Los mapas de cabeceras se editan mediante un control específico en la UI.
type ProxyField struct {
	Key         string         `json:"key"`
	Label       string         `json:"label"`
	Group       string         `json:"group"`
	Kind        string         `json:"kind"` // boolean, duration, integer, text, url, select
	Description string         `json:"description,omitempty"`
	Options     []ConfigOption `json:"options,omitempty"`
}

func ProxySchema() []ProxyField {
	return []ProxyField{
		{Key: "egress_mode", Label: "Salida al backend", Group: "Red del backend", Kind: "choice", Description: "Sistema: usa rutas de Fedora/tailscaled (por ejemplo 100.64.0.2). Tailnet Gateway: usa tsnet y su identidad propia; responde 502 si tsnet no está conectado. Vacío hereda plantilla o ajuste global.", Options: []ConfigOption{{Value: "system", Label: "Sistema operativo / Tailscale instalado"}, {Value: "tailnet", Label: "Tailnet del Gateway (tsnet)"}}},
		{Key: "preserve_host", Label: "Conservar cabecera Host", Group: "General", Kind: "boolean", Description: "Dejar sin definir utiliza el valor de la plantilla (por defecto, true)."},
		{Key: "timeout", Label: "Tiempo total del backend", Group: "Tiempos de espera", Kind: "duration", Description: "0s desactiva el límite de este transporte; no anula los límites del servidor HTTP."},
		{Key: "response_header_timeout", Label: "Espera de cabeceras", Group: "Tiempos de espera", Kind: "duration", Description: "0s permite esperar sin límite desde el transporte."},
		{Key: "dial_timeout", Label: "Establecimiento de conexión", Group: "Tiempos de espera", Kind: "duration"},
		{Key: "tls_handshake_timeout", Label: "Negociación TLS con backend", Group: "Tiempos de espera", Kind: "duration"},
		{Key: "idle_conn_timeout", Label: "Conexión inactiva", Group: "Tiempos de espera", Kind: "duration"},
		{Key: "max_idle_conns", Label: "Conexiones inactivas totales", Group: "Conexiones", Kind: "integer"},
		{Key: "max_idle_conns_per_host", Label: "Conexiones inactivas por host", Group: "Conexiones", Kind: "integer"},
		{Key: "max_connections", Label: "Conexiones simultáneas", Group: "Conexiones", Kind: "integer", Description: "0 significa sin límite para este servicio."},
		{Key: "tls.insecure_skip_verify", Label: "Omitir verificación del certificado", Group: "TLS del backend", Kind: "boolean", Description: "Inseguro: solo para pruebas o backends bajo tu control. No afecta al TLS público."},
		{Key: "tls.server_name", Label: "Nombre TLS (SNI)", Group: "TLS del backend", Kind: "text", Description: "Nombre esperado en el certificado del backend. Vacío: se hereda de la plantilla."},
		{Key: "redirect_to", Label: "URL de redirección", Group: "Redirección", Kind: "url", Description: "Redirección fija, opcional. Debe comenzar por http:// o https://."},
		{Key: "redirect_status", Label: "Código de redirección", Group: "Redirección", Kind: "select", Options: []ConfigOption{{Value: "0", Label: "0 · Predeterminado"}, {Value: "301", Label: "301 · Permanente"}, {Value: "302", Label: "302 · Temporal"}, {Value: "307", Label: "307 · Temporal (mantiene método)"}, {Value: "308", Label: "308 · Permanente (mantiene método)"}}},
	}
}
