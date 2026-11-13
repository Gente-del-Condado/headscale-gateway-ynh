package main

// BackupDataDir expone el data_dir activo para validarlo. Rechaza restauraciones
// completas si data_dir y SQLite están en rutas no relacionadas, para evitar
// restaurar silenciosamente en un destino equivocado.
func (a *adminController) BackupDataDir() string {
	if a.runtime == nil {
		return ""
	}
	return a.runtime.dataDir
}
