package admin

import (
	"crypto/tls"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// AuthenticateLDAP: TLS obligatorio, búsqueda uid con escape LDAP y bind real
// del usuario. Solo usuarios explícitamente autorizados son administradores.
func AuthenticateLDAP(cfg LDAPSettings, username, password string) error {
	_, err := LDAPGroups(cfg, username, password)
	return err
}
func LDAPGroups(cfg LDAPSettings, username, password string) ([]string, error) {
	if username == "" || password == "" || (len(cfg.AllowedUsers) > 0 && !containsIdentity(cfg.AllowedUsers, username)) {
		return nil, errors.New("credenciales incorrectas")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil || u.Scheme != "ldaps" {
		return nil, errors.New("URL LDAP insegura")
	}
	host := u.Hostname()
	conn, err := ldap.DialURL(cfg.URL, ldap.DialWithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}))
	if err != nil {
		return nil, errors.New("no se pudo conectar con LDAP TLS")
	}
	defer conn.Close()
	conn.SetTimeout(8 * time.Second)
	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			return nil, errors.New("autenticación de cuenta LDAP de consulta fallida")
		}
	}
	query := ldap.NewSearchRequest(cfg.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 8, false,
		"(&(objectClass=person)(uid="+ldap.EscapeFilter(strings.TrimSpace(username))+"))", []string{"dn"}, nil)
	res, err := conn.Search(query)
	if err != nil || len(res.Entries) != 1 {
		return nil, errors.New("credenciales incorrectas")
	}
	if err := conn.Bind(res.Entries[0].DN, password); err != nil {
		return nil, errors.New("credenciales incorrectas")
	}
	// La autorización por grupos nunca se fía de un valor proporcionado por el usuario.
	// Consultamos LDAP con el bind del usuario ya verificado.
	groups := []string{}
	groupQuery := ldap.NewSearchRequest(cfg.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 8, false,
		"(|(member="+ldap.EscapeFilter(res.Entries[0].DN)+")(uniqueMember="+ldap.EscapeFilter(res.Entries[0].DN)+")(memberUid="+ldap.EscapeFilter(username)+"))",
		[]string{"cn"}, nil)
	matches, e := conn.Search(groupQuery)
	if e != nil && (len(cfg.AllowedGroups) > 0) {
		return nil, errors.New("consulta de grupos LDAP fallida")
	}
	if e == nil {
		for _, entry := range matches.Entries {
			if name := entry.GetAttributeValue("cn"); name != "" {
				groups = append(groups, name)
			}
		}
	}
	if len(cfg.AllowedGroups) > 0 {
		allowed := false
		for _, g := range groups {
			if containsIdentity(cfg.AllowedGroups, g) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, errors.New("grupo LDAP no autorizado")
		}
	}
	return groups, nil
}
