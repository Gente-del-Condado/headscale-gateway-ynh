package main

import "github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"

	"math/big"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type localCA struct {
	CertificatePath string
	KeyPath         string
}

type localCertificate struct {
	CertificatePath string
	KeyPath         string
	Certificate     *x509.Certificate
	TLSCertificate  *tls.Certificate
}

// localCADirectoryName conserva el FQDN completo para evitar colisiones
// entre dominios distintos. Los directorios heredados con guiones no se
// reutilizan automáticamente: deben migrarse de forma explícita.
func localCADirectoryName(nodeDomain string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(nodeDomain)), ".")
}

func newLocalCA(
	dataDir string,
	nodeDomain string,
) localCA {
	dir := filepath.Join(
		dataDir,
		"local-ca",
		localCADirectoryName(nodeDomain),
	)

	return localCA{
		CertificatePath: filepath.Join(
			dir,
			"ca.crt",
		),
		KeyPath: filepath.Join(
			dir,
			"ca.key",
		),
	}
}

func newLocalCertificate(
	dataDir string,
	nodeDomain string,
) localCertificate {
	dir := filepath.Join(
		dataDir,
		"local-ca",
		localCADirectoryName(nodeDomain),
	)

	return localCertificate{
		CertificatePath: filepath.Join(
			dir,
			"certificate.crt",
		),
		KeyPath: filepath.Join(
			dir,
			"certificate.key",
		),
	}
}

func ensureLocalCA(
	ctx context.Context,
	cfg Config,
	ca localCA,
) (*x509.Certificate, *ecdsa.PrivateKey, error) {

	_ = ctx

	certExists := fileExists(ca.CertificatePath)
	keyExists := fileExists(ca.KeyPath)

	switch {
	case certExists && keyExists:
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-001", "CA local encontrada: %s", ca.CertificatePath)

		return loadLocalCA(ca)

	case certExists && !keyExists:
		return nil, nil, fmt.Errorf(
			"existe la CA local %s pero falta su clave privada %s",
			ca.CertificatePath,
			ca.KeyPath,
		)

	case !certExists && keyExists:
		return nil, nil, fmt.Errorf(
			"existe la clave privada de la CA %s pero falta el certificado %s",
			ca.KeyPath,
			ca.CertificatePath,
		)
	}

	if err := os.MkdirAll(
		filepath.Dir(ca.CertificatePath),
		0700,
	); err != nil {
		return nil, nil, fmt.Errorf(
			"creando directorio de CA local: %w",
			err,
		)
	}
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-002", "CA local no encontrada; generando nueva CA...")

	privateKey, err := ecdsa.GenerateKey(
		elliptic.P256(),
		rand.Reader,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"generando clave de CA: %w",
			err,
		)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, nil, fmt.Errorf(
			"generando número de serie de la CA: %w",
			err,
		)
	}

	now := time.Now()

	template := &x509.Certificate{
		SerialNumber: serial,

		Subject: pkix.Name{
			CommonName: "Headscale Gateway Local CA",
		},

		NotBefore: now.Add(-5 * time.Minute),
		NotAfter:  now.Add(cfg.LocalCAValidity),

		KeyUsage: x509.KeyUsageCertSign |
			x509.KeyUsageCRLSign |
			x509.KeyUsageDigitalSignature,

		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	der, err := x509.CreateCertificate(
		rand.Reader,
		template,
		template,
		&privateKey.PublicKey,
		privateKey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"creando certificado CA: %w",
			err,
		)
	}

	if err := writePEM(
		ca.CertificatePath,
		"CERTIFICATE",
		der,
		0644,
	); err != nil {
		return nil, nil, fmt.Errorf(
			"guardando certificado CA: %w",
			err,
		)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(
		privateKey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"serializando clave CA: %w",
			err,
		)
	}

	if err := writePEM(
		ca.KeyPath,
		"PRIVATE KEY",
		keyDER,
		0600,
	); err != nil {
		return nil, nil, fmt.Errorf(
			"guardando clave CA: %w",
			err,
		)
	}

	returnCert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"parseando CA recién creada: %w",
			err,
		)
	}
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-003", "CA local creada: %s", ca.CertificatePath)
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-004", "CA local válida hasta: %s", returnCert.NotAfter.Format(time.RFC3339))

	return returnCert, privateKey, nil
}

