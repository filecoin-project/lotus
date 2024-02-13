// Usage:
//  1. change strings in guidedSetup folder that use d.T() or d.say().
//  2. run `go generate` in the cmd/lotus-provider/internal/translations/ folder.
//  3. Ask ChatGPT to translate the locale/??/out.gotext.json files' translation
//     fields to their respective languages. Replace the messages.gotext.json files.
//     In web UI, you need to hit "continue generating"
//  4. run `go generate` in the cmd/lotus-provider/internal/translations/ folder to re-import.
//
// FUTURE Reliability: automate this with an openAPI call when translate fields turn up blank.
// FUTURE Cost Savings: avoid re-translating stuff that's in messages.gotext.json already.
package translations

//go:generate gotext -srclang=en update -out=catalog.go -lang=en,zh,ko github.com/filecoin-project/lotus/cmd/lotus-provider/guidedSetup
