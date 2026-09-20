// Command ldaptestd runs a small LDAPS directory for exercising the Docker Compose
// `ldap` profile without an Authentik. It is a development tool, not something to deploy:
// its accounts and passwords come from the command line, and its certificate is made on the
// spot (a new CA and a leaf for --host), with the CA written to --ca for LDAP_CA_FILE.
//
//	ldaptestd --host ldap-outpost --ca ldap/ca.pem --user ana:senha-da-ana:ana@x.dev:uuid-ana
//
// The service account is the one of the ldaptest package: cn=svc,dc=test with the password
// svc-pass, and users live under dc=test.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"strings"
	"time"

	"github.com/ocnaibill/codice/backend/internal/ldapauth/ldaptest"
)

type users []ldaptest.User

func (u *users) String() string { return fmt.Sprint(len(*u)) }
func (u *users) Set(v string) error {
	p := strings.SplitN(v, ":", 4)
	if len(p) != 4 {
		return fmt.Errorf("want name:password:email:uuid, got %q", v)
	}
	*u = append(*u, ldaptest.User{Name: p[0], Password: p[1], Email: p[2], UUID: p[3]})
	return nil
}

func main() {
	listen := flag.String("listen", ":6636", "address to listen on (LDAPS)")
	host := flag.String("host", "ldap-outpost", "the name clients use; goes in the certificate")
	caPath := flag.String("ca", "ca.pem", "where to write the CA certificate clients must trust")
	var accounts users
	flag.Var(&accounts, "user", "name:password:email:uuid (repeatable)")
	flag.Parse()

	cert, ca, err := certificate(*host)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*caPath, ca, 0o644); err != nil {
		log.Fatal(err)
	}
	ln, err := tls.Listen("tcp", *listen, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("LDAPS test directory for %q on %s, %d accounts, CA in %s", *host, ln.Addr(), len(accounts), *caPath)
	log.Fatal(ldaptest.New(accounts...).Serve(ln))
}

// certificate makes a CA and a server certificate for host, and returns the CA in PEM.
func certificate(host string) (tls.Certificate, []byte, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ldaptestd CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(30 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	caCert, _ := x509.ParseCertificate(caDER)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(30 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), nil
}
