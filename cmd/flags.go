package cmd

import (
	"github.com/spf13/pflag"
)

func WebFlags() {
	pflag.Int("web.port", 8080, "WebServer Port")
	pflag.String("web.token", "admin", "WebServer Port")
	pflag.String("web.domain", "http://localhost:8080", "Base domain where workers will try to download upload videos")

}
