package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/bettercap/readline"
	cli "github.com/jawher/mow.cli"
	"github.com/miekg/dns"
)

func main() {
	app := cli.App("dnsapp", "DNS Server with optional CLI mode")
	app.Version("v version", fmt.Sprintf("DNS Resolver %s", appversion))

	// Command-line options
	daemon := app.BoolOpt("d daemon", false, "Run as daemon (no interactive mode)")
	port := app.StringOpt("p port", "53", "Port for DNS server")
	remoteUnix := app.StringOpt("u remote-unix", "/tmp/dnsresolver.socket", "Path to UNIX domain socket")
	clientMode := app.BoolOpt("c client-mode", false, "Run in client mode (connect to UNIX socket)")
	apiMode := app.BoolOpt("a api", false, "Enable the REST API")
	apiport := app.StringOpt("apiport", "8080", "Port for the REST API")
	mdnsMode := app.BoolOpt("m mdns", false, "Enable mDNS server")
	mdnsPort := app.StringOpt("m mdns-port", "5353", "Port for mDNS server")

	app.Action = func() {
		//if we run in client mode we dont need to run the rest of the code
		if *clientMode && *remoteUnix != "" {
			connectToUnixSocket(*remoteUnix) // Connect to UNIX socket as client
			return
		}

		// Start the REST API if enabled
		if *apiMode {
			go startGinAPI(*apiport)
		}

		//Create JSON files if they don't exist
		initializeJSONFiles()

		//Load Data
		dnsRecords = loadDNSRecords()
		dnsServerSettings = loadSettings()
		cacheRecords = loadCacheRecords()
		dnsServers = loadDNSServers()

		// Set up the DNS server handler
		dns.HandleFunc(".", handleRequest)

		// Configure the DNS server settings
		server := &dns.Server{
			Addr: fmt.Sprintf(":%s", *port),
			Net:  "udp",
		}

		// Start the DNS server
		go func() {
			log.Printf("Starting DNS server on %s\n", server.Addr)
			dnsStats.ServerStartTime = time.Now()
			if err := server.ListenAndServe(); err != nil {
				fmt.Println("Error starting server:", err)
				os.Exit(1)
			}
		}()

		// mDNS server setup
		if *mdnsMode {
			go startMDNSServer(*mdnsPort)
		}

		// If running in daemon mode, exit after starting the server
		if *daemon {
			if *remoteUnix != "" {
				setupUnixSocketListener(*remoteUnix) // Set up UNIX socket listener for daemon mode
			} else {
				select {} // Keeps the program alive
			}
		} else {
			// Interactive Mode
			config := readline.Config{
				Prompt:          "> ",
				HistoryFile:     "/tmp/dnsresolver.history",
				InterruptPrompt: "^C",
				EOFPrompt:       "exit",
			}

			rl, err := readline.NewEx(&config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "readline: %v\n", err)
				return
			}
			defer rl.Close()

			handleCommandLoop(rl) // Call the function for command handling
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func startMDNSServer(port string) {
	portInt, _ := strconv.Atoi(port)

	// Set up the multicast address for mDNS
	addr := &net.UDPAddr{IP: net.ParseIP("224.0.0.251"), Port: portInt}

	// Create a UDP connection to listen on multicast address
	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		log.Fatalf("Error setting up mDNS server: %v", err)
	}

	// Set reuse address option to allow multiple listeners on the same address
	if err := conn.SetReadBuffer(65535); err != nil {
		log.Fatalf("Failed to set read buffer size: %v", err)
	}

	server := &dns.Server{
		PacketConn: conn,
	}

	dns.HandleFunc("local.", handleMDNSRequest)

	log.Printf("Starting mDNS server on %s\n", addr)
	if err := server.ActivateAndServe(); err != nil {
		log.Fatalf("Error starting mDNS server: %v", err)
	}
}

func initializeJSONFiles() {

	if _, err := os.Stat("records.json"); os.IsExist(err) {
		dnsRecords = loadDNSRecords()
	}

	fmt.Println("records loaded: ", len(dnsRecords))

	createFileIfNotExists("servers.json", `{"servers": ["1.1.1.1:53", "1.0.0.1:53"]}`)
	createFileIfNotExists("records.json", `{"records": [{"name": "example.com.", "type": "A", "value": "93.184.216.34", "ttl": 3600, "last_query": "0001-01-01T00:00:00Z"}]}`)
	createFileIfNotExists("cache.json", `{"records": [{"dnsrecord": {"name": "example.com.", "type": "A", "value": "93.184.216.34", "ttl": 3600, "last_query": "0001-01-01T00:00:00Z"}, "expiry": "0001-01-01T00:00:00Z", "timestamp": "0001-01-01T00:00:00Z", "last_query": "0001-01-01T00:00:00Z"}]}`)
	createFileIfNotExists("dnsresolver.json", `{"fallback_server_ip": "192.168.178.21", "fallback_server_port": "53", "timeout": 2, "dns_port": "53", "cache_records": true, "auto_build_ptr_from_a": true}`)
}

func createFileIfNotExists(filename, content string) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		err = os.WriteFile(filename, []byte(content), 0644)
		if err != nil {
			log.Fatalf("Error creating %s: %s", filename, err)
		}
	}
}
