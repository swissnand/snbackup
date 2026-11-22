# Hinweise für Maintainer

## Lizenz-Signaturschlüssel (einmalig)

```bash
go run ./cmd/sn-licgen keygen --out signing.key
```

- `signing.key` ist **geheim**: offline sichern (Passwort-Manager, USB-Stick im Tresor), nie committen.
- Die ausgegebene Zeile (öffentlicher Schlüssel) auf GitHub hinterlegen: *Settings → Secrets and variables → Actions → Variables → New repository variable*, Name `SN_LICENSE_PUBKEY`.
- Builds ohne diese Variable laufen vollständig, akzeptieren aber keine Business-Lizenzschlüssel.

## Lizenz ausstellen

```bash
go run ./cmd/sn-licgen issue --key signing.key --customer "Muster AG" --edition business --servers 3 --months 12
```

Den ausgegebenen Schlüssel (`SNB1.…`) erhält der Kunde. Er trägt ihn unter *Einstellungen → Lizenz* ein.

## Release veröffentlichen

1. Version in `internal/version/version.go` und `Makefile` anpassen, committen.
2. Auf GitHub: *Releases → Draft a new release → Choose a tag* → `vX.Y.Z` (neues Tag auf `main`) → *Publish release*.
3. Der Workflow **Release** baut automatisch:
   - `snbackup_amd64.deb`, `snbackup_arm64.deb` (und versionierte Kopien)
   - `snbackup-linux-amd64.tar.gz`, `snbackup-linux-arm64.tar.gz`
   - `snbackup-windows-amd64.zip` (mit `install.ps1`)
   - `SHA256SUMS`
   - Docker-Image `ghcr.io/swissnand/snbackup:X.Y.Z` und `:latest` (amd64 + arm64)
4. Beim ersten Mal: *Packages → snbackup → Package settings → Change visibility → Public*.
