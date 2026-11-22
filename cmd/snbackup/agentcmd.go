package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/swissnand/snbackup/internal/agent"
)

func defaultAgentDir() string {
	if v := os.Getenv("SNB_AGENT_DATA"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		pd := os.Getenv("ProgramData")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		return filepath.Join(pd, "SWISSNAND", "Backup Agent")
	}
	if os.Geteuid() == 0 {
		return "/var/lib/snbackup-agent"
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "snbackup-agent")
}

func cmdAgent(args []string) {
	if len(args) == 0 {
		die(errors.New("agent enroll | run"))
	}
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	data := fs.String("data", defaultAgentDir(), "Datenverzeichnis des Agents")
	server := fs.String("server", os.Getenv("SNB_SERVER"), "URL des Backup-Servers, z. B. https://backup01:8550")
	token := fs.String("token", os.Getenv("SNB_ENROLL_TOKEN"), "Registrierungs-Token aus der Web-Konsole")
	fp := fs.String("fingerprint", os.Getenv("SNB_FINGERPRINT"), "Fingerabdruck des Server-Zertifikats (sha256:…)")
	name := fs.String("name", os.Getenv("SNB_AGENT_NAME"), "Anzeigename (Standard: Hostname)")
	hostRoot := fs.String("host-root", os.Getenv("SNB_HOST_ROOT"), "Container: Pfad, unter dem das Host-Dateisystem gemountet ist (z. B. /host)")
	parse(fs, args[1:])
	switch args[0] {
	case "enroll":
		if *server == "" || *token == "" {
			die(errors.New("--server und --token angeben"))
		}
		c, err := agent.Enroll(*data, *server, *token, *fp, *name)
		if err != nil {
			die(err)
		}
		fmt.Printf("%s Agent «%s» registriert (%s)\n", green("✓"), c.Name, c.AgentID)
		fmt.Println("  Starten: snbackup agent run   ·   Als Dienst: snbackup service install --agent")
	case "run":
		c, err := agent.LoadConfig(*data)
		if err != nil {
			// Container/Kubernetes: enroll automatically on first start.
			if *server == "" || *token == "" {
				die(fmt.Errorf("nicht registriert – zuerst «snbackup agent enroll» ausführen (%v)", err))
			}
			if c, err = agent.Enroll(*data, *server, *token, *fp, *name); err != nil {
				die(err)
			}
		}
		banner()
		a, err := agent.New(*data, c, *hostRoot)
		if err != nil {
			die(err)
		}
		ctx, cancel := signalCtx()
		defer cancel()
		if err := a.Run(ctx); err != nil {
			die(err)
		}
	default:
		die(errors.New("agent enroll | run"))
	}
}
