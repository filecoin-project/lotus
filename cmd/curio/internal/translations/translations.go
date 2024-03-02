// Usage:
//  1. change strings in guidedsetup folder that use d.T() or d.say().
//  2. run `go generate` in the cmd/curio/internal/translations/ folder.
//  3. Ask ChatGPT to translate the ./locales/??/out.gotext.json files'
//     which ONLY include the un-translated messages.
//     APPEND to the messages.gotext.json files (which include other messages).
//     In web UI, you may need to hit "continue generating".
//     ChatGPT has a limit, so if you change too many strings you may need to do it in parts.
//  4. run `go generate` in the cmd/curio/internal/translations/ folder to re-import.
//
// FUTURE Reliability: automate this with an openAPI call when translate fields turn up blank.
package translations

//go:generate make all
