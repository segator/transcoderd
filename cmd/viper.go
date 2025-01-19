package cmd

import (
	"errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"net/url"
	"reflect"
	"time"
)

func ViperConfig(opts *CommandLineConfig) {

	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/transcoderd/")
	viper.AddConfigPath("$HOME/.transcoderd/")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetEnvPrefix("TR")
	err := viper.ReadInConfig()
	if err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		switch {
		case errors.As(err, &configFileNotFoundError):
			log.Warnf("No Config File Found")
		default:
			log.Panic(err)
		}
	}
	pflag.Parse()
	log.SetFormatter(&log.TextFormatter{
		ForceColors:               true,
		EnvironmentOverrideColors: true,
	})

	err = viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		log.Panic(err)
	}

	err = viper.Unmarshal(opts, decodeHook())
	if err != nil {
		log.Panic(err)
	}
	if opts.Verbose {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
}
func decodeHook() viper.DecoderConfigOption {
	return viper.DecodeHook(func(source reflect.Type, target reflect.Type, data interface{}) (interface{}, error) {
		if source.Kind() != reflect.String {
			return data, nil
		}

		if target == reflect.TypeOf(url.URL{}) {
			return url.Parse(data.(string))
		}

		if target == reflect.TypeOf(time.Duration(5)) {
			return time.ParseDuration(data.(string))
		}
		return data, nil
	})
}
