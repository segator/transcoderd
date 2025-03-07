package cmd

import (
	"fmt"
	"github.com/spf13/pflag"
	"os"
	"transcoder/update"
)

func CommonFlags() {
	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTION]...\n", os.Args[0])
		pflag.PrintDefaults()
		os.Exit(0)
	}
	pflag.Bool("version", false, "Print version and exit")
	pflag.Bool("verbose", false, "Enable verbose logging")
	pflag.Int("web.port", 8080, "WebServer Port")
	pflag.String("web.token", "admin", "WebServer Port")
	pflag.String("web.domain", "http://localhost:8080", "Base domain where workers will try to download upload videos")
	update.PFlags()
}
