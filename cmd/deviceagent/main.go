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
	case "register":
		os.Exit(cmdRegister(os.Args[2:]))
	case "login":
		os.Exit(cmdLogin(os.Args[2:]))
	case "pair-code":
		os.Exit(cmdPairCode(os.Args[2:]))
	case "pair":
		os.Exit(cmdPair(os.Args[2:]))
	case "devices":
		os.Exit(cmdDevices(os.Args[2:]))
	case "revoke":
		os.Exit(cmdRevoke(os.Args[2:]))
	case "logout":
		os.Exit(cmdLogout(os.Args[2:]))
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
	fmt.Fprintf(os.Stderr, `deviceagent — local device agent for the sync coordinator

Usage:
  deviceagent register -email e -password p -name Laptop [-coord URL] [-keystore path] [-passphrase s]
  deviceagent login    -email e -password p -name Phone  [-coord URL] [-keystore path] [-passphrase s]
  deviceagent pair-code [-keystore path] [-passphrase s]
  deviceagent pair     -code CODE -name Tablet [-coord URL] [-keystore path] [-passphrase s]
  deviceagent devices  [-keystore path] [-passphrase s]
  deviceagent revoke   -device-id ID [-keystore path] [-passphrase s]
  deviceagent logout   [-keystore path] [-passphrase s]
  deviceagent status   [-keystore path] [-passphrase s]
  deviceagent run      [-keystore path] [-passphrase s] [-interval 20s] [-endpoint addr]

Commands:
  register   Create account + first device; save encrypted keystore
  login      Link another device with account password; save keystore
  pair-code  (linked device) print a short-lived pairing code
  pair       Link this device with a pairing code; save keystore
  devices    List account devices (marks this device / revoked)
  revoke     Soft-revoke another device (or self with -device-id)
  logout     Revoke this device on the server and delete local keystore
  status     Load keystore, GET /v1/me
  run        Heartbeat loop until Ctrl+C
`)
}