func loadLocalCA(
	ca localCA,
) (*x509.Certificate, *ecdsa.PrivateKey, error) {

	certPEM, err := os.ReadFile(
		ca.CertificatePath,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"leyendo CA local: %w",
			err,
		)
	}

	certBlock, _ := pem.Decode(certPEM)

	if certBlock == nil ||
		certBlock.Type != "CERTIFICATE" {
		return nil, nil, fmt.Errorf(
			"CA local inválida: %s",
			ca.CertificatePath,
		)
	}

	cert, err := x509.ParseCertificate(
		certBlock.Bytes,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"parseando CA local: %w",
			err,
		)
	}

	if !cert.IsCA {
		return nil, nil, fmt.Errorf(
			"el certificado local no es una CA: %s",
			ca.CertificatePath,
		)
	}

	if cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, nil, fmt.Errorf(
			"la CA local no tiene permiso para firmar certificados",
		)
	}

	now := time.Now()

	if now.Before(cert.NotBefore) {
		return nil, nil, fmt.Errorf(
			"la CA local todavía no es válida hasta %s",
			cert.NotBefore.Format(time.RFC3339),
		)
	}

	if now.After(cert.NotAfter) {
		return nil, nil, fmt.Errorf(
			"la CA local está caducada desde %s",
			cert.NotAfter.Format(time.RFC3339),
		)
	}

	keyPEM, err := os.ReadFile(
		ca.KeyPath,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"leyendo clave de CA local: %w",
			err,
		)
	}

	keyBlock, _ := pem.Decode(keyPEM)

	if keyBlock == nil ||
		keyBlock.Type != "PRIVATE KEY" {
		return nil, nil, fmt.Errorf(
			"clave CA local inválida: %s",
			ca.KeyPath,
		)
	}

	keyAny, err := x509.ParsePKCS8PrivateKey(
		keyBlock.Bytes,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"parseando clave CA local: %w",
			err,
		)
	}

	privateKey, ok := keyAny.(*ecdsa.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf(
			"la clave CA local no es ECDSA",
		)
	}

	publicKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf(
			"la clave pública de la CA local no es ECDSA",
		)
	}

	if !privateKey.PublicKey.Equal(publicKey) {
		return nil, nil, fmt.Errorf(
			"la clave privada de la CA no corresponde al certificado CA",
		)
	}
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-005", "CA local cargada correctamente: %s", ca.CertificatePath)
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-006", "CA local válida hasta: %s", cert.NotAfter.Format(time.RFC3339))

	return cert, privateKey, nil
}

func ensureLocalCertificate(
	ctx context.Context,
	cfg Config,
	caCert *x509.Certificate,
	caKey *ecdsa.PrivateKey,
	nodeDomain string,
	localCert localCertificate,
) (*tls.Certificate, *x509.Certificate, error) {

	_ = ctx

	certExists := fileExists(
		localCert.CertificatePath,
	)

	keyExists := fileExists(
		localCert.KeyPath,
	)

	switch {
	case certExists && keyExists:

		certPEM, err := os.ReadFile(
			localCert.CertificatePath,
		)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"leyendo certificado local: %w",
				err,
			)
		}

		keyPEM, err := os.ReadFile(
			localCert.KeyPath,
		)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"leyendo clave del certificado local: %w",
				err,
			)
		}

		cert, valid, reason, err := validateLocalCertificate(
			certPEM,
			keyPEM,
			caCert,
			nodeDomain,
			cfg.CertificateRenewBefore,
		)
		if err != nil {
			return nil, nil, err
		}

		if valid {
			for _, ip := range localCertificateIPs(cfg) {
				if cert.VerifyHostname(ip.String()) != nil {
					valid = false
					reason = "faltan direcciones IP del Gateway en SAN"
					break
				}
			}
			if cert.VerifyHostname("localhost") != nil {
				valid = false
				reason = "falta localhost en SAN"
			}
		}
		if valid {
			tlsCert, err := tls.X509KeyPair(
				certPEM,
				keyPEM,
			)
			if err != nil {
				return nil, nil, fmt.Errorf(
					"cargando certificado TLS local: %w",
					err,
				)
			}
			diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-007", "Certificado local válido: %s", localCert.CertificatePath)
			diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-008", "Certificado local válido hasta: %s", cert.NotAfter.Format(time.RFC3339))

			if reason != "" {
				diagnostics.Emit("gateway", diagnostics.Warn, "HGW-LOCALCA-009", "%s", reason)
			}

			return &tlsCert, cert, nil
		}
		diagnostics.Emit("gateway", diagnostics.Warn, "HGW-LOCALCA-010", "Certificado local no utilizable: %s", reason)
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-011", "Generando un nuevo certificado local firmado por la CA existente...")

	case certExists && !keyExists:
		diagnostics.Emit("gateway", diagnostics.Warn, "HGW-LOCALCA-012", "existe %s pero falta %s", localCert.CertificatePath,
			localCert.KeyPath,
		)
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-013", "Generando un nuevo certificado local...")

	case !certExists && keyExists:
		diagnostics.Emit("gateway", diagnostics.Warn, "HGW-LOCALCA-014", "existe %s pero falta %s", localCert.KeyPath,
			localCert.CertificatePath,
		)
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-015", "Generando un nuevo certificado local...")

	default:
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-016", "Certificado local no encontrado; generando uno nuevo...")
	}

	return generateLocalCertificate(
		cfg,
		caCert,
		caKey,
		nodeDomain,
		localCert.CertificatePath,
		localCert.KeyPath,
	)
}

