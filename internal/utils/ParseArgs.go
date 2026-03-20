package utils

import (
	"errors"
	"fmt"
	"strings"

	"net-cat-server/internal/server"
)

// ParseArgs processes the slice of command line arguments passed to the net-cat program.
func ParseArgs(arguments []string) (server.Config, error) {
	errorConfig := server.Config{PrintHelp: true}
	var configs server.Config
	nonOptions := []string{}
	i := 0
	var arg string

	handleOptionArgs := func(opt string) (string, error) {
		_, arg, found := strings.Cut(arg, "=")

		if found {
			if len(arg) < 1 {
				return "", fmt.Errorf("missing argument to option %q", opt)
			}

			return arg, nil
		}

		i++
		if i >= len(arguments) || arguments[i] == "--" {
			return "", fmt.Errorf("missing argument to option %q", opt)
		}

		return arguments[i], nil
	}

	for ; i < len(arguments); i++ {
		arg = arguments[i]

		if !strings.HasPrefix(arg, "-") {
			nonOptions = append(nonOptions, arg)
			continue
		}

		arg = strings.TrimLeft(arg, "-")
		if strings.HasPrefix(arg, "logfile") {
			filename, err := handleOptionArgs(arg)
			if err != nil {
				return errorConfig, err
			}

			configs.LogFile = filename
		} else if arg == "help" || arg == "h" {
			return server.Config{PrintHelp: true}, nil
		} else if strings.HasPrefix(arg, "max-connections") {
			numStr, err := handleOptionArgs(arg)
			if err != nil {
				return errorConfig, err
			}

			maxConns, err := Atoi(numStr)

			if (err != nil && strings.Contains(err.Error(), "overflow")) || maxConns < 0 {
				maxConns = -1
			} else if err != nil {
				return errorConfig, fmt.Errorf("invalid argument to 'max-connections': %w", err)
			}

			configs.MaxConnections = maxConns
		} else if len(arg) > 0 {
			return errorConfig, fmt.Errorf("unrecognised option: %q", arguments[i])
		} else {
			break
		}
	}

	// This specific case requires this message.
	if len(nonOptions) > 1 {
		return server.Config{}, errors.New("[USAGE]: ./TCPChat $port")
	}

	if len(nonOptions) == 1 {
		configs.Port = nonOptions[0]
	}

	return configs, nil
}
