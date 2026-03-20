package main

import (
	"fmt"
	"log"
	"os"

	"net-cat-server/internal/server"
	"net-cat-server/internal/utils"
)

func printHelp() {
	helpMessage := `Usage: %s [OPTIONS] [PORT]

Run TCP-Chat server that facilitates sending of messages between clients and "chat rooms".
PORT is the TCP port to listen on, if PORT is omitted, defaults to %s.

Options:
  -h, --help                      Show this help message and exit
      --logfile FILE              Write chat logs to FILE
      --max-connections N         Set max concurrent clients (default: %d, negative numbers means unlimited)

Examples:
  %s
  %s 2525
  %s --logfile chat.log --max-connections 32 2525
`

	fmt.Fprintf(os.Stderr, helpMessage, os.Args[0], server.PortDefault, server.MaxConnectionsDefault, os.Args[0], os.Args[0], os.Args[0])
}

func main() {
	// program flags
	args := os.Args[1:]

	configs, err := utils.ParseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if configs.PrintHelp {
		printHelp()
		return
	}

	s, err := server.NewServer(configs)
	if err != nil {
		log.Fatal("Error starting server:", err)
	}
	defer s.Stop()

	s.Run() // Blocks until Server.Stop() is called externally.
}
