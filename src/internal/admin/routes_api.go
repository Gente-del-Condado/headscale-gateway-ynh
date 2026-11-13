package admin

// Índice de módulos REST. Cada módulo mantiene sus rutas y permisos.

func (s *Server) registerAPIRoutes(mux routeRegistrar) {
	s.registerAuthRoutes(mux)
	s.registerNetworkRoutes(mux)
	s.registerGatewayPrefsRoutes(mux)
	s.registerHeadscaleManagementRoutes(mux)
	s.registerHeadscalePolicyDNSRoutes(mux)
	s.registerPolicyTemplateRoutes(mux)
	s.registerGatewayRoutes(mux)
	s.registerConfigurationTransferRoutes(mux)
	s.registerForgejoRoutes(mux)
	s.registerFirewallRoutes(mux)
	s.registerTemplateTransferRoutes(mux)
	s.registerDiagnosticsRoutes(mux)
	s.registerBackupRoutes(mux)
	s.registerPKIRoutes(mux)
	s.registerPortalRoutes(mux)
}
