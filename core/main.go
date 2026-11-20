package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:])
	case "ca":
		cmdCA(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `httpx-core — HTTP/HTTPS 抓包核心

用法:
  httpx-core serve [--listen 127.0.0.1:8899] [--port 8900] [--data-dir data] [--max-body 1048576]
  httpx-core ca [--data-dir data] [--install]

serve 启动代理 + 控制/事件服务；ca 管理根证书（--install 装入当前用户 ROOT 库）。
`)
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	listen := fs.String("listen", "127.0.0.1:8899", "proxy listen address")
	port := fs.Int("port", 8900, "control/event tcp port")
	dataDir := fs.String("data-dir", "data", "directory for CA files")
	maxBody := fs.Int("max-body", 1<<20, "max recorded body bytes per side")
	_ = fs.Parse(args)

	ca, err := LoadOrCreateCA(*dataDir)
	if err != nil {
		log.Fatalf("ca: %v", err)
	}
	store := NewSessionStore(5000)
	interceptor := NewInterceptor(nil)
	engine := NewRedirectEngine()
	proxy := NewProxy(ca, store, *maxBody, interceptor, engine)
	transparent := NewTransparentServer(proxy, engine)
	es := NewEventServer(proxy, store, interceptor, transparent, *dataDir, *maxBody)
	interceptor.broadcast = es.broadcast
	if err := es.Start(*port); err != nil {
		log.Fatalf("event server: %v", err)
	}
	log.Printf("httpx-core ready: %s", ca.Info())
	log.Printf("proxy listen=%s event=127.0.0.1:%d caInstalled=%v",
		*listen, *port, CAInstalled(ca.CertPath()))
	select {}
}

func cmdCA(args []string) {
	fs := flag.NewFlagSet("ca", flag.ExitOnError)
	dataDir := fs.String("data-dir", "data", "directory for CA files")
	install := fs.Bool("install", false, "install CA to current-user Root store")
	_ = fs.Parse(args)

	ca, err := LoadOrCreateCA(*dataDir)
	if err != nil {
		log.Fatalf("ca: %v", err)
	}
	fmt.Printf("cert:     %s\n", ca.CertPath())
	fmt.Printf("key:      %s\n", ca.KeyPath())
	fmt.Printf("installed: %v\n", CAInstalled(ca.CertPath()))
	if *install {
		if err := InstallCA(ca.CertPath()); err != nil {
			log.Fatalf("install: %v", err)
		}
		fmt.Println("installed to current-user Root store.")
	}
}