func cmdRegister(args []string) int {
	fs := flag.NewFlagSet("register", flag.ExitOnError)
	coord := fs.String("coord", "http://localhost:8080", "coordinator base URL")
	email := fs.String("email", "", "account email")
	password := fs.String("password", "", "account password (>=8)")
	name := fs.String("name", "", "device display name")
	platform := fs.String("platform", "", "device platform (default: GOOS)")
	ksPath := fs.String("keystore", "", "keystore path")
	pass := fs.String("passphrase", "", "optional keystore passphrase wrap")
	_ = fs.Parse(args)

	if *email == "" || *password == "" || *name == "" {
		log.Printf("-email, -password, and -name are required")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c := client.New(*coord, "")
	link, err := c.Register(ctx, *email, *password, client.DeviceInfo{Name: *name, Platform: *platform})
	if err != nil {
		log.Printf("register failed: %v", err)
		return 1
	}
	path, err := saveLink(*ksPath, *coord, *pass, link)
	if err != nil {
		log.Printf("save keystore: %v", err)
		return 1
	}
	fmt.Printf("registered user=%s device=%s\nkeystore=%s\n", link.UserID, link.DeviceID, path)
	return 0
}

func cmdLogin(args []string) int {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	coord := fs.String("coord", "http://localhost:8080", "coordinator base URL")
	email := fs.String("email", "", "account email")
	password := fs.String("password", "", "account password")
	name := fs.String("name", "", "device display name")
	platform := fs.String("platform", "", "device platform (default: GOOS)")
	ksPath := fs.String("keystore", "", "keystore path")
	pass := fs.String("passphrase", "", "optional keystore passphrase wrap")
	_ = fs.Parse(args)

	if *email == "" || *password == "" || *name == "" {
		log.Printf("-email, -password, and -name are required")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c := client.New(*coord, "")
	link, err := c.Login(ctx, *email, *password, client.DeviceInfo{Name: *name, Platform: *platform})
	if err != nil {
		log.Printf("login failed: %v", err)
		return 1
	}
	path, err := saveLink(*ksPath, *coord, *pass, link)
	if err != nil {
		log.Printf("save keystore: %v", err)
		return 1
	}
	fmt.Printf("linked user=%s device=%s\nkeystore=%s\n", link.UserID, link.DeviceID, path)
	return 0
}

func cmdPairCode(args []string) int {
	fs := flag.NewFlagSet("pair-code", flag.ExitOnError)
	ksPath := fs.String("keystore", "", "keystore path")
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
	code, exp, err := c.CreatePairingCode(ctx)
	if err != nil {
		log.Printf("pair-code failed: %v", err)
		return 1
	}
	fmt.Printf("pairing code: %s\nexpires_at:  %s\n(from keystore %s)\n", code, exp.UTC().Format(time.RFC3339), path)
	return 0
}

func cmdPair(args []string) int {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	coord := fs.String("coord", "http://localhost:8080", "coordinator base URL")
	code := fs.String("code", "", "pairing code from another device")
	name := fs.String("name", "", "device display name")
	platform := fs.String("platform", "", "device platform (default: GOOS)")
	ksPath := fs.String("keystore", "", "keystore path")
	pass := fs.String("passphrase", "", "optional keystore passphrase wrap")
	_ = fs.Parse(args)

	if *code == "" || *name == "" {
		log.Printf("-code and -name are required")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c := client.New(*coord, "")
	link, err := c.RedeemPairingCode(ctx, *code, client.DeviceInfo{Name: *name, Platform: *platform})
	if err != nil {
		log.Printf("pair failed: %v", err)
		return 1
	}
	path, err := saveLink(*ksPath, *coord, *pass, link)
	if err != nil {
		log.Printf("save keystore: %v", err)
		return 1
	}
	fmt.Printf("paired user=%s device=%s\nkeystore=%s\n", link.UserID, link.DeviceID, path)
	return 0
}

func cmdDevices(args []string) int {
	fs := flag.NewFlagSet("devices", flag.ExitOnError)
	ksPath := fs.String("keystore", "", "keystore path")
	pass := fs.String("passphrase", "", "keystore passphrase if wrap=passphrase")
	_ = fs.Parse(args)

	rec, _, err := loadKeystore(*ksPath, *pass)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := client.New(rec.CoordURL, rec.Secrets.Token)
	list, err := c.ListDevices(ctx)
	if err != nil {
		log.Printf("devices failed: %v", err)
		return 1
	}

	fmt.Printf("this_device_id=%s  active=%d  total=%d\n", list.ThisDeviceID, list.ActiveCount, list.TotalCount)
	for _, d := range list.Devices {
		marker := " "
		if d.IsThisDevice {
			marker = "*"
		}
		state := "active"
		if d.Revoked {
			state = "revoked"
		}
		fmt.Printf("%s %s  %-16s  %-10s  %s  last_seen=%s\n",
			marker, d.DeviceID, d.Name, d.Platform, state, d.LastSeenAt.UTC().Format(time.RFC3339))
	}
	return 0
}

func cmdRevoke(args []string) int {
	fs := flag.NewFlagSet("revoke", flag.ExitOnError)
	deviceID := fs.String("device-id", "", "device to revoke")
	ksPath := fs.String("keystore", "", "keystore path")
	pass := fs.String("passphrase", "", "keystore passphrase if wrap=passphrase")
	_ = fs.Parse(args)

	if *deviceID == "" {
		log.Printf("-device-id is required")
		return 2
	}

	rec, path, err := loadKeystore(*ksPath, *pass)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := client.New(rec.CoordURL, rec.Secrets.Token)
	rev, err := c.RevokeDevice(ctx, *deviceID)
	if err != nil {
		log.Printf("revoke failed: %v", err)
		return 1
	}

	fmt.Printf("revoked device=%s name=%s\n", rev.DeviceID, rev.Name)

	// Self-revoke: drop local credentials so this agent cannot keep using a dead token.
	if *deviceID == rec.DeviceID {
		if err := keystore.Remove(path); err != nil {
			log.Printf("revoked on server but failed to delete keystore %s: %v", path, err)
			return 1
		}
		fmt.Printf("cleared local keystore %s\n", path)
	}
	return 0
}

func cmdLogout(args []string) int {
	fs := flag.NewFlagSet("logout", flag.ExitOnError)
	ksPath := fs.String("keystore", "", "keystore path")
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
	if _, err := c.RevokeDevice(ctx, rec.DeviceID); err != nil {
		log.Printf("server revoke failed (still clearing local keystore): %v", err)
	} else {
		fmt.Printf("revoked device=%s on coordinator\n", rec.DeviceID)
	}

	if err := keystore.Remove(path); err != nil {
		log.Printf("delete keystore %s: %v", path, err)
		return 1
	}
	fmt.Printf("cleared local keystore %s\n", path)
	return 0
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

func saveLink(path, coordURL, passphrase string, link client.LinkResult) (string, error) {
	if path == "" {
		p, err := keystore.DefaultPath()
		if err != nil {
			return "", err
		}
		path = p
	}
	opts := keystore.Options{}
	if passphrase != "" {
		opts.Method = keystore.WrapPassphrase
		opts.Passphrase = passphrase
	}
	rec := keystore.Record{
		UserID:    link.UserID,
		DeviceID:  link.DeviceID,
		CoordURL:  coordURL,
		PublicKey: link.PublicKey,
		Secrets: keystore.Secrets{
			PrivateKey: link.PrivateKey,
			Token:      link.Token,
		},
	}
	if err := keystore.Save(path, rec, opts); err != nil {
		return path, err
	}
	return path, nil
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
			return keystore.Record{}, path, fmt.Errorf("%w — run register, login, or pair first", err)
		}
		return keystore.Record{}, path, err
	}
	return rec, path, nil
}
