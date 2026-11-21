//go:build windows

package jobs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// createShadows creates a Volume Shadow Copy for every volume used by the
// given paths and returns a path mapper so open files (databases, Outlook
// PST, Exchange, SQL Server, Hyper-V configs ...) are read consistently.
func createShadows(ctx context.Context, paths []string, log func(level, msg string)) (func(string) string, func(), error) {
	vols := map[string]bool{}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, nil, err
		}
		v := filepath.VolumeName(abs)
		if v == "" || strings.HasPrefix(v, `\\`) {
			continue
		}
		vols[strings.ToUpper(v)] = true
	}
	type shadow struct{ vol, id, link string }
	var shadows []shadow
	cleanup := func() {
		for _, s := range shadows {
			_ = os.Remove(s.link)
			script := fmt.Sprintf(`Get-CimInstance Win32_ShadowCopy | Where-Object { $_.ID -eq '%s' } | Remove-CimInstance`, s.id)
			if out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput(); err != nil {
				log("warn", "VSS-Schattenkopie konnte nicht entfernt werden: "+strings.TrimSpace(string(out)))
			} else {
				log("info", "VSS-Schattenkopie entfernt: "+s.vol)
			}
		}
	}
	base := filepath.Join(os.Getenv("ProgramData"), "SWISSNAND", "Backup", "vss")
	_ = os.MkdirAll(base, 0o700)
	for v := range vols {
		script := fmt.Sprintf(
			`$res = Invoke-CimMethod -ClassName Win32_ShadowCopy -MethodName Create -Arguments @{Volume='%s\'; Context='ClientAccessible'}; `+
				`if ($res.ReturnValue -ne 0) { throw "VSS ReturnValue $($res.ReturnValue)" }; `+
				`$s = Get-CimInstance Win32_ShadowCopy | Where-Object { $_.ID -eq $res.ShadowID }; `+
				`Write-Output "$($s.ID)|$($s.DeviceObject)"`, v)
		out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("%s: %v %s (Dienst als Administrator/SYSTEM ausführen)", v, err, strings.TrimSpace(string(out)))
		}
		parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
		if len(parts) != 2 {
			cleanup()
			return nil, nil, fmt.Errorf("unerwartete VSS-Antwort: %s", out)
		}
		link := filepath.Join(base, strings.TrimSuffix(v, ":")+"-"+fmt.Sprint(os.Getpid()))
		_ = os.Remove(link)
		if o, err := exec.Command("cmd.exe", "/c", "mklink", "/d", link, parts[1]+`\`).CombinedOutput(); err != nil {
			shadows = append(shadows, shadow{v, parts[0], link})
			cleanup()
			return nil, nil, fmt.Errorf("mklink: %v %s", err, o)
		}
		shadows = append(shadows, shadow{v, parts[0], link})
		log("info", "VSS-Schattenkopie erstellt für "+v)
	}
	mapper := func(p string) string {
		abs, _ := filepath.Abs(p)
		v := strings.ToUpper(filepath.VolumeName(abs))
		for _, s := range shadows {
			if s.vol == v {
				return filepath.Join(s.link, strings.TrimPrefix(abs[len(v):], `\`))
			}
		}
		return p
	}
	return mapper, cleanup, nil
}
