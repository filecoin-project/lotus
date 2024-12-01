package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

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
	header := tabwriter.NewWriter(g.writer, 1, 8, 2, ' ', 0)
	defer header.Flush() //nolint: errcheck

	if _, err := header.Write([]byte(fmt.Sprintf("# %s\n\n```\n", g.app.Name))); err != nil {
		return err
	}

	if err := g.writeBasicInfo(header); err != nil {
		return err
	}

	if err := g.writeCommandsSection(header); err != nil {
		return err
	}

	if err := g.writeGlobalOptions(header); err != nil {
		return err
	}

	if _, err := header.Write([]byte("\n```\n")); err != nil {
		return err
	}

	return nil
}

func (g *DocGenerator) writeBasicInfo(w *tabwriter.Writer) error {
	metadata := []struct {
		section string
		content string
	}{
		{"NAME", fmt.Sprintf("%s - %s", g.app.Name, g.app.Usage)},
		{"USAGE", generateUsage(g.app.Name)},
		{"VERSION", g.app.Version},
	}

	for _, m := range metadata {
		if _, err := w.Write([]byte(fmt.Sprintf("%s:\n   %s\n\n", m.section, m.content))); err != nil {
			return err
		}
	}

	return nil
}

func (g *DocGenerator) writeCommandsSection(w *tabwriter.Writer) error {
	if len(g.app.Commands) == 0 {
		return nil
	}

	if _, err := w.Write([]byte("COMMANDS:\n")); err != nil {
		return err
	}

	return g.writeCategoryContent(w)
}

func (g *DocGenerator) writeGlobalOptions(w *tabwriter.Writer) error {
	if len(g.app.Flags) == 0 {
		return nil
	}

	if _, err := w.Write([]byte("\nGLOBAL OPTIONS:\n")); err != nil {
		return err
	}

	for _, flag := range g.app.Flags {
		if _, err := w.Write([]byte(NewFlagFormatter(flag).Format())); err != nil {
			return err
		}
	}

	return nil
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
