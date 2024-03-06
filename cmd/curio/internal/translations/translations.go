// Usage:
// To UPDATE translations:
//  1. add/change strings in guidedsetup folder that use d.T() or d.say().
//  2. run `go generate` in the cmd/curio/internal/translations/ folder.
//  3. Ask ChatGPT to translate the ./locales/??/out.gotext.json files'
//     which ONLY include the un-translated messages.
//     APPEND to the messages.gotext.json files (which include other messages).
//     In web UI, you may need to hit "continue generating".
//     ChatGPT has a limit of about 60. After that, you can translate in sections.
//  4. run `go generate` in the cmd/curio/internal/translations/ folder to re-import.
//
// To ADD a language:
//  1. Add it to the list in updateLang.sh
//  2. Run `go generate` in the cmd/curio/internal/translations/ folder.
//  3. Follow the "Update translations" steps here.
//  4. Code will auto-detect the new language and use it.
//
// FUTURE Reliability: OpenAPI automation.
package translations

//go:generate ./updateLang.sh
