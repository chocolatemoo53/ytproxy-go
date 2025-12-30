package log

import (
	"os"
	"time"

	"git.nadeko.net/Fijxu/http3-ytproxy/internal/config"
	"github.com/rs/zerolog"
)

var Logger zerolog.Logger

func StartLogger() {
	logLevel, err := zerolog.ParseLevel(config.Cfg.LogLevel)
	if err != nil {
		logLevel = zerolog.InfoLevel
	}

	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	Logger = zerolog.New(output).With().Timestamp().Logger().Level(logLevel)
}
