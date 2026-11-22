// Command snbackup is the SWISSNAND Backup server and command line tool.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/swissnand/snbackup/internal/archive"
	"github.com/swissnand/snbackup/internal/backend"
	"github.com/swissnand/snbackup/internal/jobs"
	"github.com/swissnand/snbackup/internal/license"
	"github.com/swissnand/snbackup/internal/repo"
	"github.com/swissnand/snbackup/internal/schedule"
	"github.com/swissnand/snbackup/internal/secure"
	"github.com/swissnand/snbackup/internal/server"
	"github.com/swissnand/snbackup/internal/store"
	"github.com/swissnand/snbackup/internal/version"
)

var color = os.Getenv("NO_COLOR") == "" && isTTY()

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func c(code, s string) string {
	if !color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func blue(s string) string  { return c("38;2;77;163;255", s) }
func green(s string) string { return c("32", s) }
func red(s string) string   { return c("31", s) }
func dim(s string) string   { return c("2", s) }
func bold(s string) string  { return c("1", s) }

func banner() {
	fmt.Println(blue("  ┌─┐┬ ┬┬┌─┐┌─┐┌┐┌┌─┐┌┐┌┌┬┐") + "  " + bold("SWISSNAND Backup") + " " + dim("v"+version.Version))
	fmt.Println(blue("  └─┐││││└─┐└─┐│││├─┤│││ ││") + "  " + dim("Sicher. Verschlüsselt. Swiss made."))
	fmt.Println(blue("  └─┘└┴┘┴└─┘└─┘┘└┘┴ ┴┘└┘─┴┘"))
	fmt.Println()
}

func defaultDataDir() string {
	if v := os.Getenv("SNB_DATA"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		pd := os.Getenv("ProgramData")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		return filepath.Join(pd, "SWISSNAND", "Backup")
	}
	if os.Geteuid() == 0 {
		return "/var/lib/snbackup"
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "snbackup")
}

func die(err error) {
	fmt.Fprintln(os.Stderr, red("Fehler: ")+err.Error())
	os.Exit(1)
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

// parse allows flags and positional arguments in any order.
func parse(fs *flag.FlagSet, args []string) []string {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			os.Exit(2)
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func usage() {
	banner()
	fmt.Print(`Verwendung: snbackup <befehl> [optionen]

` + bold("Server & GUI") + `
  server                 Web-Konsole + Planer starten (HTTPS, Standard :8550)
  service install        Als Dienst einrichten (systemd / Windows-Dienst via Aufgabenplanung)
  service uninstall      Dienst entfernen
  user reset <name>      Passwort eines Kontos zurücksetzen (Notfall)

` + bold("Agent (Server steuert diesen Rechner)") + `
  agent enroll           Beim Backup-Server registrieren (--server, --token, --fingerprint)
  agent run              Agent starten (läuft im Vordergrund)
  service install --agent  Agent als Dienst einrichten

` + bold("Jobs (verwendet die Server-Konfiguration)") + `
  job list               Alle Jobs mit nächstem Lauf anzeigen
  job run <name|id>      Job sofort ausführen (--full für Vollbackup)
  job history [name]     Letzte Läufe anzeigen

` + bold("Repository direkt") + `   (--repo PFAD | s3:https://host/bucket/prefix | webdav:https://… | rclone:remote:pfad)
  init                   Neues verschlüsseltes Repository anlegen
  backup <pfade…>        Backup erstellen (--exclude, --tag, --full, --stdin-name)
  snapshots              Snapshots auflisten
  ls <snapshot> [pfad]   Inhalt eines Snapshots anzeigen
  restore <snapshot>     Wiederherstellen (--target, --include, --overwrite, --verify)
  dump <snapshot> <datei>  Einzelne Datei nach stdout ausgeben
  forget                 Aufbewahrung anwenden (--keep-last/-daily/-weekly/-monthly/-yearly, --prune)
  prune                  Unbenutzte Daten löschen
  verify                 Integrität prüfen (--read-data 0-100)
  copy                   Snapshots in zweites Repository kopieren (--to, --to-password-file)
  unlock                 Hängende Sperren entfernen
  passwd                 Repository-Passwort ändern / hinzufügen (--add)
  archive create|extract Verschlüsselte .tar.gz(.snarc)-Archive

` + bold("Weitere") + `
  license                Lizenzstatus anzeigen
  version                Version anzeigen

Passwort: Umgebungsvariable SNB_PASSWORD, --password-file oder interaktive Eingabe.
Konfiguration: --data (Standard: ` + defaultDataDir() + `)
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(0)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "server", "serve":
		cmdServer(args)
	case "init":
		cmdInit(args)
	case "backup":
		cmdBackup(args)
	case "snapshots", "snaps":
		cmdSnapshots(args)
	case "ls":
		cmdLs(args)
	case "restore":
		cmdRestore(args)
	case "dump":
		cmdDump(args)
	case "forget":
		cmdForget(args)
	case "prune":
		cmdPrune(args)
	case "verify", "check":
		cmdVerify(args)
	case "copy":
		cmdCopy(args)
	case "unlock":
		cmdUnlock(args)
	case "passwd":
		cmdPasswd(args)
	case "archive":
		cmdArchive(args)
	case "job", "jobs":
		cmdJob(args)
	case "user":
		cmdUser(args)
	case "service":
		cmdService(args)
	case "agent":
		cmdAgent(args)
	case "license":
		cmdLicense(args)
	case "version", "--version", "-v":
		fmt.Printf("%s %s (%s, %s/%s)\n", version.Product, version.Version, version.Commit, runtime.GOOS, runtime.GOARCH)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintln(os.Stderr, "Unbekannter Befehl:", cmd)
		usage()
		os.Exit(2)
	}
}

func signalCtx() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// ---------------------------------------------------------------------------
// Repository helpers

type repoFlags struct {
	repo, pwFile, s3key, s3secret, s3region, davUser, davPass string
	pathStyle                                                 bool
	bw                                                        int
}

func (r *repoFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&r.repo, "repo", os.Getenv("SNB_REPO"), "Repository (Pfad, s3:, webdav:, rclone:)")
	fs.StringVar(&r.pwFile, "password-file", os.Getenv("SNB_PASSWORD_FILE"), "Datei mit dem Repository-Passwort")
	fs.StringVar(&r.s3key, "s3-access-key", os.Getenv("AWS_ACCESS_KEY_ID"), "S3 Access Key")
	fs.StringVar(&r.s3secret, "s3-secret-key", os.Getenv("AWS_SECRET_ACCESS_KEY"), "S3 Secret Key")
	fs.StringVar(&r.s3region, "s3-region", envOr("AWS_DEFAULT_REGION", "us-east-1"), "S3 Region")
	fs.BoolVar(&r.pathStyle, "s3-path-style", false, "S3 Path-Style-Adressierung (MinIO)")
	fs.StringVar(&r.davUser, "webdav-user", os.Getenv("SNB_WEBDAV_USER"), "WebDAV Benutzer")
	fs.StringVar(&r.davPass, "webdav-password", os.Getenv("SNB_WEBDAV_PASSWORD"), "WebDAV Passwort")
	fs.IntVar(&r.bw, "limit-upload", 0, "Upload-Limit in KB/s")
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func (r *repoFlags) backendConfig(url string) (backend.Config, error) {
	if url == "" {
		return backend.Config{}, errors.New("--repo fehlt")
	}
	cfg := backend.Config{BandwidthKBs: r.bw}
	switch {
	case strings.HasPrefix(url, "s3:"):
		u := strings.TrimPrefix(url, "s3:")
		if !strings.Contains(u, "://") {
			u = "https://" + u
		}
		scheme := u[:strings.Index(u, "://")+3]
		rest := strings.TrimPrefix(u, scheme)
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) < 2 {
			return cfg, errors.New("S3-Format: s3:https://endpoint/bucket[/prefix]")
		}
		cfg.Kind = "s3"
		cfg.S3 = backend.S3Config{Endpoint: scheme + parts[0], Bucket: parts[1], Region: r.s3region, AccessKey: r.s3key, SecretKey: r.s3secret, PathStyle: r.pathStyle}
		if len(parts) == 3 {
			cfg.S3.Prefix = parts[2]
		}
	case strings.HasPrefix(url, "webdav:"):
		cfg.Kind, cfg.WebDAVURL, cfg.WebDAVUser, cfg.WebDAVPassword = "webdav", strings.TrimPrefix(url, "webdav:"), r.davUser, r.davPass
	case strings.HasPrefix(url, "rclone:"):
		cfg.Kind, cfg.RcloneRemote = "rclone", strings.TrimPrefix(url, "rclone:")
	default:
		cfg.Kind, cfg.Path = "local", url
	}
	return cfg, nil
}

func readPassword(prompt, file string) string {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			die(err)
		}
		return strings.TrimRight(string(b), "\r\n")
	}
	if v := os.Getenv("SNB_PASSWORD"); v != "" {
		return v
	}
	fmt.Fprint(os.Stderr, prompt)
	if runtime.GOOS == "windows" {
		out, err := exec.Command("powershell.exe", "-NoProfile", "-Command",
			`$p = Read-Host -AsSecureString; [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($p))`).Output()
		if err == nil {
			return strings.TrimRight(string(out), "\r\n")
		}
	} else {
		st := exec.Command("stty", "-echo")
		st.Stdin = os.Stdin
		_ = st.Run()
		defer func() {
			e := exec.Command("stty", "echo")
			e.Stdin = os.Stdin
			_ = e.Run()
			fmt.Fprintln(os.Stderr)
		}()
	}
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

func (r *repoFlags) open(ctx context.Context) *repo.Repo {
	cfg, err := r.backendConfig(r.repo)
	if err != nil {
		die(err)
	}
	be, err := backend.Open(cfg)
	if err != nil {
		die(err)
	}
	rp, err := repo.Open(ctx, be, readPassword("Repository-Passwort: ", r.pwFile))
	if err != nil {
		die(err)
	}
	return rp
}

func human(n int64) string { return jobs.Human(n) }

func progressPrinter() func(repo.Progress) {
	if !isTTY() {
		return nil
	}
	return func(p repo.Progress) {
		pct := ""
		if p.TotalBytes > 0 {
			pct = fmt.Sprintf("%3.0f%% ", float64(p.Bytes)*100/float64(p.TotalBytes))
		}
		cur := p.Current
		if len(cur) > 50 {
			cur = "…" + cur[len(cur)-49:]
		}
		fmt.Fprintf(os.Stderr, "\r\x1b[K%s%s %d Dateien · %s · neu %s  %s", blue(pct), "⏵", p.Files, human(p.Bytes), human(p.NewBytes), dim(cur))
	}
}

// ---------------------------------------------------------------------------
// Commands

func cmdServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	listen := fs.String("listen", envOr("SNB_LISTEN", ":8550"), "Adresse der Web-Konsole")
	data := fs.String("data", defaultDataDir(), "Datenverzeichnis")
	cert := fs.String("cert", "", "TLS-Zertifikat (PEM), sonst selbstsigniert")
	key := fs.String("key", "", "TLS-Schlüssel (PEM)")
	noTLS := fs.Bool("no-tls", false, "Ohne TLS (nur auf 127.0.0.1 hinter Reverse-Proxy)")
	parse(fs, args)
	banner()
	st, err := store.Open(*data)
	if err != nil {
		die(err)
	}
	if pw, err := server.Bootstrap(st); err != nil {
		die(err)
	} else if pw != "" {
		fmt.Println(bold("  Erster Start – Administrator-Konto erstellt"))
		fmt.Println("  Benutzer: " + bold("admin"))
		fmt.Println("  Passwort: " + bold(pw) + dim("  (auch in "+filepath.Join(*data, "initial-admin-password")+")"))
		fmt.Println("  Das Passwort muss bei der ersten Anmeldung geändert werden.")
		fmt.Println()
	}
	m := jobs.New(st)
	m.Start()
	host, _ := os.Hostname()
	scheme := "https"
	if *noTLS {
		scheme = "http"
	}
	addr := *listen
	if strings.HasPrefix(addr, ":") {
		addr = host + addr
	}
	fmt.Printf("  Web-Konsole: %s\n", blue(scheme+"://"+addr))
	fmt.Printf("  Daten:       %s\n", *data)
	fmt.Printf("  Lizenz:      %s\n\n", license.Evaluate(st.Settings().License).Edition)
	ctx, cancel := signalCtx()
	defer cancel()
	srv := server.New(server.Config{Listen: *listen, DataDir: *data, CertFile: *cert, KeyFile: *key, NoTLS: *noTLS}, st, m)
	err = srv.Run(ctx)
	m.Stop()
	if err != nil {
		die(err)
	}
}

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	var rf repoFlags
	rf.register(fs)
	parse(fs, args)
	cfg, err := rf.backendConfig(rf.repo)
	if err != nil {
		die(err)
	}
	be, err := backend.Open(cfg)
	if err != nil {
		die(err)
	}
	pw := readPassword("Neues Repository-Passwort: ", rf.pwFile)
	if rf.pwFile == "" && os.Getenv("SNB_PASSWORD") == "" {
		if readPassword("Passwort wiederholen: ", "") != pw {
			die(errors.New("Passwörter stimmen nicht überein"))
		}
	}
	ctx, cancel := signalCtx()
	defer cancel()
	r, err := repo.Init(ctx, be, pw)
	if err != nil {
		die(err)
	}
	fmt.Println(green("✓") + " Repository " + r.ID()[:12] + " erstellt in " + be.Location())
	fmt.Println(dim("  Bewahre das Passwort sicher auf: ohne Passwort sind die Daten nicht wiederherstellbar."))
}

func cmdBackup(args []string) {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	var rf repoFlags
	rf.register(fs)
	var ex, tags multi
	fs.Var(&ex, "exclude", "Ausschlussmuster (mehrfach)")
	exFile := fs.String("exclude-file", "", "Datei mit Ausschlussmustern")
	fs.Var(&tags, "tag", "Tag (mehrfach)")
	full := fs.Bool("full", false, "Vollbackup: alle Dateien neu lesen")
	diff := fs.Bool("differential", false, "Differenziell: Änderungen seit dem letzten Vollbackup")
	oneFS := fs.Bool("one-file-system", false, "Nicht in andere Dateisysteme wechseln")
	caches := fs.Bool("exclude-caches", false, "Ordner mit CACHEDIR.TAG ausschliessen")
	larger := fs.Int("exclude-larger-than", 0, "Dateien grösser als N MB ausschliessen")
	comp := fs.String("compression", "auto", "off|fast|auto|max")
	workers := fs.Int("workers", 4, "Parallele Uploads")
	stdinName := fs.String("stdin-name", "", "Standardeingabe als Datei mit diesem Namen sichern")
	job := fs.String("job", "cli", "Job-Kennung für die Gruppierung")
	paths := parse(fs, args)
	if *exFile != "" {
		b, err := os.ReadFile(*exFile)
		if err != nil {
			die(err)
		}
		ex = append(ex, strings.Split(string(b), "\n")...)
	}
	if len(paths) == 0 && *stdinName == "" {
		die(errors.New("keine Pfade angegeben"))
	}
	ctx, cancel := signalCtx()
	defer cancel()
	r := rf.open(ctx)
	r.Compression = *comp
	lk, err := r.Lock(ctx, false, "cli backup")
	if err != nil {
		die(err)
	}
	defer lk.Unlock()
	opt := repo.BackupOptions{Paths: paths, Excludes: ex, Full: *full, Differential: *diff && !*full, OneFileSystem: *oneFS, ExcludeCaches: *caches, Workers: *workers, JobID: *job, JobName: *job, Tags: tags, Progress: progressPrinter(),
		Log: func(level, msg string) {
			if level != "info" {
				fmt.Fprintln(os.Stderr, "\r\x1b[K"+c("33", "! ")+msg)
			}
		}}
	if *larger > 0 {
		opt.ExcludeLargerThan = int64(*larger) << 20
	}
	if *stdinName != "" {
		opt.Kind = "stdin"
		opt.Streams = []repo.Stream{{Name: "_streams/stdin/" + *stdinName, Open: func(ctx context.Context) (io.ReadCloser, func() error, error) {
			return os.Stdin, func() error { return nil }, nil
		}}}
	}
	s, err := r.Backup(ctx, opt)
	if opt.Progress != nil {
		fmt.Fprintln(os.Stderr)
	}
	if err != nil {
		die(err)
	}
	st := s.Stats
	fmt.Printf("%s Snapshot %s gespeichert (%s)\n", green("✓"), bold(s.ID[:8]), s.Mode)
	fmt.Printf("  Dateien: %d (neu %d, geändert %d, unverändert %d)  Ordner: %d\n", st.Files, st.NewFiles, st.ChangedFile, st.Unchanged, st.Dirs)
	fmt.Printf("  Daten:   %s gesamt · %s neu · %s übertragen · %s\n", human(st.Bytes), human(st.NewBytes), human(st.StoredBytes), (time.Duration(st.DurationMs) * time.Millisecond).Round(time.Millisecond))
	if st.Errors > 0 {
		fmt.Printf("  %s %d Dateien konnten nicht gelesen werden\n", red("!"), st.Errors)
		os.Exit(3)
	}
}

func cmdSnapshots(args []string) {
	fs := flag.NewFlagSet("snapshots", flag.ExitOnError)
	var rf repoFlags
	rf.register(fs)
	parse(fs, args)
	ctx, cancel := signalCtx()
	defer cancel()
	r := rf.open(ctx)
	snaps, err := r.Snapshots(ctx)
	if err != nil {
		die(err)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, bold("ID\tZEIT\tHOST\tJOB\tART\tMODUS\tDATEIEN\tGRÖSSE\tNEU\tPFADE"))
	modes := map[string]string{"full": "voll", "incremental": "inkrementell", "differential": "differenziell"}
	for _, s := range snaps {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n", s.ID[:8], s.Time.Local().Format("02.01.2006 15:04"), s.Host, s.JobName, s.Kind, modes[s.Mode], s.Stats.Files, human(s.Stats.Bytes), human(s.Stats.NewBytes), strings.Join(s.Paths, ", "))
	}
	tw.Flush()
	fmt.Println(dim(fmt.Sprintf("%d Snapshots", len(snaps))))
}

func cmdLs(args []string) {
	fs := flag.NewFlagSet("ls", flag.ExitOnError)
	var rf repoFlags
	rf.register(fs)
	long := fs.Bool("l", false, "Details")
	pos := parse(fs, args)
	if len(pos) == 0 {
		die(errors.New("Snapshot-ID fehlt"))
	}
	ctx, cancel := signalCtx()
	defer cancel()
	r := rf.open(ctx)
	s, err := r.LoadSnapshot(ctx, pos[0])
	if err != nil {
		die(err)
	}
	prefix := ""
	if len(pos) > 1 {
		prefix = strings.Trim(filepath.ToSlash(pos[1]), "/")
	}
	_ = r.WalkTree(ctx, s, func(n *repo.Node) error {
		if prefix != "" && n.Path != prefix && !strings.HasPrefix(n.Path, prefix+"/") {
			return nil
		}
		if *long {
			fmt.Printf("%s %10s  %s  %s\n", n.Type, human(n.Size), time.Unix(0, n.MTime).Format("2006-01-02 15:04"), n.Path)
		} else {
			fmt.Println(n.Path)
		}
		return nil
	})
}

func cmdRestore(args []string) {
	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	var rf repoFlags
	rf.register(fs)
	target := fs.String("target", "", "Zielordner (leer = Originalort, benötigt --in-place)")
	inPlace := fs.Bool("in-place", false, "An den Originalort wiederherstellen")
	var inc multi
	fs.Var(&inc, "include", "Nur diesen Pfad wiederherstellen (mehrfach)")
	ow := fs.String("overwrite", "always", "always|never|newer|rename")
	verify := fs.Bool("verify", true, "Prüfsummen kontrollieren")
	owner := fs.Bool("owner", false, "Besitzer wiederherstellen (root)")
	dry := fs.Bool("dry-run", false, "Nur anzeigen")
	pos := parse(fs, args)
	if len(pos) == 0 {
		die(errors.New("Snapshot-ID fehlt (oder 'latest')"))
	}
	if *target == "" && !*inPlace {
		die(errors.New("--target angeben oder --in-place für den Originalort"))
	}
	ctx, cancel := signalCtx()
	defer cancel()
	r := rf.open(ctx)
	var s *repo.Snapshot
	var err error
	if pos[0] == "latest" {
		snaps, e := r.Snapshots(ctx)
		if e != nil || len(snaps) == 0 {
			die(errors.New("keine Snapshots"))
		}
		s = snaps[0]
	} else if s, err = r.LoadSnapshot(ctx, pos[0]); err != nil {
		die(err)
	}
	for i := range inc {
		if abs, err := filepath.Abs(inc[i]); err == nil && filepath.IsAbs(inc[i]) {
			inc[i] = repo.StoreName(abs)
		}
	}
	res, err := r.Restore(ctx, s, repo.RestoreOptions{Target: *target, Include: inc, Overwrite: *ow, Verify: *verify, RestoreOwner: *owner, DryRun: *dry, Progress: progressPrinter(),
		Log: func(level, msg string) { fmt.Fprintln(os.Stderr, "\r\x1b[K"+c("33", "! ")+msg) }})
	if isTTY() {
		fmt.Fprintln(os.Stderr)
	}
	if err != nil {
		die(err)
	}
	fmt.Printf("%s %d Dateien · %d Ordner · %s wiederhergestellt · %d übersprungen · %d Fehler\n", green("✓"), res.Files, res.Dirs, human(res.Bytes), res.Skipped, res.Errors)
	if res.Errors > 0 {
		os.Exit(3)
	}
}

func cmdDump(args []string) {
	fs := flag.NewFlagSet("dump", flag.ExitOnError)
	var rf repoFlags
	rf.register(fs)
	pos := parse(fs, args)
	if len(pos) < 2 {
		die(errors.New("Verwendung: dump <snapshot> <pfad>"))
	}
	ctx, cancel := signalCtx()
	defer cancel()
	r := rf.open(ctx)
	s, err := r.LoadSnapshot(ctx, pos[0])
	if err != nil {
		die(err)
	}
	want := strings.Trim(filepath.ToSlash(pos[1]), "/")
	if filepath.IsAbs(pos[1]) {
		want = repo.StoreName(pos[1])
	}
	found := false
	_ = r.WalkTree(ctx, s, func(n *repo.Node) error {
		if n.Path == want && n.Type == "f" {
			found = true
			return r.WriteFile(ctx, n, os.Stdout)
		}
		return nil
	})
	if !found {
		die(errors.New("Datei nicht im Snapshot gefunden"))
	}
}

func retentionFlags(fs *flag.FlagSet) *repo.Retention {
	p := &repo.Retention{}
	fs.IntVar(&p.KeepLast, "keep-last", 0, "Letzte N behalten")
	fs.IntVar(&p.KeepHourly, "keep-hourly", 0, "N stündliche")
	fs.IntVar(&p.KeepDaily, "keep-daily", 0, "N tägliche")
	fs.IntVar(&p.KeepWeekly, "keep-weekly", 0, "N wöchentliche")
	fs.IntVar(&p.KeepMonthly, "keep-monthly", 0, "N monatliche")
	fs.IntVar(&p.KeepYearly, "keep-yearly", 0, "N jährliche")
	fs.IntVar(&p.KeepWithinDays, "keep-within-days", 0, "Alle der letzten N Tage")
	return p
}

func cmdForget(args []string) {
	fs := flag.NewFlagSet("forget", flag.ExitOnError)
	var rf repoFlags
	rf.register(fs)
	p := retentionFlags(fs)
	job := fs.String("job", "", "Nur Snapshots dieses Jobs")
	dry := fs.Bool("dry-run", false, "Nur anzeigen")
	prune := fs.Bool("prune", false, "Danach Speicher freigeben")
	parse(fs, args)
	if p.Empty() {
		die(errors.New("mindestens eine --keep-* Regel angeben"))
	}
	ctx, cancel := signalCtx()
	defer cancel()
	r := rf.open(ctx)
	dec, err := r.Forget(ctx, *p, *job, *dry)
	if err != nil {
		die(err)
	}
	for _, d := range dec {
		mark := green("behalten")
		if !d.Keep {
			mark = red("entfernen")
		}
		fmt.Printf("%s  %s  %-10s %s\n", d.Snapshot.ID[:8], d.Snapshot.Time.Local().Format("02.01.2006 15:04"), mark, dim(strings.Join(d.Reasons, ", ")))
	}
	if *prune && !*dry {
		cmdPruneWith(ctx, r)
	}
}

func cmdPrune(args []string) {
	fs := flag.NewFlagSet("prune", flag.ExitOnError)
	var rf repoFlags
	rf.register(fs)
	parse(fs, args)
	ctx, cancel := signalCtx()
	defer cancel()
	cmdPruneWith(ctx, rf.open(ctx))
}

func cmdPruneWith(ctx context.Context, r *repo.Repo) {
	res, err := r.Prune(ctx, false, func(s string) { fmt.Fprintln(os.Stderr, s) })
	if err != nil {
		die(err)
	}
	fmt.Printf("%s %d von %d Chunks gelöscht · %s freigegeben\n", green("✓"), res.Deleted, res.TotalChunks, human(res.FreedBytes))
}

func cmdVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	var rf repoFlags
	rf.register(fs)
	pct := fs.Int("read-data", 0, "Prozent der Daten lesen und prüfen (0-100)")
	parse(fs, args)
	ctx, cancel := signalCtx()
	defer cancel()
	r := rf.open(ctx)
	res, err := r.Verify(ctx, *pct, func(s string) { fmt.Println(dim("  " + s)) })
	if err != nil {
		die(err)
	}
	for _, p := range res.Problems {
		fmt.Println(red("✗ ") + p)
	}
	fmt.Printf("Snapshots: %d · Chunks: %d · gelesen: %d · fehlend: %d · beschädigt: %d · verwaist: %d\n", res.Snapshots, res.Chunks, res.ReadChunks, res.Missing, res.Corrupt, res.OrphanChunks)
	if !res.OK() {
		fmt.Println(red("✗ Repository beschädigt"))
		os.Exit(3)
	}
	fmt.Println(green("✓ Repository ist intakt"))
}

func cmdCopy(args []string) {
	fs := flag.NewFlagSet("copy", flag.ExitOnError)
	var rf, df repoFlags
	rf.register(fs)
	fs.StringVar(&df.repo, "to", "", "Ziel-Repository")
	fs.StringVar(&df.pwFile, "to-password-file", "", "Passwortdatei des Ziels")
	job := fs.String("job", "", "Nur Snapshots dieses Jobs")
	parse(fs, args)
	df.s3key, df.s3secret, df.s3region, df.pathStyle = rf.s3key, rf.s3secret, rf.s3region, rf.pathStyle
	ctx, cancel := signalCtx()
	defer cancel()
	src := rf.open(ctx)
	if df.pwFile == "" {
		os.Unsetenv("SNB_PASSWORD")
	}
	dst := df.open(ctx)
	res, err := repo.CopySnapshots(ctx, src, dst, func(s *repo.Snapshot) bool { return *job == "" || s.JobID == *job }, "", func(s string) { fmt.Println(dim("  " + s)) })
	if err != nil {
		die(err)
	}
	fmt.Printf("%s %d kopiert · %d vorhanden · %s übertragen\n", green("✓"), res.Copied, res.Skipped, human(res.StoredBytes))
}

func cmdUnlock(args []string) {
	fs := flag.NewFlagSet("unlock", flag.ExitOnError)
	var rf repoFlags
	rf.register(fs)
	parse(fs, args)
	ctx, cancel := signalCtx()
	defer cancel()
	n, err := rf.open(ctx).RemoveAllLocks(ctx)
	if err != nil {
		die(err)
	}
	fmt.Printf("%s %d Sperren entfernt\n", green("✓"), n)
}

func cmdPasswd(args []string) {
	fs := flag.NewFlagSet("passwd", flag.ExitOnError)
	var rf repoFlags
	rf.register(fs)
	add := fs.Bool("add", false, "Zusätzliches Passwort hinzufügen statt ersetzen")
	parse(fs, args)
	ctx, cancel := signalCtx()
	defer cancel()
	r := rf.open(ctx)
	os.Unsetenv("SNB_PASSWORD")
	pw := readPassword("Neues Passwort: ", "")
	if readPassword("Wiederholen: ", "") != pw {
		die(errors.New("Passwörter stimmen nicht überein"))
	}
	var err error
	if *add {
		err = r.AddPassword(ctx, pw, "cli")
	} else {
		err = r.ChangePassword(ctx, pw)
	}
	if err != nil {
		die(err)
	}
	fmt.Println(green("✓") + " Passwort gespeichert")
}

func cmdArchive(args []string) {
	if len(args) == 0 {
		die(errors.New("Verwendung: archive create --out DATEI pfade… | archive extract DATEI --target DIR"))
	}
	sub := args[0]
	fs := flag.NewFlagSet("archive", flag.ExitOnError)
	out := fs.String("out", "", "Archivdatei (.tar.gz oder .tar.gz.snarc für verschlüsselt)")
	target := fs.String("target", ".", "Zielordner beim Entpacken")
	pwFile := fs.String("password-file", "", "Passwortdatei")
	var ex multi
	fs.Var(&ex, "exclude", "Ausschlussmuster")
	pos := parse(fs, args[1:])
	switch sub {
	case "create":
		if *out == "" || len(pos) == 0 {
			die(errors.New("--out und Pfade angeben"))
		}
		pw := ""
		if strings.HasSuffix(*out, ".snarc") {
			pw = readPassword("Archiv-Passwort: ", *pwFile)
		}
		m, err := repo.NewMatcher(ex)
		if err != nil {
			die(err)
		}
		n, b, err := archive.Create(*out, pos, m, pw, func(s string) { fmt.Fprintln(os.Stderr, s) })
		if err != nil {
			die(err)
		}
		fmt.Printf("%s %s: %d Dateien, %s\n", green("✓"), *out, n, human(b))
	case "extract":
		if len(pos) == 0 {
			die(errors.New("Archivdatei angeben"))
		}
		pw := ""
		if strings.HasSuffix(pos[0], ".snarc") {
			pw = readPassword("Archiv-Passwort: ", *pwFile)
		}
		n, err := archive.Extract(pos[0], *target, pw)
		if err != nil {
			die(err)
		}
		fmt.Printf("%s %d Dateien entpackt nach %s\n", green("✓"), n, *target)
	default:
		die(errors.New("create oder extract"))
	}
}

func openStore(fs *flag.FlagSet, args []string) (*store.Store, []string) {
	data := fs.String("data", defaultDataDir(), "Datenverzeichnis")
	pos := parse(fs, args)
	st, err := store.Open(*data)
	if err != nil {
		die(err)
	}
	return st, pos
}

func findJob(st *store.Store, key string) *store.Job {
	for _, j := range st.Jobs() {
		if j.ID == key || strings.EqualFold(j.Name, key) {
			return j
		}
	}
	die(fmt.Errorf("Job %q nicht gefunden", key))
	return nil
}

func statusColor(s string) string {
	switch s {
	case "success":
		return green("● erfolgreich")
	case "warning":
		return c("33", "● Warnung")
	case "failed":
		return red("● Fehler")
	case "running":
		return blue("● läuft")
	case "":
		return dim("○ noch nie")
	}
	return dim("● " + s)
}

func cmdJob(args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}
	sub := args[0]
	fs := flag.NewFlagSet("job", flag.ExitOnError)
	full := fs.Bool("full", false, "Vollbackup erzwingen")
	st, pos := openStore(fs, args[1:])
	switch sub {
	case "list", "ls":
		js := st.Jobs()
		sort.Slice(js, func(i, k int) bool { return js[i].Name < js[k].Name })
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, bold("NAME\tTYP\tZEITPLAN\tNÄCHSTER LAUF\tLETZTER STATUS\tAKTIV"))
		for _, j := range js {
			next := "–"
			if t := schedule.Next(j, time.Now()); !t.IsZero() {
				next = t.Format("02.01. 15:04")
			}
			act := green("ja")
			if !j.Enabled {
				act = dim("nein")
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", j.Name, j.Type, schedule.Describe(j.Schedule), next, statusColor(j.LastStatus), act)
		}
		tw.Flush()
	case "run":
		if len(pos) == 0 {
			die(errors.New("Jobname angeben"))
		}
		j := findJob(st, pos[0])
		m := jobs.New(st)
		fmt.Printf("Starte %s …\n", bold(j.Name))
		if err := m.Trigger(j.ID, "cli", *full); err != nil {
			die(err)
		}
		lastLen := 0
		for {
			time.Sleep(400 * time.Millisecond)
			live := m.Running()
			if len(live) == 0 && !m.IsBusy(j.ID) {
				break
			}
			for _, l := range live {
				for _, line := range l.Log[min(lastLen, len(l.Log)):] {
					fmt.Println(dim(line))
				}
				lastLen = len(l.Log)
			}
		}
		runs := st.Runs(j.ID, 1)
		if len(runs) > 0 {
			fmt.Println(statusColor(runs[0].Status) + "  " + runs[0].Message)
			if runs[0].Status == "failed" {
				os.Exit(3)
			}
		}
	case "history":
		job := ""
		if len(pos) > 0 {
			job = findJob(st, pos[0]).ID
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, bold("START\tJOB\tSTATUS\tDAUER\tMELDUNG"))
		for _, r := range st.Runs(job, 30) {
			d := "–"
			if r.End != nil {
				d = r.End.Sub(r.Start).Round(time.Second).String()
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Start.Format("02.01. 15:04"), r.JobName, statusColor(r.Status), d, r.Message)
		}
		tw.Flush()
	default:
		die(errors.New("list, run oder history"))
	}
}

func cmdUser(args []string) {
	if len(args) < 2 || args[0] != "reset" {
		die(errors.New("Verwendung: snbackup user reset <name> [--data DIR]"))
	}
	fs := flag.NewFlagSet("user", flag.ExitOnError)
	st, _ := openStore(fs, args[2:])
	name := strings.ToLower(args[1])
	pw := secure.RandomToken(12)
	h, _ := secure.HashPassword(pw)
	found := false
	err := st.Update(func(s *store.State) error {
		for _, u := range s.Users {
			if u.Username == name {
				u.PassHash, u.MustChange, u.Disabled = h, true, false
				u.TOTPEnabled, u.TOTPSecret = false, ""
				found = true
			}
		}
		if !found {
			s.Users = append(s.Users, &store.User{Username: name, Role: "admin", PassHash: h, MustChange: true, Created: time.Now()})
		}
		return nil
	})
	if err != nil {
		die(err)
	}
	fmt.Printf("%s Konto %s: neues Passwort %s (muss bei Anmeldung geändert werden, 2FA zurückgesetzt). Server neu starten, falls aktiv.\n", green("✓"), bold(name), bold(pw))
}

func cmdLicense(args []string) {
	fs := flag.NewFlagSet("license", flag.ExitOnError)
	st, _ := openStore(fs, args)
	info := license.Evaluate(st.Settings().License)
	fmt.Printf("Edition: %s\n%s\n", bold(info.Edition), info.Message)
	if info.Customer != "" {
		fmt.Printf("Kunde: %s · Server: %s\n", info.Customer, strconv.Itoa(info.Servers))
	}
}
