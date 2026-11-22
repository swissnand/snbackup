# SWISSNAND Backup – Handbuch

## Inhalt
1. [Begriffe](#begriffe)
2. [Erste Schritte](#erste-schritte)
3. [Zentraler Backup-Server und Agenten](#zentraler-backup-server-und-agenten)
4. [VMs, Disk-Images und Bare Metal](#vms-disk-images-und-bare-metal)
5. [Docker und Kubernetes](#docker-und-kubernetes)
3. [Backup-Varianten](#backup-varianten)
4. [Zeitpläne](#zeitpläne)
5. [Aufbewahrung (GFS)](#aufbewahrung-gfs)
6. [Ziele](#ziele)
7. [Wiederherstellung](#wiederherstellung)
8. [Kommandozeile](#kommandozeile)
9. [REST-API](#rest-api)
10. [Dateien & Verzeichnisse](#dateien--verzeichnisse)
11. [Fehlerbehebung](#fehlerbehebung)

## Begriffe

| Begriff | Bedeutung |
|---|---|
| **Ziel / Repository** | Verschlüsselter Speicherort für Backups (Ordner, NAS, S3-Bucket, WebDAV, rclone) |
| **Job** | Was, wohin und wann gesichert wird |
| **Snapshot** | Vollständiger Wiederherstellungspunkt eines Backup-Laufs |
| **Chunk** | Verschlüsseltes Datenstück (Ø 1 MiB). Gleiche Daten werden nur einmal gespeichert |
| **Prune** | Löscht Chunks, die von keinem Snapshot mehr gebraucht werden |

## Erste Schritte

1. **Web-Konsole öffnen:** `https://<server>:8550`, Anmeldung als `admin` mit dem Startpasswort, danach eigenes Passwort setzen.
2. **Ziel hinzufügen:** *Backup-Ziele → Ziel hinzufügen*. Typ wählen, Verbindung testen, ein starkes Verschlüsselungspasswort generieren und **sicher notieren**.
3. **Job erstellen:** *Backup-Jobs → Neuer Job*. Der Assistent führt durch Typ, Quelle, Ziel, Zeitplan, Aufbewahrung, Optionen und Benachrichtigung.
4. **Testen:** Job mit *Start* ausführen, unter *Aktivität* das Protokoll ansehen und unter *Wiederherstellen* eine Datei herunterladen.
5. **Absichern:** *Einstellungen → Sicherheit* → 2FA einrichten, E-Mail-Benachrichtigungen konfigurieren.

## Zentraler Backup-Server und Agenten

Ein Backup-Server steuert beliebig viele Rechner. Auf jedem Rechner läuft ein **Agent**: dasselbe Programm `snbackup` im Agent-Modus.

```
Agent ──HTTPS (ausgehend, alle 5 s)──► Backup-Server :8550
  ◄── Auftrag: Job + Zugangsdaten zum Ziel
  ──► Phase, Fortschritt, Protokoll, Ergebnis
Agent ──Daten (verschlüsselt)──► Ziel (NAS, S3, WebDAV …)
```

- **Keine offenen Ports auf den Agenten.** Nur der Backup-Server ist auf Port 8550 erreichbar.
- **Die Daten gehen direkt vom Agent ins Ziel**, nicht über den Backup-Server. Das Ziel muss deshalb vom Agent **und** vom Server aus erreichbar sein (NAS, S3, WebDAV), damit der Server die Snapshots durchsuchen kann.
- **Registrierung:** Einmal-Token (oder mehrfach verwendbar für Docker/Kubernetes) und der SHA-256-Fingerabdruck des Server-Zertifikats. Jeder Agent bekommt ein eigenes Geheimnis und kann einzeln gesperrt oder entfernt werden.
- **Zeitpläne, Verlauf, Aufbewahrung und Benachrichtigungen** bleiben auf dem Backup-Server. Ist ein Agent offline, wartet der Auftrag bis zu 30 Minuten und schlägt dann mit «Agent offline» fehl.
- **Wiederherstellung:** Im Dialog den Zielrechner wählen. Vorausgewählt ist der Rechner, von dem das Backup stammt.

**Mit oder ohne Agent?**

| Situation | Empfehlung |
|---|---|
| Linux-/Windows-Server mit lokalen Daten | Agent |
| Windows mit geöffneten Dateien (VSS), Hyper-V, Bare Metal | Agent (Pflicht) |
| Proxmox-Host, Disk-Images | Agent auf dem Host |
| NAS-Freigaben (NFS/SMB) | agentenlos: Freigabe auf dem Backup-Server mounten |
| Datenbanken | agentenlos übers Netz (Host im Job) oder Agent auf dem DB-Server |

## VMs, Disk-Images und Bare Metal

| Typ | Ablauf | Wiederherstellung |
|---|---|---|
| **Proxmox VE** | `vzdump --stdout --compress 0` pro VM/Container, direkt in den Stream. Unkomprimiert, damit die Deduplizierung greift | `snbackup dump <snap> _streams/proxmox/vzdump-qemu-101.vma \| qmrestore - 101`; Container: Datei speichern, `pct restore` |
| **Hyper-V** | `Export-VM` mit Produktions-Checkpoint in einen festen Zwischenordner, danach Backup und Aufräumen | Ordner wiederherstellen, in Hyper-V «Virtuellen Computer importieren» |
| **Disk-Image (Linux)** | Block-Geräte oder Image-Dateien, optional per LVM-Snapshot (`lvcreate -s`, danach `lvremove`) | Aus einem Rettungssystem: `snbackup dump <snap> _streams/image/dev-sda.img > /dev/sda` |
| **Windows Bare Metal** | `wbadmin start backup -allCritical` in einen Zwischenspeicher (Volume oder Freigabe), dann ins Repository | Abbild zurückholen, vom Windows-Setup starten → «Systemimage-Wiederherstellung» |

## Docker und Kubernetes

**Image:** `ghcr.io/swissnand/snbackup` (amd64/arm64). Es enthält pg_dump, mysqldump, rclone und die Docker-CLI.

- **Server:** Daten unter `/data`, Port 8550 (HTTPS), Health-Check `GET /healthz`.
- **Agent im Container:** `agent run` mit den Variablen `SNB_SERVER`, `SNB_ENROLL_TOKEN`, `SNB_FINGERPRINT` und `SNB_HOST_ROOT=/host`. Das Host-Dateisystem wird nach `/host` gemountet. Gespeichert werden die Originalpfade (`/etc`, nicht `/host/etc`). Für Wiederherstellungen den Mount schreibbar machen.
- **Kubernetes:** `deploy/kubernetes/server.yaml` (Deployment, PVC, Service, optional Ingress mit `backend-protocol: HTTPS`) und `agent-daemonset.yaml` (ein Agent pro Node, Name = Node-Name, Identität unter `/var/lib/snbackup-agent` auf dem Node).
- **Ziele im Container** sollten Netzwerkziele sein (S3, WebDAV, NFS-Volume). Ein lokaler Pfad muss in den Container gemountet sein.

## Backup-Varianten

In allen drei Modi werden nur neue Datenblöcke gespeichert, und jeder Snapshot lässt sich allein vollständig wiederherstellen. Der Modus bestimmt, **welche Dateien gelesen und geprüft** werden. Das beeinflusst Laufzeit, Last auf der Quelle und wie früh stille Datenfehler auffallen.

| Variante | Beschreibung | Typischer Einsatz |
|---|---|---|
| **Modus Vollbackup** | Alle Dateien werden gelesen und geprüft | Wöchentlich oder monatlich |
| **Modus inkrementell** | Nur Dateien, die seit dem **letzten Lauf** geändert wurden (Grösse, Änderungszeit, Inode) | Täglich, stündlich |
| **Modus differenziell** | Alle Dateien, die seit dem **letzten Vollbackup** geändert wurden. Gibt es noch keines, wird zuerst eines erstellt | Täglich, kombiniert mit wöchentlichem Vollbackup |
| **Datei-Backup, forever-incremental** | Nur geänderte Dateien werden gelesen (Grösse, Änderungszeit, Inode). Jeder Snapshot ist trotzdem vollständig | Standard für Server |
| **Aktives Vollbackup** | Alle Dateien werden neu gelesen und geprüft (Option *Modus* oder *Vollbackup*-Knopf) | Periodisch, z. B. sonntags |
| **Periodisches Vollbackup** | Automatisch wöchentlich oder monatlich ein aktives Vollbackup | Zusätzliche Sicherheit |
| **PostgreSQL** | `pg_dump -Fc` je Datenbank oder `pg_dumpall`, optional im Docker-Container | Datenbankserver |
| **MySQL / MariaDB** | `mysqldump --single-transaction` inkl. Routinen, Triggern und Events | Webshops, CMS |
| **Docker-Volumes** | Benannte Volumes, Container optional für Konsistenz stoppen | Container-Hosts |
| **Befehl / Stream** | Standardausgabe eines Programms (z. B. `slapcat`, `redis-cli --rdb -`) | Spezialfälle |
| **Windows VSS** | Schattenkopie für geöffnete Dateien | Fileserver, Outlook-PST, Datenbanken unter Windows |
| **Spiegelung (Sync)** | Lesbare 1:1-Kopie, gelöschte Dateien optional in einen Papierkorb | USB-Disk, zweites NAS |
| **Archiv** | Eine `.tar.gz`-Datei pro Lauf, optional verschlüsselt (`.snarc`) | Langzeitarchiv, Übergabe |
| **Backup-Kopie (3-2-1)** | Kopiert Snapshots auf ein zweites Ziel, neu verschlüsselt | Offsite / Cloud |
| **Integritätsprüfung** | Liest einen Anteil der Daten, entschlüsselt und prüft sie | Wöchentlich |
| **Bereinigung** | Aufbewahrung anwenden und Speicher freigeben | Bei grossen Repositories |

### Ausschlussmuster

| Muster | Wirkung |
|---|---|
| `*.tmp` | Alle Dateien mit Endung `.tmp` |
| `node_modules` | Jeder Ordner/Datei mit diesem Namen, überall |
| `/var/log/**` | Alles unterhalb von `/var/log` |
| `**/cache` | Ordner `cache` in beliebiger Tiefe |
| `C:/Users/*/AppData/Local/Temp` | Windows-Pfad (Gross-/Kleinschreibung egal) |

### Skripte (Hooks)

Pre-, Post- und Fehler-Skripte laufen unter Linux mit `/bin/sh -c`, unter Windows mit PowerShell.
Verfügbare Variablen: `SNB_JOB_ID`, `SNB_JOB_NAME`, `SNB_JOB_TYPE`, `SNB_STATUS`.

## Zeitpläne

| Art | Beispiel |
|---|---|
| Intervall | alle 15 min, 2 h, 6 h |
| Täglich | `07:00, 12:00, 18:00` (mehrere Zeiten) |
| Wöchentlich | Mo–Fr 22:00 |
| Monatlich | am 1. und am letzten Tag, 03:00 |
| Cron | `30 2 * * 1-5`, `0 */4 * * *`, `@daily` |
| Nach Job | startet nach Job X, optional nur bei Erfolg (Verkettung) |

Zusätzlich: **Backup-Fenster** (z. B. 22:00–06:00, auch über Mitternacht), **Pausen** mit Datum,
**zufällige Verzögerung** und **Nachholen** verpasster Läufe nach einem Neustart.
Der Assistent zeigt die nächsten Ausführungen als Vorschau.

## Aufbewahrung (GFS)

Pro Zeitraum wird jeweils der **neueste** Snapshot behalten. Beispiel *Standard*:
`letzte 3 · täglich 7 · wöchentlich 4 · monatlich 12 · jährlich 3`. Die Regeln gelten pro Job und Host.
Ein Snapshot wird behalten, sobald **eine** Regel zutrifft.

## Ziele

### S3-kompatibel
| Anbieter | Endpoint (Beispiel) |
|---|---|
| AWS S3 | `https://s3.eu-central-2.amazonaws.com` (Zürich) |
| Wasabi | `https://s3.eu-central-2.wasabisys.com` |
| Infomaniak Swiss Backup | `https://s3.swiss-backup02.infomaniak.com` |
| Exoscale (CH) | `https://sos-ch-gva-2.exo.io` |
| Cloudflare R2 | `https://<account>.r2.cloudflarestorage.com` |
| MinIO | `https://minio.firma.local:9000` + *Path-Style* |

**Object Lock:** Bucket mit aktivierter Objektsperre anlegen, im Ziel *Compliance* und z. B. 30 Tage
wählen. Daten und Snapshots können dann während dieser Zeit von niemandem gelöscht werden.
Prune kann gesperrte Objekte erst nach Ablauf entfernen.

### rclone
`rclone config` als Dienstbenutzer ausführen (Linux: `sudo rclone config`), dann z. B. `nas-sftp:backups/server1` als Remote eintragen.

## Wiederherstellung

- **Einzelne Datei:** Wiederherstellen → Snapshot → Datei → Download.
- **Ordner als ZIP:** Ordner öffnen → *Ordner als ZIP*.
- **Viele Dateien:** auswählen → *Wiederherstellen…* → anderer Ordner oder Originalort.
- **Datenbanken:** `.dump`-Datei herunterladen und mit `pg_restore -d shop shop.dump` bzw. `mysql < shop.sql` einspielen.
- **Notfall ohne Server:** `snbackup restore latest --repo <ziel> --target /restore` funktioniert auf jedem Rechner mit dem Repository-Passwort.

## Kommandozeile

```
snbackup server        [--listen :8550] [--data DIR] [--cert F --key F] [--no-tls]
snbackup agent         enroll --server URL --token T [--fingerprint F] [--name N] | run [--host-root /host]
snbackup service       install|uninstall|status [--agent]
snbackup job           list | run <name> [--full] | history [name]
snbackup user reset    <name>
snbackup init          --repo R
snbackup backup        --repo R [--full | --differential] [--exclude P] [--exclude-file F] [--tag T] [--full] [--one-file-system]
                       [--exclude-caches] [--exclude-larger-than MB] [--compression off|fast|auto|max]
                       [--workers N] [--limit-upload KB/s] [--stdin-name NAME] pfade…
snbackup snapshots     --repo R
snbackup ls            --repo R <snapshot|latest> [pfad] [-l]
snbackup restore       --repo R <snapshot|latest> --target DIR | --in-place [--include P] [--overwrite always|never|newer|rename]
snbackup dump          --repo R <snapshot> <datei>
snbackup forget        --repo R --keep-last/-hourly/-daily/-weekly/-monthly/-yearly N [--keep-within-days N] [--job ID] [--prune] [--dry-run]
snbackup prune         --repo R
snbackup verify        --repo R [--read-data PROZENT]
snbackup copy          --repo R --to R2 [--to-password-file F] [--job ID]
snbackup unlock        --repo R
snbackup passwd        --repo R [--add]
snbackup archive       create --out F.tar.gz[.snarc] pfade… | extract F --target DIR
snbackup license | version
```

Repository-Angaben: `/pfad`, `D:\Backup`, `s3:https://host/bucket/prefix`, `webdav:https://…`, `rclone:remote:pfad`.
Umgebungsvariablen: `SNB_REPO`, `SNB_PASSWORD`, `SNB_PASSWORD_FILE`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `SNB_WEBDAV_USER`, `SNB_WEBDAV_PASSWORD`, `SNB_DATA`.

Exit-Codes: `0` OK, `1` Fehler, `2` falscher Aufruf, `3` mit Fehlern abgeschlossen bzw. Prüfung fehlgeschlagen.

## REST-API

Authentifizierung mit API-Token (*Einstellungen → Benutzer & API*):

```bash
curl -H "Authorization: Bearer snb_…" https://server:8550/api/dashboard
curl -H "Authorization: Bearer snb_…" -X POST https://server:8550/api/jobs/<id>/run
```

| Methode | Pfad | Zweck |
|---|---|---|
| GET | `/api/dashboard` | Kennzahlen, laufende Jobs, Verlauf (für Monitoring) |
| GET | `/api/jobs` · `/api/jobs/{id}` | Jobs |
| POST | `/api/jobs/{id}/run` · `/stop` · `/toggle` | Steuerung |
| GET | `/api/runs?job=&status=&limit=` · `/api/runs/{id}/log` | Verlauf & Protokolle |
| GET | `/api/repos` · `/api/repos/{id}/snapshots` | Ziele & Snapshots |
| GET | `/api/repos/{id}/snapshots/{sid}/ls?path=` · `/download?path=` | Durchsuchen & Herunterladen |
| POST | `/api/restore` | Wiederherstellung starten |
| GET | `/api/schedule?days=7` | Geplante Läufe |

## Dateien & Verzeichnisse

| Linux | Windows | Inhalt |
|---|---|---|
| `/var/lib/snbackup/state.json` | `C:\ProgramData\SWISSNAND\Backup\state.json` | Konfiguration (Geheimnisse verschlüsselt) |
| `…/secret.key` | `…\secret.key` | Schlüssel für die Konfiguration – **mitsichern!** |
| `…/runs.jsonl`, `…/logs/` | ebenso | Verlauf und Protokolle |
| `…/audit.jsonl` | ebenso | Audit-Log |
| `…/tls-cert.pem`, `tls-key.pem` | ebenso | Selbstsigniertes Zertifikat |

Tipp: Das Datenverzeichnis selbst mit einem kleinen Job auf ein externes Ziel sichern.

## Fehlerbehebung

| Problem | Lösung |
|---|---|
| «repository is locked» | Ein anderer Vorgang läuft oder ist abgestürzt → Ziel → *Sperren entfernen* bzw. `snbackup unlock` |
| «wrong password» | Passwort des Ziels prüfen; Gross-/Kleinschreibung beachten |
| VSS-Fehler | Dienst muss als SYSTEM/Administrator laufen, Dienst «Volumeschattenkopie» aktiv |
| Browser warnt vor Zertifikat | Normal beim selbstsignierten Zertifikat – eigenes Zertifikat mit `--cert/--key` angeben |
| Admin-Passwort vergessen | `sudo snbackup user reset admin`, danach Dienst neu starten |
| Linux-Dienst-Protokoll | `journalctl -u snbackup -f` |
