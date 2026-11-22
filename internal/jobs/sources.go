package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/swissnand/snbackup/internal/repo"
)

// shell returns the platform shell invocation for a command line.
func shell(ctx context.Context, cmdline string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", cmdline)
	}
	return exec.CommandContext(ctx, "/bin/sh", "-c", cmdline)
}

// niceCmd lowers CPU/IO priority of child processes on Linux.
func niceCmd(ctx context.Context, prio string, name string, args ...string) *exec.Cmd {
	if runtime.GOOS == "linux" && (prio == "low" || prio == "idle") {
		if _, err := exec.LookPath("nice"); err == nil {
			pre := []string{"-n", "15"}
			if _, err := exec.LookPath("ionice"); err == nil {
				class := "2"
				if prio == "idle" {
					class = "3"
				}
				pre = append(pre, "ionice", "-c", class, name)
			} else {
				pre = append(pre, name)
			}
			return exec.CommandContext(ctx, "nice", append(pre, args...)...)
		}
	}
	return exec.CommandContext(ctx, name, args...)
}

func (rc *runCtx) hook(ctx context.Context, label, cmdline, status string) error {
	to := rc.job.Hooks.TimeoutMin
	if to <= 0 {
		to = 30
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(to)*time.Minute)
	defer cancel()
	rc.phase(label)
	cmd := shell(ctx, cmdline)
	cmd.Env = append(os.Environ(), "SNB_JOB_ID="+rc.job.ID, "SNB_JOB_NAME="+rc.job.Name, "SNB_JOB_TYPE="+rc.job.Type, "SNB_STATUS="+status)
	out, err := cmd.CombinedOutput()
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			rc.log("info", "  "+l)
		}
	}
	return err
}

// cmdStream wraps a command whose stdout becomes a backup stream.
func cmdStream(name string, mk func(ctx context.Context) (*exec.Cmd, func(), error)) repo.Stream {
	return repo.Stream{Name: name, Open: func(ctx context.Context) (io.ReadCloser, func() error, error) {
		cmd, cleanup, err := mk(ctx)
		if err != nil {
			return nil, nil, err
		}
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, err
		}
		if err := cmd.Start(); err != nil {
			if cleanup != nil {
				cleanup()
			}
			return nil, nil, fmt.Errorf("%s: %w", cmd.Path, err)
		}
		return out, func() error {
			err := cmd.Wait()
			if cleanup != nil {
				cleanup()
			}
			if err != nil {
				msg := strings.TrimSpace(stderr.String())
				if len(msg) > 500 {
					msg = msg[:500]
				}
				return fmt.Errorf("%v: %s", err, msg)
			}
			return nil
		}, nil
	}}
}

