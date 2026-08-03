package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Not-Satya/sync_engine/internal/device/bindings"
	"github.com/Not-Satya/sync_engine/internal/device/client"
)

func cmdFolders(args []string) int {
	if len(args) < 1 {
		foldersUsage()
		return 2
	}
	switch args[0] {
	case "create":
		return cmdFoldersCreate(args[1:])
	case "list":
		return cmdFoldersList(args[1:])
	case "status":
		return cmdFoldersStatus(args[1:])
	case "subscribe":
		return cmdFoldersSubscribe(args[1:])
	case "unsubscribe":
		return cmdFoldersUnsubscribe(args[1:])
	case "bind":
		return cmdFoldersBind(args[1:])
	case "unbind":
		return cmdFoldersUnbind(args[1:])
	case "add":
		return cmdFoldersAdd(args[1:])
	case "help", "-h", "--help":
		foldersUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown folders subcommand %q\n", args[0])
		foldersUsage()
		return 2
	}
}

func foldersUsage() {
	fmt.Fprintf(os.Stderr, `deviceagent folders — sync folder + local path bindings

Usage:
  deviceagent folders create      -name Movies
  deviceagent folders list
  deviceagent folders status
  deviceagent folders subscribe   -folder-id ID
  deviceagent folders unsubscribe -folder-id ID
  deviceagent folders bind        -folder-id ID -path DIR
  deviceagent folders unbind      -folder-id ID
  deviceagent folders add         -name Movies -path DIR

  Common flags: [-keystore path] [-passphrase s] [-bindings path]

Notes:
  create/subscribe talk to the coordinator (no local path).
  bind/unbind update device-local folder_bindings.json only.
  add = create + subscribe + bind (convenience).
  list = account folders from server + local path if bound.
  status = local bindings + path health (no fsnotify / no file sync yet).
  Paths must exist and be directories (validated on bind/add).
`)
}

func cmdFoldersCreate(args []string) int {
	fs := flag.NewFlagSet("folders create", flag.ExitOnError)
	name := fs.String("name", "", "folder display name")
	ksPath, pass, _ := addAuthFlags(fs)
	_ = fs.Parse(args)
	if *name == "" {
		log.Printf("-name is required")
		return 2
	}

	c, _, err := coordFromKeystore(*ksPath, *pass)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	f, err := c.CreateFolder(ctx, *name)
	if err != nil {
		log.Printf("create folder failed: %v", err)
		return 1
	}
	fmt.Printf("created folder_id=%s name=%s (this device auto-subscribed on server)\n", f.FolderID, f.Name)
	return 0
}

func cmdFoldersList(args []string) int {
	fs := flag.NewFlagSet("folders list", flag.ExitOnError)
	ksPath, pass, bindPath := addAuthFlags(fs)
	_ = fs.Parse(args)

	c, _, err := coordFromKeystore(*ksPath, *pass)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}
	store, err := openBindings(*bindPath)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	folders, err := c.ListFolders(ctx)
	if err != nil {
		log.Printf("list folders failed: %v", err)
		return 1
	}
	subs, err := c.ListSubscriptions(ctx)
	if err != nil {
		log.Printf("list subscriptions failed: %v", err)
		return 1
	}
	subSet := map[string]bool{}
	for _, s := range subs {
		subSet[s.FolderID] = true
	}
	local, err := store.List()
	if err != nil {
		log.Printf("list bindings failed: %v", err)
		return 1
	}
	bindByID := map[string]bindings.Binding{}
	for _, b := range local {
		bindByID[b.FolderID] = b
	}

	if len(folders) == 0 {
		fmt.Println("(no folders on account)")
		return 0
	}
	for _, f := range folders {
		sub := "no"
		if subSet[f.FolderID] {
			sub = "yes"
		}
		path := "-"
		if b, ok := bindByID[f.FolderID]; ok {
			path = b.LocalPath
		}
		fmt.Printf("%s  %-20s  subscribed=%-3s  path=%s\n", f.FolderID, f.Name, sub, path)
	}
	return 0
}

func cmdFoldersStatus(args []string) int {
	fs := flag.NewFlagSet("folders status", flag.ExitOnError)
	ksPath, pass, bindPath := addAuthFlags(fs)
	_ = fs.Parse(args)

	// Keystore proves this machine is linked; status itself is local-only.
	if _, _, err := loadKeystore(*ksPath, *pass); err != nil {
		log.Printf("%v", err)
		return 1
	}
	store, err := openBindings(*bindPath)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}

	rows, err := store.Status()
	if err != nil {
		log.Printf("status failed: %v", err)
		return 1
	}
	if len(rows) == 0 {
		fmt.Println("(no local folder bindings)")
		fmt.Println("hint: deviceagent folders add -name Movies -path <dir>")
		return 0
	}

	ok, bad := 0, 0
	for _, r := range rows {
		fmt.Println(r.String())
		if r.Health == bindings.PathOK {
			ok++
		} else {
			bad++
		}
	}
	fmt.Printf("summary: %d binding(s), %d ok, %d problem(s) — watching starts in Phase 4\n", len(rows), ok, bad)
	if bad > 0 {
		return 1
	}
	return 0
}

func cmdFoldersSubscribe(args []string) int {
	fs := flag.NewFlagSet("folders subscribe", flag.ExitOnError)
	folderID := fs.String("folder-id", "", "folder id")
	ksPath, pass, bindPath := addAuthFlags(fs)
	_ = fs.Parse(args)
	if *folderID == "" {
		log.Printf("-folder-id is required")
		return 2
	}

	c, _, err := coordFromKeystore(*ksPath, *pass)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sub, err := c.SubscribeFolder(ctx, *folderID)
	if err != nil {
		log.Printf("subscribe failed: %v", err)
		return 1
	}
	store, err := openBindings(*bindPath)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}
	if _, err := store.Get(*folderID); err == nil {
		_ = store.SetSubscribed(*folderID, true)
	}
	fmt.Printf("subscribed folder_id=%s device_id=%s\n", sub.FolderID, sub.DeviceID)
	return 0
}

