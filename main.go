package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/dyntrait/goctl-swagger/action"
	"github.com/urfave/cli/v2"
)

var (
	version  = time.Now().Format(time.RFC3339)
	commands = []*cli.Command{
		{
			Name:   "swagger",
			Usage:  "generates swagger.json",
			Action: action.Generator,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "host",
					Usage: "api request address",
				},
				&cli.StringFlag{
					Name:  "basepath",
					Usage: "url request prefix",
				},
				&cli.StringFlag{
					Name:  "filename",
					Usage: "swagger save file name",
				},
			},
		},
	}
)

func main() {
	cli.VersionFlag = &cli.BoolFlag{
		Name:    "print-version",
		Aliases: []string{"V"},
		Usage:   "print only the version",
	}
	app := cli.NewApp()
	app.Usage = "a plugin of goctl to generate swagger.json"
	app.Version = fmt.Sprintf("%s %s/%s", version, runtime.GOOS, runtime.GOARCH)
	app.Commands = commands
	if err := app.Run(os.Args); err != nil {
		fmt.Printf("goctl-swagger1: %+v\n", err)
	}
}
