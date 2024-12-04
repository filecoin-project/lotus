package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
)

// DocGenerator handles CLI documentation generation
type DocGenerator struct {
	app       *cli.App
	outputDir string
	writer    io.Writer
}

// NewDocGenerator creates a new documentation generator
func NewDocGenerator(outputDir string, app *cli.App) *DocGenerator {
	return &DocGenerator{
		outputDir: outputDir,
		app:       app,
	}
}

// Generate generates documentation for the CLI app
func (g *DocGenerator) Generate(name string) error {
	file, err := g.createMarkdownFile(name)
	if err != nil {
		return fmt.Errorf("failed to create markdown file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("failed to close markdown file: %v\n", err)
		}
	}()

	return g.generateContent(file, name)
}

// createMarkdownFile creates a new markdown file for output.
func (g *DocGenerator) createMarkdownFile(name string) (*os.File, error) {
	filePath := filepath.Join(g.outputDir, fmt.Sprintf("cli-%s.md", name))
	return os.Create(filePath)
}

func (g *DocGenerator) generateContent(file *os.File, name string) error {
	bufferedWriter := bufio.NewWriter(file)
	g.writer = bufferedWriter
	g.app.Writer = bufferedWriter

	if err := g.generateDocs(name); err != nil {
		return fmt.Errorf("failed to generate documentation: %w", err)
	}

	return bufferedWriter.Flush()
}

// generateDocs orchestrates the documentation generation process
func (g *DocGenerator) generateDocs(name string) error {
	if err := g.writeAppHeader(); err != nil {
		return fmt.Errorf("failed to write app header: %w", err)
	}

	return g.writeCommandDocs(g.app.Commands, name, 0)
}

// writeAppHeader writes the application header documentation
func (g *DocGenerator) writeAppHeader() error {
	header := &strings.Builder{}
	header.WriteString(fmt.Sprintf("# %s\n\n```\n", g.app.Name))

	customAppData := func() map[string]interface{} {
		return map[string]interface{}{
			"ExtraInfo": g.app.ExtraInfo,
		}
	}

	cli.HelpPrinterCustom(header, cli.AppHelpTemplate, g.app, customAppData())

	header.WriteString("```\n")
	_, err := g.writer.Write([]byte(header.String()))

	return err
}

// writeCommandDocs writes documentation for all commands recursively
func (g *DocGenerator) writeCommandDocs(commands []*cli.Command, rootName string, depth int) error {
	for _, cmd := range commands {
		if cmd.Name == "help" {
			continue
		}

		cmdName := fmt.Sprintf("%s %s", rootName, cmd.Name)

		if _, err := g.writer.Write([]byte(fmt.Sprintf("\n%s %s\n\n```\n", strings.Repeat("#", depth+2), cmdName))); err != nil {
			return fmt.Errorf("failed to write command docs: %w", err)
		}

		if err := g.app.Run(getArgs(rootName, cmd.Name)); err != nil {
			return fmt.Errorf("failed to write command docs: %w", err)
		}

		if _, err := g.writer.Write([]byte("```\n")); err != nil {
			return fmt.Errorf("failed to write command docs: %w", err)
		}

		if len(cmd.Subcommands) > 0 {
			if err := g.writeCommandDocs(cmd.Subcommands, rootName+" "+cmd.Name, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func getArgs(rootName string, cmdName string) []string {
	args := strings.Split(rootName, " ")
	args = append(args, cmdName)
	args = append(args, "-h")
	return args
}