func cmdFoldersUnsubscribe(args []string) int {
	fs := flag.NewFlagSet("folders unsubscribe", flag.ExitOnError)
	folderID := fs.String("folder-id", "", "folder id")
	ksPath, pass, bindPath := addAuthFlags(fs)
	_ = fs.Parse(args)
	if *folderID == "" {
		log.Printf("-folder-id is required")
		return 2
	}

	c, _, err := coordFromKeystore(*ksPath, *pass)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.UnsubscribeFolder(ctx, *folderID); err != nil {
		log.Printf("unsubscribe failed: %v", err)
		return 1
	}
	store, err := openBindings(*bindPath)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}
	if _, err := store.Get(*folderID); err == nil {
		_ = store.SetSubscribed(*folderID, false)
	}
	fmt.Printf("unsubscribed folder_id=%s (local path binding kept if any)\n", *folderID)
	return 0
}

func cmdFoldersBind(args []string) int {
	fs := flag.NewFlagSet("folders bind", flag.ExitOnError)
	folderID := fs.String("folder-id", "", "folder id")
	path := fs.String("path", "", "local directory path")
	name := fs.String("name", "", "optional display name for local record")
	ksPath, pass, bindPath := addAuthFlags(fs)
	_ = fs.Parse(args)
	if *folderID == "" || *path == "" {
		log.Printf("-folder-id and -path are required")
		return 2
	}

	if _, _, err := loadKeystore(*ksPath, *pass); err != nil {
		log.Printf("%v", err)
		return 1
	}

	store, err := openBindings(*bindPath)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}
	b := bindings.Binding{
		FolderID:   *folderID,
		LocalPath:  *path,
		Name:       *name,
		Subscribed: true,
		BoundAt:    time.Now().UTC(),
	}
	if err := store.PutValidated(b); err != nil {
		log.Printf("bind failed: %v", err)
		return 1
	}
	got, _ := store.Get(*folderID)
	fmt.Printf("bound folder_id=%s path=%s\n", got.FolderID, got.LocalPath)
	return 0
}

func cmdFoldersUnbind(args []string) int {
	fs := flag.NewFlagSet("folders unbind", flag.ExitOnError)
	folderID := fs.String("folder-id", "", "folder id")
	ksPath, pass, bindPath := addAuthFlags(fs)
	_ = fs.Parse(args)
	if *folderID == "" {
		log.Printf("-folder-id is required")
		return 2
	}
	if _, _, err := loadKeystore(*ksPath, *pass); err != nil {
		log.Printf("%v", err)
		return 1
	}
	store, err := openBindings(*bindPath)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}
	if err := store.Remove(*folderID); err != nil {
		log.Printf("unbind failed: %v", err)
		return 1
	}
	fmt.Printf("unbound folder_id=%s (server subscription unchanged)\n", *folderID)
	return 0
}

func cmdFoldersAdd(args []string) int {
	fs := flag.NewFlagSet("folders add", flag.ExitOnError)
	name := fs.String("name", "", "folder display name")
	path := fs.String("path", "", "local directory path")
	ksPath, pass, bindPath := addAuthFlags(fs)
	_ = fs.Parse(args)
	if *name == "" || *path == "" {
		log.Printf("-name and -path are required")
		return 2
	}

	c, _, err := coordFromKeystore(*ksPath, *pass)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}

	// Validate path before creating anything on the server.
	normalized, err := bindings.ValidateAndNormalizePath(*path)
	if err != nil {
		log.Printf("invalid path: %v", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	f, err := c.CreateFolder(ctx, *name)
	if err != nil {
		log.Printf("create folder failed: %v", err)
		return 1
	}
	// Create already auto-subscribes; explicit subscribe is idempotent.
	if _, err := c.SubscribeFolder(ctx, f.FolderID); err != nil {
		log.Printf("subscribe failed: %v", err)
		return 1
	}

	store, err := openBindings(*bindPath)
	if err != nil {
		log.Printf("%v", err)
		return 1
	}
	if err := store.Put(bindings.Binding{
		FolderID:   f.FolderID,
		LocalPath:  normalized,
		Name:       f.Name,
		Subscribed: true,
		BoundAt:    time.Now().UTC(),
	}); err != nil {
		log.Printf("bind failed: %v", err)
		return 1
	}
	fmt.Printf("added folder_id=%s name=%s path=%s\n", f.FolderID, f.Name, normalized)
	return 0
}

func addAuthFlags(fs *flag.FlagSet) (ksPath, pass, bindPath *string) {
	ksPath = fs.String("keystore", "", "keystore path")
	pass = fs.String("passphrase", "", "keystore passphrase if wrap=passphrase")
	bindPath = fs.String("bindings", "", "folder bindings path (default: user config dir)")
	return
}

func coordFromKeystore(ksPath, passphrase string) (*client.Client, string, error) {
	rec, path, err := loadKeystore(ksPath, passphrase)
	if err != nil {
		return nil, "", err
	}
	return client.New(rec.CoordURL, rec.Secrets.Token), path, nil
}

func openBindings(path string) (*bindings.Store, error) {
	if path == "" {
		p, err := bindings.DefaultPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	return bindings.Open(path), nil
}
