package utils

import (
	"reflect"
	"testing"

	"net-cat/internal/server"
)

func TestParseArgsExtended(t *testing.T) {
	testCases := []struct {
		name        string
		args        []string
		expectErr   bool
		expectedCfg server.Config
	}{
		{name: "unknown option", args: []string{"--unknown-option-404"}, expectErr: true},
		{name: "no args", args: []string{"--"}},
		{name: "no args 2", args: []string{}},
		{name: "only port", args: []string{"1234"}, expectedCfg: server.Config{Port: "1234"}},
		{name: "--logfile=", args: []string{"--logfile=test.log"}, expectedCfg: server.Config{LogFile: "test.log"}},
		{name: "--logfile", args: []string{"--logfile", "test.log"}, expectedCfg: server.Config{LogFile: "test.log"}},
		{name: "--logfile= err1", args: []string{"--logfile="}, expectErr: true},
		{name: "--logfile err2", args: []string{"--logfile"}, expectErr: true},
		{name: "--logfile err3", args: []string{"--logfile", "--"}, expectErr: true},
		{name: "--help", args: []string{"--help"}, expectedCfg: server.Config{PrintHelp: true}},
		{name: "-h", args: []string{"-h"}, expectedCfg: server.Config{PrintHelp: true}},
		{name: "--max-connections=", args: []string{"--max-connections=-1"}, expectedCfg: server.Config{MaxConnections: -1}},
		{name: "--max-connections maxUint64", args: []string{"--max-connections", "18446744073709551615"}, expectedCfg: server.Config{MaxConnections: -1}},
		{name: "--max-connections=minInt64", args: []string{"--max-connections=-9223372036854775807"}, expectedCfg: server.Config{MaxConnections: -1}},
		{name: "--max-connections minInt64", args: []string{"--max-connections", "-9223372036854775807"}, expectedCfg: server.Config{MaxConnections: -1}},
		{name: "--max-connections=maxUint64", args: []string{"--max-connections=18446744073709551615"}, expectedCfg: server.Config{MaxConnections: -1}},
		{name: "--max-connections= err1", args: []string{"--max-connections="}, expectErr: true},
		{name: "--max-connections err2", args: []string{"--max-connections"}, expectErr: true},
		{name: "--max-connections err3", args: []string{"--max-connections", "--"}, expectErr: true},
		{name: "multiple options + port", args: []string{"--max-connections", "128", "--logfile=test.log", "7200"}, expectedCfg: server.Config{Port: "7200", LogFile: "test.log", MaxConnections: 128}},
		{name: "port + multiple options", args: []string{"7200", "--max-connections=0", "--logfile", "test.log"}, expectedCfg: server.Config{Port: "7200", LogFile: "test.log", MaxConnections: 0}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := ParseArgs(tc.args)

			if tc.expectErr && err != nil {
				return
			}

			if tc.expectErr && err == nil {
				t.Fatal("expected an error got nil")
			}

			if err != nil {
				t.Error("unexpected error")
			}

			if !reflect.DeepEqual(tc.expectedCfg, cfg) {
				t.Errorf("Got\t: %#v\nExpected: %#v", cfg, tc.expectedCfg)
			}
		})
	}
}
