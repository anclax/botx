package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudcarver/botx/pkg/codegen"
	"github.com/urfave/cli/v2"
)

var version = "dev"

func main() {
	app := &cli.App{
		Name:    "botx",
		Usage:   "Botx generator",
		Version: version,
		Commands: []*cli.Command{
			{
				Name:  "gen",
				Usage: "Generate botx code",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "package",
						Usage: "Package name for generated code",
					},
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   "Output path",
					},
					&cli.StringFlag{
						Name:    "config",
						Aliases: []string{"c"},
						Usage:   "Config file path",
						Value:   "./botx.yaml",
					},
				},
				Action: func(c *cli.Context) error {
					configPath := c.String("config")
					outputPath := c.String("output")
					packageName := c.String("package")
					if outputPath == "" {
						return fmt.Errorf("output path is required")
					}
					parser := codegen.NewParser()
					doc, err := parser.ParseFile(configPath)
					if err != nil {
						return err
					}
					if packageName != "" {
						doc.Package = packageName
					}
					content, err := codegen.Generate(doc)
					if err != nil {
						return err
					}
					resolved := codegen.ResolveOutputPath(outputPath)
					if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
						return err
					}
					return os.WriteFile(resolved, content, 0o644)
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
