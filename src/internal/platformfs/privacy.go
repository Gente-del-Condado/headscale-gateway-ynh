// El paquete platformfs comprueba permisos portables de los datos privados del Gateway.
// Los bits Unix no representan las listas de control de acceso NTFS de Windows.
// En Windows, instala la aplicación en un directorio de datos protegido.
package platformfs

import (
	"os"
	"runtime"
)

func PrivateDirectory(info os.FileInfo) bool {
	return info.IsDir() && (runtime.GOOS == "windows" || info.Mode().Perm()&0077 == 0)
}
func PrivateFile(info os.FileInfo) bool {
	return info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm()&0077 == 0)
}
