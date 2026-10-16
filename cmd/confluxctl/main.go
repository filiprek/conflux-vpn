package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/filiprek/conflux-vpn/internal/control"
	"github.com/filiprek/conflux-vpn/internal/version"
	"io"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	pipe := flag.String("pipe", defaultPipe(), "Conflux-SID named pipe")
	v := flag.Bool("version", false, "print version")
	flag.Parse()
	if *v {
		fmt.Println(version.String())
		return nil
	}
	if flag.NArg() != 1 {
		return fmt.Errorf("usage: confluxctl [--pipe Conflux-SID] status|connect|disconnect|recover|import-relay|import-path|diagnostics|configure|test-path|test-relay")
	}
	methods := map[string]string{"status": "status", "connect": "connect", "disconnect": "disconnect", "recover": "recover", "import-relay": "importRelay", "import-path": "importPath", "diagnostics": "diagnostics", "configure": "configure", "test-path": "testPath", "test-relay": "testRelay"}
	method, ok := methods[flag.Arg(0)]
	if !ok {
		return fmt.Errorf("unknown command")
	}
	params := json.RawMessage(`{}`)
	if method == "importRelay" || method == "importPath" || method == "configure" || method == "testPath" {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, control.MaxMessage+1))
		if err != nil || len(b) > control.MaxMessage {
			return fmt.Errorf("invalid or oversized JSON input")
		}
		if !json.Valid(b) {
			return fmt.Errorf("provide JSON parameters on stdin, never credentials on command line")
		}
		params = b
	}
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	r, err := control.CallPipe(ctx, *pipe, control.Request{Version: 1, ID: "cli", Method: method, Params: params})
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(r.Result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
