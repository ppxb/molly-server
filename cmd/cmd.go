package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "Molly",
		Usage: "Out-of-the-box enterprise cloud drive system with native support for S3 and self-hosted MinIO.",
		Commands: []*cli.Command{
			{
				Name:  "server",
				Usage: "Start a web server",
				Action: func(context *cli.Context) error {
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
