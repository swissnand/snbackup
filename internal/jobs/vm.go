package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/swissnand/snbackup/internal/repo"
)

// VM, disk image and system image backups.
//
//   - proxmox:  vzdump streams each VM/container uncompressed (so the data
//     deduplicates between runs) straight into the repository. Restore:
//     snbackup dump <snap> _streams/proxmox/vzdump-qemu-101.vma | qmrestore - 101
//   - hyperv:   Export-VM (production checkpoint, VM keeps running) into a
//     stable staging folder which is then backed up and removed.
//   - image:    raw block devices or image files, optionally from an LVM
//     snapshot. Restore from a rescue system:
//     snbackup dump <snap> _streams/image/dev-sda.img > /dev/sda
//   - sysimage: Windows Server Backup (wbadmin -allCritical) bare-metal
//     image into a staging volume, then into the repository.

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func streamName(prefix, s string) string {
	return prefix + strings.Trim(unsafeChars.ReplaceAllString(s, "-"), "-")
}

func fileStream(name, path string) repo.Stream {
	return repo.Stream{Name: name, Open: func(ctx context.Context) (io.ReadCloser, func() error, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, nil, err
		}
		return f, func() error { return nil }, nil
	}}
}

func (rc *runCtx) prepareVM(ctx context.Context, opt *repo.BackupOptions) (func(), error) {
	s := rc.job.Source
	prio := rc.job.Options.Priority
	opt.Kind = s.Kind
	opt.Paths = nil
	switch s.Kind {

	case "proxmox":
		if _, err := exec.LookPath("vzdump"); err != nil {
			return nil, errors.New("vzdump nicht gefunden – der Job muss auf einem Proxmox-VE-Host laufen (Agent auf dem Host installieren)")
		}
		ids := s.VMIDs
		if len(ids) == 0 || (len(ids) == 1 && strings.EqualFold(ids[0], "all")) {
			ids = nil
			for _, lister := range [][]string{{"qm", "list"}, {"pct", "list"}} {
				out, err := exec.CommandContext(ctx, lister[0], lister[1:]...).Output()
				if err != nil {
					continue
				}
				for i, line := range strings.Split(string(out), "\n") {
					f := strings.Fields(line)
					if i == 0 || len(f) == 0 {
						continue
					}
					ids = append(ids, f[0])
				}
			}
			rc.logf2("info", "Proxmox: %d VMs/Container gefunden", len(ids))
		}
		mode := s.VzdumpMode
		if mode == "" {
			mode = "snapshot"
		}
		for _, id := range ids {
			id := strings.TrimSpace(id)
			if _, err := fmt.Sscanf(id, "%d", new(int)); err != nil {
				return nil, fmt.Errorf("ungültige VM-ID %q", id)
			}
			kind := "qemu"
			ext := ".vma"
			if exec.CommandContext(ctx, "qm", "config", id).Run() != nil {
				kind, ext = "lxc", ".tar"
			}
			opt.Streams = append(opt.Streams, cmdStream(streamName("_streams/proxmox/vzdump-"+kind+"-", id)+ext, func(ctx context.Context) (*exec.Cmd, func(), error) {
				return niceCmd(ctx, prio, "vzdump", id, "--mode", mode, "--stdout", "--compress", "0", "--quiet", "1"), nil, nil
			}))
		}
		if len(opt.Streams) == 0 {
			return nil, errors.New("keine VMs gefunden")
		}
		return nil, nil

	case "hyperv":
		if runtime.GOOS != "windows" {
			return nil, errors.New("Hyper-V-Backups laufen nur auf dem Hyper-V-Host (Windows)")
		}
		if len(s.HyperVVMs) == 0 {
			return nil, errors.New("keine VM angegeben")
		}
		base := filepath.Join(os.Getenv("ProgramData"), "SWISSNAND", "Backup", "hyperv-export")
		var dirs []string
		cleanup := func() {
			for _, d := range dirs {
				_ = os.RemoveAll(d)
			}
		}
		for _, vm := range s.HyperVVMs {
			if strings.ContainsAny(vm, `'"`+"`$;") {
				cleanup()
				return nil, fmt.Errorf("ungültiger VM-Name %q", vm)
			}
			dir := filepath.Join(base, streamName("", vm))
			_ = os.RemoveAll(dir)
			_ = os.MkdirAll(dir, 0o700)
			rc.phase("Hyper-V: VM «" + vm + "» exportieren")
			script := fmt.Sprintf(`Export-VM -Name '%s' -Path '%s' -CaptureLiveState CaptureSavedState -ErrorAction Stop`, vm, dir)
			out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
			if err != nil {
				cleanup()
				return nil, fmt.Errorf("Export-VM %s: %v %s", vm, err, strings.TrimSpace(string(out)))
			}
			dirs = append(dirs, dir)
			opt.Paths = append(opt.Paths, dir)
		}
		return cleanup, nil

	case "image":
		if len(s.Devices) == 0 {
			return nil, errors.New("kein Gerät angegeben")
		}
		var snaps []string
		cleanup := func() {
			for _, sn := range snaps {
				if out, err := exec.Command("lvremove", "-f", sn).CombinedOutput(); err != nil {
					rc.logf2("error", "LVM-Snapshot %s nicht entfernt: %v %s", sn, err, strings.TrimSpace(string(out)))
				} else {
					rc.logf2("info", "LVM-Snapshot entfernt: %s", sn)
				}
			}
		}
		for _, dev := range s.Devices {
			read := dev
			if s.LVMSnapshot {
				if runtime.GOOS != "linux" {
					return nil, errors.New("LVM-Snapshots nur unter Linux")
				}
				name := "snb_" + streamName("", filepath.Base(dev))
				out, err := exec.CommandContext(ctx, "lvcreate", "-s", "-n", name, "-l", "20%ORIGIN", dev).CombinedOutput()
				if err != nil {
					cleanup()
					return nil, fmt.Errorf("lvcreate für %s: %v %s", dev, err, strings.TrimSpace(string(out)))
				}
				read = filepath.Join(filepath.Dir(dev), name)
				snaps = append(snaps, read)
				rc.logf2("info", "LVM-Snapshot erstellt: %s", read)
			}
			name := streamName("_streams/image/", strings.TrimPrefix(dev, "/"))
			if !strings.HasSuffix(name, ".img") {
				name += ".img"
			}
			opt.Streams = append(opt.Streams, fileStream(name, read))
		}
		return cleanup, nil

	case "sysimage":
		if runtime.GOOS != "windows" {
			return nil, errors.New("Systemabbilder über Windows Server Backup gibt es nur unter Windows")
		}
		target := strings.TrimRight(rc.job.TargetPath, `\`)
		if target == "" {
			return nil, errors.New("Zwischenspeicher (Laufwerk wie E: oder UNC-Freigabe) für wbadmin angeben")
		}
		args := []string{"start", "backup", "-backupTarget:" + target, "-quiet"}
		if v := strings.TrimSpace(s.SysVolumes); v != "" && v != "-allCritical" {
			args = append(args, "-include:"+v)
		} else {
			args = append(args, "-allCritical")
		}
		rc.phase("Windows Server Backup: Systemabbild (Bare Metal) erstellen")
		out, err := exec.CommandContext(ctx, "wbadmin", args...).CombinedOutput()
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if strings.TrimSpace(l) != "" {
				rc.log("info", "  "+strings.TrimSpace(l))
			}
		}
		if err != nil {
			return nil, fmt.Errorf("wbadmin: %v (Feature «Windows Server-Sicherung» installiert?)", err)
		}
		host, _ := os.Hostname()
		dir := filepath.Join(target+`\`, "WindowsImageBackup", host)
		if _, err := os.Stat(dir); err != nil {
			dir = filepath.Join(target+`\`, "WindowsImageBackup")
		}
		opt.Paths = []string{dir}
		opt.Kind = "sysimage"
		return nil, nil
	}
	return nil, fmt.Errorf("unbekannte Quelle %q", s.Kind)
}
