# Sicherheit

## Sicherheitslücken melden

Bitte **keine öffentlichen Issues** für Sicherheitslücken. Melde sie vertraulich über
GitHub → *Security* → *Report a vulnerability*. Wir antworten innerhalb von 5 Werktagen.

## Kryptografisches Design

```
Repository-Passwort ──PBKDF2-HMAC-SHA256 (600 000 It., 32 B Salt)──► KEK
KEK ──AES-256-GCM──► keys/<id>   (enthält Master-Key: enc | mac | gear-seed, je 256 Bit)

Datei ─► Content-Defined Chunking (Gear-Hash, geheimer Seed, Ø 1 MiB)
      ─► Chunk-ID = HMAC-SHA256(mac, Klartext)
      ─► Deflate-Kompression (falls kleiner)
      ─► AES-256-GCM(enc, Nonce 96 Bit zufällig, AAD = Chunk-ID) ─► data/xx/<id>
Snapshot (JSON) ─► AES-256-GCM(enc, AAD = "snapshot:"+id) ─► snapshots/<id>
```

- **Vertraulichkeit:** Der Speicherort sieht nur verschlüsselte Objekte mit zufällig wirkenden Namen. Chunk-Grenzen hängen von einem geheimen Seed ab, Chunk-IDs sind HMACs. Ein Anbieter kann daher nicht prüfen, ob eine bestimmte bekannte Datei gespeichert ist.
- **Integrität & Authentizität:** GCM-Tags, AAD-Bindung an den Objektnamen, erneute HMAC-Prüfung jedes geladenen Chunks und SHA-256 pro Datei. Manipulation, Vertauschen oder Abschneiden von Daten wird erkannt.
- **Mehrere Passwörter:** Jede Schlüsseldatei schützt denselben Master-Key. Passwortwechsel ersetzt nur die Schlüsseldateien, die Daten müssen nicht neu verschlüsselt werden.
- **Archive (.snarc):** AES-256-GCM in 1-MiB-Segmenten mit Zähler-Nonce und Endmarkierung (Schutz vor Umsortieren und Abschneiden).

## Web-Konsole & API

- Nur HTTPS (TLS ≥ 1.2, X25519/P-256). Ohne eigenes Zertifikat wird ein selbstsigniertes ECDSA-Zertifikat erzeugt. `--no-tls` ist nur auf Loopback erlaubt (Reverse-Proxy).
- Header: CSP (`script-src 'self'`, keine Inline-Skripte), HSTS, `X-Frame-Options: DENY`, `nosniff`, `Referrer-Policy: no-referrer`, COOP.
- Sitzungen: 256-Bit-Token (nur der Hash wird gespeichert), HttpOnly, Secure, SameSite=Strict, Inaktivitäts-Timeout, maximal 12 Stunden.
- CSRF: Synchronizer-Token im Header `X-CSRF-Token` für alle ändernden Anfragen.
- Anmeldung: PBKDF2-Passworthashes, konstante Laufzeit, Sperre nach 5 Fehlversuchen pro IP und Benutzer, optional erzwungene TOTP-2FA.
- Rollen: `admin`, `operator` (Jobs starten, wiederherstellen in andere Ordner), `viewer`. Wiederherstellung an den Originalort nur für Admins.
- API-Tokens: nur SHA-256-Hash gespeichert, Rolle und Ablaufdatum, `Authorization: Bearer`.
- Audit-Log aller sicherheitsrelevanten Aktionen (`audit.jsonl`).
- Optional IP-Allowlist (CIDR). Loopback bleibt immer erlaubt.

## Gespeicherte Geheimnisse

`state.json` enthält Repository-Passwörter, S3-Keys, DB- und SMTP-Passwörter **verschlüsselt**
(AES-256-GCM, Schlüssel in `secret.key`). Das Datenverzeichnis ist nur für root/SYSTEM lesbar
(`0700` bzw. ACL). Datenbank-Passwörter werden über Umgebung bzw. eine temporäre Datei mit
Rechten `0600` übergeben und erscheinen nie in der Prozessliste.

## Empfehlungen für den Betrieb

1. Repository-Passwörter zusätzlich **offline** aufbewahren (Passwort-Manager, Tresor).
2. **3-2-1-Regel:** Backup-Kopie-Job auf ein zweites Ziel ausser Haus.
3. Für Ransomware-Schutz ein S3-Ziel mit **Object Lock (Compliance)** verwenden.
4. Wöchentlicher **Integritätsprüfungs-Job** und regelmässige Test-Wiederherstellungen.
5. 2FA für alle Konten erzwingen und die Web-Konsole per IP-Allowlist oder VPN einschränken.