func validateLocalCertificate(
	certPEM []byte,
	keyPEM []byte,
	caCert *x509.Certificate,
	nodeDomain string,
	renewBefore time.Duration,
) (*x509.Certificate, bool, string, error) {

	certBlock, _ := pem.Decode(certPEM)

	if certBlock == nil ||
		certBlock.Type != "CERTIFICATE" {
		return nil, false,
			"el certificado local no contiene un bloque PEM CERTIFICATE",
			nil
	}

	cert, err := x509.ParseCertificate(
		certBlock.Bytes,
	)
	if err != nil {
		return nil, false,
			"el certificado local no se puede analizar",
			fmt.Errorf(
				"parseando certificado local: %w",
				err,
			)
	}

	now := time.Now()

	if now.Before(cert.NotBefore) {
		return cert, false,
			fmt.Sprintf(
				"el certificado local todavía no es válido hasta %s",
				cert.NotBefore.Format(time.RFC3339),
			),
			nil
	}

	if now.After(cert.NotAfter) {
		return cert, false,
			fmt.Sprintf(
				"el certificado local está caducado desde %s",
				cert.NotAfter.Format(time.RFC3339),
			),
			nil
	}

	expectedNames := map[string]bool{
		nodeDomain:        false,
		"*." + nodeDomain: false,
	}

	for _, name := range cert.DNSNames {
		if _, ok := expectedNames[name]; ok {
			expectedNames[name] = true
		}
	}

	for name, found := range expectedNames {
		if !found {
			return cert, false,
				fmt.Sprintf(
					"el certificado local no contiene el SAN requerido %s",
					name,
				),
				nil
		}
	}

	if cert.ExtKeyUsage != nil {
		serverAuth := false

		for _, usage := range cert.ExtKeyUsage {
			if usage == x509.ExtKeyUsageServerAuth ||
				usage == x509.ExtKeyUsageAny {
				serverAuth = true
				break
			}
		}

		if !serverAuth {
			return cert, false,
				"el certificado local no permite ServerAuth",
				nil
		}
	}

	if !publicKeysEqual(
		cert.PublicKey,
		keyPEM,
	) {
		return cert, false,
			"la clave privada no corresponde al certificado local",
			nil
	}

	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	intermediates := x509.NewCertPool()

	intermediates.AddCert(
		caCert,
	)

	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		DNSName:       nodeDomain,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	if _, err := cert.Verify(opts); err != nil {
		return cert, false,
			"el certificado local no está firmado correctamente por la CA local",
			nil
	}

	if renewBefore > 0 {
		renewAt := cert.NotAfter.Add(
			-renewBefore,
		)

		if now.After(renewAt) {
			return cert, false,
				fmt.Sprintf(
					"el certificado local entra en renovación: caduca %s",
					cert.NotAfter.Format(time.RFC3339),
				),
				nil
		}
	}

	return cert, true, "", nil
}

func publicKeysEqual(
	certPublicKey any,
	keyPEM []byte,
) bool {

	keyBlock, _ := pem.Decode(keyPEM)

	if keyBlock == nil ||
		keyBlock.Type != "PRIVATE KEY" {
		return false
	}

	keyAny, err := x509.ParsePKCS8PrivateKey(
		keyBlock.Bytes,
	)
	if err != nil {
		return false
	}

	privateKey, ok := keyAny.(*ecdsa.PrivateKey)
	if !ok {
		return false
	}

	publicKey, ok := certPublicKey.(*ecdsa.PublicKey)
	if !ok {
		return false
	}

	return privateKey.PublicKey.Equal(
		publicKey,
	)
}

