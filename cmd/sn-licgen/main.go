// Command sn-licgen creates the licence signing key pair and issues
// SWISSNAND Backup licence keys. Keep the private key OFFLINE and never
// commit it to a repository.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/swissnand/snbackup/internal/license"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("sn-licgen keygen --out signing.key\nsn-licgen issue --key signing.key --customer 'Firma AG' --edition business --servers 3 --months 12")
		os.Exit(0)
	}
	switch os.Args[1] {
	case "keygen":
		fs := flag.NewFlagSet("keygen", flag.ExitOnError)
		out := fs.String("out", "signing.key", "Datei für den privaten Schlüssel")
		fs.Parse(os.Args[2:])
		if _, err := os.Stat(*out); err == nil {
			fmt.Fprintln(os.Stderr, "Datei existiert bereits – nicht überschrieben")
			os.Exit(1)
		}
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(*out, []byte(base64.StdEncoding.EncodeToString(priv)), 0o600); err != nil {
			panic(err)
		}
		fmt.Println("Privater Schlüssel gespeichert:", *out, "(GEHEIM halten, offline sichern!)")
		fmt.Println("Öffentlicher Schlüssel (als GitHub-Variable SN_LICENSE_PUBKEY hinterlegen):")
		fmt.Println(base64.StdEncoding.EncodeToString(pub))
	case "issue":
		fs := flag.NewFlagSet("issue", flag.ExitOnError)
		keyFile := fs.String("key", "signing.key", "Privater Schlüssel")
		customer := fs.String("customer", "", "Kunde (Firma)")
		email := fs.String("email", "", "E-Mail")
		edition := fs.String("edition", "business", "business | enterprise")
		servers := fs.Int("servers", 1, "Anzahl Server (0 = unbegrenzt)")
		months := fs.Int("months", 12, "Laufzeit in Monaten (0 = unbefristet)")
		fs.Parse(os.Args[2:])
		if strings.TrimSpace(*customer) == "" {
			fmt.Fprintln(os.Stderr, "--customer fehlt")
			os.Exit(1)
		}
		raw, err := os.ReadFile(*keyFile)
		if err != nil {
			panic(err)
		}
		priv, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil || len(priv) != ed25519.PrivateKeySize {
			fmt.Fprintln(os.Stderr, "ungültiger Schlüssel")
			os.Exit(1)
		}
		idb := make([]byte, 6)
		rand.Read(idb)
		l := license.License{ID: fmt.Sprintf("SNB-%X", idb), Edition: *edition, Customer: *customer, Email: *email, Servers: *servers, Issued: time.Now().UTC()}
		if *months > 0 {
			l.Expires = time.Now().UTC().AddDate(0, *months, 0)
		}
		key, err := license.Sign(l, ed25519.PrivateKey(priv))
		if err != nil {
			panic(err)
		}
		fmt.Println(key)
	}
}
