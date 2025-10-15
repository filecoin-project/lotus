package util

import (
    "encoding/json"
    "fmt"
    "github.com/urfave/cli/v2"
)

func FormatOutput(cctx *cli.Context, data interface{}) error {
    format := cctx.String("output")
    switch format {
    case "json":
        out, err := json.MarshalIndent(data, "", "  ")
        if err != nil {
            return err
        }
        fmt.Println(string(out))
    case "text":
        fmt.Println(data)
    default:
        return fmt.Errorf("unsupported output format: %s", format)
    }
    return nil
}