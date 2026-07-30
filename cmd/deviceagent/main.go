package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Not-Satya/sync_engine/internal/device/agent"
	"github.com/Not-Satya/sync_engine/internal/device/client"
	"github.com/Not-Satya/sync_engine/internal/device/keystore"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("deviceagent: ")

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "status":
		os.Exit(cmdStatus(os.Args[2:]))
	case "run":
		os.Exit(cmdRun(os.Args[2:]))
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `deviceagent — local device stub for the sync coordinator

Usage:
  deviceagent status [-keystore path] [-passphrase s]
  deviceagent run    [-keystore path] [-passphrase s] [-interval 20s] [-endpoint addr]

Commands:
  status   Load keystore, GET /v1/me, print identity
  run      Load keystore and heartbeat until interrupted (Ctrl+C)

Register / login / pair land in a later slice (P2.7).
`)
}

func cmdStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	ksPath := fs.String("keystore", "", "keystore path (default: user config dir)")
	pass := fs.String("passphrase", "", "keystore passphrase if wrap=passphrase")
	_ = fs.Parse(args)

	rec, path, err := loadKeystore(*ksPath, *pass)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := client.New(rec.CoordURL, rec.Secrets.Token)
	me, err := c.Me(ctx)
	if err != nil {
		log.Printf("keystore=%s device=%s — /me failed: %v", path, rec.DeviceID, err)
		return 1
	}

	fmt.Printf("keystore:  %s\n", path)
	fmt.Printf("coord_url: %s\n", rec.CoordURL)
	fmt.Printf("user_id:   %s\n", me.UserID)
	fmt.Printf("email:     %s\n", me.Email)
	fmt.Printf("device_id: %s\n", rec.DeviceID)
	if name, ok := me.Device["name"].(string); ok {
		fmt.Printf("name:      %s\n", name)
	}
	return 0
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	ksPath := fs.String("keystore", "", "keystore path (default: user config dir)")
	pass := fs.String("passphrase", "", "keystore passphrase if wrap=passphrase")
	interval := fs.Duration("interval", agent.DefaultHeartbeatInterval, "heartbeat interval")
	endpoint := fs.String("endpoint", "", "optional presence endpoint hint")
	_ = fs.Parse(args)

	rec, path, err := loadKeystore(*ksPath, *pass)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	c := client.New(rec.CoordURL, rec.Secrets.Token)
	if err := c.Healthz(ctx); err != nil {
		log.Printf("coordinator unreachable (%s): %v", rec.CoordURL, err)
		return 1
	}

	me, err := c.Me(ctx)
	if err != nil {
		log.Printf("/me failed: %v", err)
		return 1
	}
	log.Printf("linked as %s (%s) via %s [keystore=%s]", me.UserID, rec.DeviceID, rec.CoordURL, path)

	err = agent.Run(ctx, agent.Config{
		Client:   c,
		Interval: *interval,
		Endpoint: *endpoint,
	})
	if err != nil && err != context.Canceled {
		log.Printf("agent stopped: %v", err)
		return 1
	}
	log.Printf("stopped")
	return 0
}

func loadKeystore(path, passphrase string) (keystore.Record, string, error) {
	if path == "" {
		p, err := keystore.DefaultPath()
		if err != nil {
			return keystore.Record{}, "", err
		}
		path = p
	}
	rec, err := keystore.Load(path, passphrase)
	if err != nil {
		if err == keystore.ErrNotFound {
			return keystore.Record{}, path, fmt.Errorf("%w — link a device first (P2.7 adds register/login/pair)", err)
		}
		return keystore.Record{}, path, err
	}
	return rec, path, nil
}