func generateLocalCertificate(
	cfg Config,
	caCert *x509.Certificate,
	caKey *ecdsa.PrivateKey,
	nodeDomain string,
	certPath string,
	keyPath string,
) (*tls.Certificate, *x509.Certificate, error) {

	privateKey, err := ecdsa.GenerateKey(
		elliptic.P256(),
		rand.Reader,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"generando clave del certificado local: %w",
			err,
		)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, nil, fmt.Errorf(
			"generando número de serie del certificado local: %w",
			err,
		)
	}

	now := time.Now()

	template := &x509.Certificate{
		SerialNumber: serial,

		Subject: pkix.Name{
			CommonName: nodeDomain,
		},

		NotBefore: now.Add(-5 * time.Minute),
		NotAfter:  now.Add(cfg.LocalCertificateValidity),

		KeyUsage: x509.KeyUsageDigitalSignature |
			x509.KeyUsageKeyEncipherment,

		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},

		IPAddresses: localCertificateIPs(cfg),
		DNSNames: []string{
			"localhost",
			nodeDomain,
			"*." + nodeDomain,
		},

		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	der, err := x509.CreateCertificate(
		rand.Reader,
		template,
		caCert,
		&privateKey.PublicKey,
		caKey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"creando certificado local: %w",
			err,
		)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(
		privateKey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"serializando clave del certificado local: %w",
			err,
		)
	}

	if err := os.MkdirAll(
		filepath.Dir(certPath),
		0700,
	); err != nil {
		return nil, nil, fmt.Errorf(
			"creando directorio del certificado local: %w",
			err,
		)
	}

	if err := writePEM(
		certPath,
		"CERTIFICATE",
		der,
		0644,
	); err != nil {
		return nil, nil, fmt.Errorf(
			"guardando certificado local: %w",
			err,
		)
	}

	if err := writePEM(
		keyPath,
		"PRIVATE KEY",
		keyDER,
		0600,
	); err != nil {
		return nil, nil, fmt.Errorf(
			"guardando clave del certificado local: %w",
			err,
		)
	}

	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"parseando certificado local recién creado: %w",
			err,
		)
	}

	certificatePEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: der,
		},
	)

	caPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: caCert.Raw,
		},
	)

	fullChainPEM := append(
		certificatePEM,
		caPEM...,
	)

	keyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: keyDER,
		},
	)

	tlsCert, err := tls.X509KeyPair(
		fullChainPEM,
		keyPEM,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"creando certificado TLS local: %w",
			err,
		)
	}
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-017", "Certificado local generado para %s", nodeDomain)
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-LOCALCA-018", "Certificado local válido hasta: %s", leaf.NotAfter.Format(time.RFC3339))

	return &tlsCert, leaf, nil
}

func randomSerial() (*big.Int, error) {
	max := new(big.Int).Lsh(
		big.NewInt(1),
		128,
	)

	serial, err := rand.Int(
		rand.Reader,
		max,
	)
	if err != nil {
		return nil, err
	}

	if serial.Sign() == 0 {
		serial.SetInt64(1)
	}

	return serial, nil
}

func writePEM(
	path string,
	blockType string,
	data []byte,
	mode os.FileMode,
) error {

	file, err := os.OpenFile(
		path,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		mode,
	)
	if err != nil {
		return fmt.Errorf(
			"creando %s: %w",
			path,
			err,
		)
	}

	defer file.Close()

	if err := pem.Encode(
		file,
		&pem.Block{
			Type:  blockType,
			Bytes: data,
		},
	); err != nil {
		return fmt.Errorf(
			"escribiendo %s: %w",
			path,
			err,
		)
	}

	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

// IP SANs identify a host, independent of its HTTPS port.
func localCertificateIPs(cfg Config) []net.IP {
	values := append([]string{"127.0.0.1", "::1"}, cfg.LocalCertificateIPs...)
	if host, _, err := net.SplitHostPort(cfg.LocalListenAddr); err == nil {
		values = append(values, host)
	}
	seen := map[string]net.IP{}
	for _, value := range values {
		ip := net.ParseIP(value)
		if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() {
			continue
		}
		seen[ip.String()] = ip
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]net.IP, 0, len(keys))
	for _, key := range keys {
		result = append(result, seen[key])
	}
	return result
}
