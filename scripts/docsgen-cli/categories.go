package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/urfave/cli/v2"
)

// Category represents a group of related commands
type Category struct {
	Name     string
	Commands []*cli.Command
}

func (g *DocGenerator) writeCategoryContent(w *tabwriter.Writer) error {
	maxWidth := calculateMaxCommandWidth(g.app.Commands, 1)

	if err := g.writeUncategorizedCommands(w, maxWidth); err != nil {
		return err
	}

	return g.writeCategorizedCommands(w)
}

func (g *DocGenerator) writeUncategorizedCommands(w *tabwriter.Writer, maxWidth int) error {
	for _, cmd := range g.app.Commands {
		if cmd.Category == "" {
			if _, err := w.Write([]byte(formatCommandSummary(cmd, maxWidth))); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *DocGenerator) writeCategorizedCommands(w *tabwriter.Writer) error {
	categories := g.groupCommandsByCategory()

	for _, category := range categories {
		if err := g.writeCategorySection(w, category); err != nil {
			return err
		}
	}
	return nil
}

func (g *DocGenerator) groupCommandsByCategory() []Category {
	categoryMap := make(map[string][]*cli.Command, len(g.app.Commands))
	var order []string

	// Group commands by category
	for _, cmd := range g.app.Commands {
		if cmd.Category != "" {
			if _, exists := categoryMap[cmd.Category]; !exists {
				order = append(order, cmd.Category)
			}
			categoryMap[cmd.Category] = append(categoryMap[cmd.Category], cmd)
		}
	}

	// Create ordered categories
	categories := make([]Category, 0, len(order))
	for _, name := range order {
		categories = append(categories, Category{
			Name:     name,
			Commands: categoryMap[name],
		})
	}

	return categories
}

func (g *DocGenerator) writeCategorySection(w *tabwriter.Writer, category Category) error {
	if _, err := w.Write([]byte(fmt.Sprintf("   %s:\n", category.Name))); err != nil {
		return err
	}

	maxWidth := calculateMaxCommandWidth(category.Commands, 1)
	for _, cmd := range category.Commands {
		if _, err := w.Write([]byte("  " + formatCommandSummary(cmd, maxWidth))); err != nil {
			return err
		}
	}

	return nil
}
