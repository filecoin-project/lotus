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
	if _, err := g.writer.Write([]byte(fmt.Sprintf("# %s\n\n```\n", g.app.Name))); err != nil {
		return err
	}

	if err := g.app.Run(getHelpArgs("", "")); err != nil {
		return fmt.Errorf("failed to write command docs: %w", err)
	}

	if _, err := g.writer.Write([]byte("```\n")); err != nil {
		return err
	}

	return nil
}

func (g *DocGenerator) writeCommandDocs(commands cli.Commands, rootName string, depth int) error {
	uncategorizedCmds, categorizedCmds := separateCommands(commands)

	// Write uncategorized commands first
	if err := g.writeCommands(uncategorizedCmds, rootName, depth); err != nil {
		return fmt.Errorf("failed to write uncategorized commands: %w", err)
	}

	// Write categorized commands next
	if err := g.writeCommands(categorizedCmds, rootName, depth); err != nil {
		return fmt.Errorf("failed to write categorized commands: %w", err)
	}

	return nil
}

// separateCommands separates commands into uncategorized and categorized
func separateCommands(commands []*cli.Command) (uncategorized cli.Commands, categorized cli.Commands) {
	for _, cmd := range commands {
		if cmd.Category == "" {
			uncategorized = append(uncategorized, cmd)
		} else {
			categorized = append(categorized, cmd)
		}
	}

	return uncategorized, categorized
}

// writeCommands writes documentation for all commands recursively
func (g *DocGenerator) writeCommands(commands cli.Commands, rootName string, depth int) error {
	for _, cmd := range commands {
		if cmd.Name == "help" || cmd.Hidden {
			continue
		}

		cmdName := fmt.Sprintf("%s %s", rootName, cmd.Name)

		if _, err := g.writer.Write([]byte(fmt.Sprintf("\n%s %s\n\n```\n", strings.Repeat("#", depth+2), cmdName))); err != nil {
			return err
		}

		if err := g.app.Run(getHelpArgs(rootName, cmd.Name)); err != nil {
			return fmt.Errorf("failed to write command docs: %w", err)
		}

		if _, err := g.writer.Write([]byte("```\n")); err != nil {
			return err
		}

		if len(cmd.Subcommands) > 0 {
			if err := g.writeCommands(cmd.Subcommands, rootName+" "+cmd.Name, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func getHelpArgs(rootName string, cmdName string) []string {
	if rootName == "" && cmdName == "" {
		return []string{"-h"}
	}

	args := strings.Split(rootName, " ")
	args = append(args, cmdName)
	args = append(args, "-h")
	return args
}
