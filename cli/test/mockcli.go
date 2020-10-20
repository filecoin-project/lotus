package test

import (
	"bytes"
	"flag"
	"strings"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	lcli "github.com/urfave/cli/v2"
)

type mockCLI struct {
	t    *testing.T
	cmds []*lcli.Command
	cctx *lcli.Context
	out  *bytes.Buffer
}

func newMockCLI(t *testing.T, cmds []*lcli.Command) *mockCLI {
	// Create a CLI App with an --api-url flag so that we can specify which node
	// the command should be executed against
	app := &lcli.App{
		Flags: []lcli.Flag{
			&lcli.StringFlag{
				Name:   "api-url",
				Hidden: true,
			},
		},
		Commands: cmds,
	}

	var out bytes.Buffer
	app.Writer = &out
	app.Setup()

	cctx := lcli.NewContext(app, &flag.FlagSet{}, nil)
	return &mockCLI{t: t, cmds: cmds, cctx: cctx, out: &out}
}

func (c *mockCLI) client(addr multiaddr.Multiaddr) *mockCLIClient {
	return &mockCLIClient{t: c.t, cmds: c.cmds, addr: addr, cctx: c.cctx, out: c.out}
}

// mockCLIClient runs commands against a particular node
type mockCLIClient struct {
	t    *testing.T
	cmds []*lcli.Command
	addr multiaddr.Multiaddr
	cctx *lcli.Context
	out  *bytes.Buffer
}

func (c *mockCLIClient) run(cmd []string, params []string, args []string) string {
	// Add parameter --api-url=<node api listener address>
	apiFlag := "--api-url=" + c.addr.String()
	params = append([]string{apiFlag}, params...)

	err := c.cctx.App.Run(append(append(cmd, params...), args...))
	require.NoError(c.t, err)

	// Get the output
	str := strings.TrimSpace(c.out.String())
	c.out.Reset()
	return str
}

func (c *mockCLIClient) runCmd(input []string) string {
	cmd := c.cmdByNameSub(input[0], input[1])
	out, err := c.runCmdRaw(cmd, input[2:])
	require.NoError(c.t, err)

	return out
}

func (c *mockCLIClient) cmdByNameSub(name string, sub string) *lcli.Command {
	for _, c := range c.cmds {
		if c.Name == name {
			for _, s := range c.Subcommands {
				if s.Name == sub {
					return s
				}
			}
		}
	}
	return nil
}

func (c *mockCLIClient) runCmdRaw(cmd *lcli.Command, input []string) (string, error) {
	// prepend --api-url=<node api listener address>
	apiFlag := "--api-url=" + c.addr.String()
	input = append([]string{apiFlag}, input...)

	fs := c.flagSet(cmd)
	err := fs.Parse(input)
	require.NoError(c.t, err)

	err = cmd.Action(lcli.NewContext(c.cctx.App, fs, c.cctx))

	// Get the output
	str := strings.TrimSpace(c.out.String())
	c.out.Reset()
	return str, err
}

func (c *mockCLIClient) flagSet(cmd *lcli.Command) *flag.FlagSet {
	// Apply app level flags (so we can process --api-url flag)
	fs := &flag.FlagSet{}
	for _, f := range c.cctx.App.Flags {
		err := f.Apply(fs)
		if err != nil {
			c.t.Fatal(err)
		}
	}
	// Apply command level flags
	for _, f := range cmd.Flags {
		err := f.Apply(fs)
		if err != nil {
			c.t.Fatal(err)
		}
	}
	return fs
}

func (c *mockCLIClient) runInteractiveCmd(cmd []string, interactive []string) string {
	c.toStdin(strings.Join(interactive, "\n") + "\n")
	return c.runCmd(cmd)
}

func (c *mockCLIClient) toStdin(s string) {
	c.cctx.App.Metadata["stdin"] = bytes.NewBufferString(s)
}
