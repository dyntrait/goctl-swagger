package generate

import (
	"gopkg.in/natefinch/lumberjack.v2"
	"log"
)

func init() {
	log.SetOutput(&lumberjack.Logger{
		Filename:   "./foo.log",
		MaxSize:    500, // megabytes
		MaxBackups: 3,
		MaxAge:     28,   //days
		Compress:   true, // disabled by default
	})
}
