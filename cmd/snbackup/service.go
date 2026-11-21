package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const systemdUnit = `[Unit]
Description=SWISSNAND Backup Server
Documentation=https://github.com/swissnand/snbackup
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s server --data %s --listen %s
Restart=on-failure
RestartSec=10
Nice=5
IOSchedulingClass=best-effort
IOSchedulingPriority=6
# Hardening (root is needed to read all files for system backups)
NoNewPrivileges=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
UMask=0077

[Install]
WantedBy=multi-user.target
`

func cmdService(args []string) {
	if len(args) == 0 {
		die(errors.New("service install | uninstall | status"))
	}
	fs := flag.NewFlagSet("service", flag.ExitOnError)
	data := fs.String("data", defaultDataDir(), "Datenverzeichnis")
	listen := fs.String("listen", ":8550", "Adresse der Web-Konsole")
	parse(fs, args[1:])
	exe, err := os.Executable()
	if err != nil {
		die(err)
	}
	exe, _ = filepath.EvalSymlinks(exe)
	switch runtime.GOOS {
	case "linux":
		linuxService(args[0], exe, *data, *listen)
	case "windows":
		windowsService(args[0], exe, *data, *listen)
	default:
		die(errors.New("Dienstinstallation wird auf diesem System nicht unterstützt"))
	}
}

func run(name string, a ...string) error {
	cmd := exec.Command(name, a...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func linuxService(action, exe, data, listen string) {
	unit := "/etc/systemd/system/snbackup.service"
	switch action {
	case "install":
		if os.Geteuid() != 0 {
			die(errors.New("bitte mit sudo ausführen"))
		}
		if err := os.MkdirAll(data, 0o700); err != nil {
			die(err)
		}
		if err := os.WriteFile(unit, []byte(fmt.Sprintf(systemdUnit, exe, data, listen)), 0o644); err != nil {
			die(err)
		}
		_ = run("systemctl", "daemon-reload")
		if err := run("systemctl", "enable", "--now", "snbackup"); err != nil {
			die(err)
		}
		fmt.Println(green("✓") + " Dienst snbackup installiert und gestartet")
		fmt.Println("  Status:   systemctl status snbackup")
		fmt.Println("  Protokoll: journalctl -u snbackup -f")
		fmt.Println("  Erstes Admin-Passwort: sudo cat " + filepath.Join(data, "initial-admin-password"))
	case "uninstall":
		_ = run("systemctl", "disable", "--now", "snbackup")
		_ = os.Remove(unit)
		_ = run("systemctl", "daemon-reload")
		fmt.Println(green("✓") + " Dienst entfernt (Daten in " + data + " bleiben erhalten)")
	case "status":
		_ = run("systemctl", "status", "snbackup", "--no-pager")
	default:
		die(errors.New("install | uninstall | status"))
	}
}

// On Windows the server runs as a SYSTEM task that starts at boot and is
// restarted automatically – no third-party service wrapper needed.
func windowsService(action, exe, data, listen string) {
	const task = `SWISSNAND Backup`
	switch action {
	case "install":
		if err := os.MkdirAll(data, 0o700); err != nil {
			die(err)
		}
		// Restrict the data folder to SYSTEM and Administrators.
		_ = run("icacls", data, "/inheritance:r", "/grant:r", "*S-1-5-18:(OI)(CI)F", "/grant:r", "*S-1-5-32-544:(OI)(CI)F")
		ps := fmt.Sprintf(`$a = New-ScheduledTaskAction -Execute '%s' -Argument 'server --data "%s" --listen %s';`+
			`$t = New-ScheduledTaskTrigger -AtStartup;`+
			`$s = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -StartWhenAvailable;`+
			`$p = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest;`+
			`Register-ScheduledTask -TaskName '%s' -Action $a -Trigger $t -Settings $s -Principal $p -Force | Out-Null;`+
			`Start-ScheduledTask -TaskName '%s';`+
			`New-NetFirewallRule -DisplayName 'SWISSNAND Backup Web-Konsole' -Direction Inbound -Protocol TCP -LocalPort %s -Action Allow -Profile Domain,Private -ErrorAction SilentlyContinue | Out-Null`,
			exe, data, listen, task, task, strings.TrimPrefix(listen[strings.LastIndex(listen, ":"):], ":"))
		if err := run("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps); err != nil {
			die(fmt.Errorf("%v (PowerShell als Administrator ausführen)", err))
		}
		fmt.Println(green("✓") + " SWISSNAND Backup läuft als SYSTEM-Dienst (Aufgabenplanung, Autostart)")
		fmt.Println("  Web-Konsole: https://localhost" + listen[strings.LastIndex(listen, ":"):])
		fmt.Println("  Erstes Admin-Passwort: " + filepath.Join(data, "initial-admin-password"))
	case "uninstall":
		_ = run("powershell.exe", "-NoProfile", "-Command", fmt.Sprintf(`Stop-ScheduledTask -TaskName '%s' -ErrorAction SilentlyContinue; Unregister-ScheduledTask -TaskName '%s' -Confirm:$false`, task, task))
		fmt.Println(green("✓") + " Dienst entfernt")
	case "status":
		_ = run("powershell.exe", "-NoProfile", "-Command", fmt.Sprintf(`Get-ScheduledTask -TaskName '%s' | Get-ScheduledTaskInfo`, task))
	default:
		die(errors.New("install | uninstall | status"))
	}
}