// prepareSource fills BackupOptions according to the job's source kind.
func (rc *runCtx) prepareSource(ctx context.Context, opt *repo.BackupOptions) (func(), error) {
	s := rc.job.Source
	prio := rc.job.Options.Priority
	box := rc.m.st.Box()
	switch s.Kind {
	case "files", "":
		opt.Kind = "files"
		if len(s.Paths) == 0 {
			return nil, errors.New("keine Quellpfade angegeben")
		}
		if s.UseVSS {
			if runtime.GOOS != "windows" {
				rc.log("warn", "VSS ist nur unter Windows verfügbar – wird ignoriert")
				return nil, nil
			}
			rc.phase("VSS-Schattenkopie erstellen")
			mapper, cleanup, err := createShadows(ctx, s.Paths, rc.log)
			if err != nil {
				return nil, fmt.Errorf("VSS: %w", err)
			}
			opt.ReadPath = mapper
			return cleanup, nil
		}
		return nil, nil

	case "postgres":
		pw, _ := box.Decrypt(s.DBPassword)
		base := func(ctx context.Context, bin string, args ...string) (*exec.Cmd, func(), error) {
			conn := []string{}
			if s.DBHost != "" {
				conn = append(conn, "-h", s.DBHost)
			}
			if s.DBPort != 0 {
				conn = append(conn, "-p", fmt.Sprint(s.DBPort))
			}
			if s.DBUser != "" {
				conn = append(conn, "-U", s.DBUser)
			}
			all := append(conn, args...)
			all = append(all, strings.Fields(s.DumpArgs)...)
			var cmd *exec.Cmd
			if s.DockerContainer != "" {
				dargs := []string{"exec", "-i"}
				if pw != "" {
					dargs = append(dargs, "-e", "PGPASSWORD")
				}
				dargs = append(dargs, s.DockerContainer, bin)
				cmd = niceCmd(ctx, prio, "docker", append(dargs, all...)...)
			} else {
				cmd = niceCmd(ctx, prio, bin, all...)
			}
			cmd.Env = append(os.Environ(), "PGPASSWORD="+pw)
			return cmd, nil, nil
		}
		if s.AllDatabases || len(s.Databases) == 0 {
			opt.Streams = append(opt.Streams, cmdStream("_streams/postgres/pg_dumpall.sql", func(ctx context.Context) (*exec.Cmd, func(), error) {
				return base(ctx, "pg_dumpall", "--clean", "--if-exists")
			}))
		} else {
			for _, db := range s.Databases {
				db := db
				opt.Streams = append(opt.Streams, cmdStream("_streams/postgres/"+safeName(db)+".dump", func(ctx context.Context) (*exec.Cmd, func(), error) {
					return base(ctx, "pg_dump", "-Fc", "-d", db)
				}))
			}
		}
		opt.Kind = "postgres"
		opt.Paths = nil
		return nil, nil

	case "mysql":
		pw, _ := box.Decrypt(s.DBPassword)
		mk := func(dbs []string) func(ctx context.Context) (*exec.Cmd, func(), error) {
			return func(ctx context.Context) (*exec.Cmd, func(), error) {
				// Credentials are passed via a private option file, never on
				// the command line (would be visible in the process list).
				f, err := os.CreateTemp("", "snb-my-*.cnf")
				if err != nil {
					return nil, nil, err
				}
				_ = f.Chmod(0o600)
				fmt.Fprintf(f, "[client]\nuser=%q\npassword=%q\n", s.DBUser, pw)
				if s.DBHost != "" {
					fmt.Fprintf(f, "host=%s\n", s.DBHost)
				}
				if s.DBPort != 0 {
					fmt.Fprintf(f, "port=%d\n", s.DBPort)
				}
				f.Close()
				cleanup := func() { os.Remove(f.Name()) }
				args := []string{"--defaults-extra-file=" + f.Name(), "--single-transaction", "--quick", "--routines", "--triggers", "--events", "--hex-blob"}
				args = append(args, strings.Fields(s.DumpArgs)...)
				if len(dbs) == 0 {
					args = append(args, "--all-databases")
				} else {
					args = append(args, "--databases")
					args = append(args, dbs...)
				}
				bin := "mysqldump"
				if _, err := exec.LookPath(bin); err != nil {
					if _, err2 := exec.LookPath("mariadb-dump"); err2 == nil {
						bin = "mariadb-dump"
					}
				}
				if s.DockerContainer != "" {
					cleanup()
					dargs := []string{"exec", "-i", "-e", "MYSQL_PWD", s.DockerContainer, bin, "-u", s.DBUser, "--single-transaction", "--quick", "--routines", "--triggers", "--events"}
					if len(dbs) == 0 {
						dargs = append(dargs, "--all-databases")
					} else {
						dargs = append(append(dargs, "--databases"), dbs...)
					}
					cmd := niceCmd(ctx, prio, "docker", dargs...)
					cmd.Env = append(os.Environ(), "MYSQL_PWD="+pw)
					return cmd, nil, nil
				}
				return niceCmd(ctx, prio, bin, args...), cleanup, nil
			}
		}
		if s.AllDatabases || len(s.Databases) == 0 {
			opt.Streams = append(opt.Streams, cmdStream("_streams/mysql/all-databases.sql", mk(nil)))
		} else {
			for _, db := range s.Databases {
				opt.Streams = append(opt.Streams, cmdStream("_streams/mysql/"+safeName(db)+".sql", mk([]string{db})))
			}
		}
		opt.Kind = "mysql"
		opt.Paths = nil
		return nil, nil

	case "command":
		if s.Command == "" {
			return nil, errors.New("kein Befehl angegeben")
		}
		name := s.CommandName
		if name == "" {
			name = "output"
		}
		opt.Streams = append(opt.Streams, cmdStream("_streams/command/"+safeName(name), func(ctx context.Context) (*exec.Cmd, func(), error) {
			return shell(ctx, s.Command), nil, nil
		}))
		opt.Kind = "command"
		opt.Paths = append([]string(nil), s.Paths...)
		return nil, nil

	case "docker":
		opt.Kind = "docker"
		root := "/var/lib/docker/volumes"
		if runtime.GOOS == "windows" {
			root = `C:\ProgramData\docker\volumes`
		}
		opt.Paths = nil
		for _, v := range s.DockerVolumes {
			if strings.ContainsAny(v, `/\`) || v == ".." {
				return nil, fmt.Errorf("ungültiger Volume-Name %q", v)
			}
			opt.Paths = append(opt.Paths, filepath.Join(root, v, "_data"))
		}
		opt.Paths = append(opt.Paths, s.Paths...)
		if len(opt.Paths) == 0 {
			return nil, errors.New("keine Docker-Volumes angegeben")
		}
		var stopped []string
		for _, c := range s.StopContainers {
			rc.logf2("info", "Container stoppen: %s", c)
			if out, err := exec.CommandContext(ctx, "docker", "stop", c).CombinedOutput(); err != nil {
				rc.logf2("warn", "docker stop %s: %v %s", c, err, strings.TrimSpace(string(out)))
				continue
			}
			stopped = append(stopped, c)
		}
		return func() {
			for _, c := range stopped {
				rc.logf2("info", "Container starten: %s", c)
				if out, err := exec.Command("docker", "start", c).CombinedOutput(); err != nil {
					rc.logf2("error", "docker start %s: %v %s", c, err, strings.TrimSpace(string(out)))
				}
			}
		}, nil
	case "proxmox", "hyperv", "image", "sysimage":
		return rc.prepareVM(ctx, opt)
	}
	return nil, fmt.Errorf("unbekannte Quelle %q", s.Kind)
}
