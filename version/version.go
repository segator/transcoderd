package version

import log "github.com/sirupsen/logrus"

var (
	Version = "v0.0.0-dev"
	Commit  = "0000000"
	Date    = "0000-00-00T00:00:00Z"
)

func AppLogger() log.FieldLogger {
	return log.WithFields(log.Fields{
		"Version": Version,
		"Commit":  Commit,
		"Date":    Date,
	})
}

func LogVersion() {
	AppLogger().Info("Version Info")
}
