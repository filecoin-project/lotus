// Usage:
// To UPDATE translations:
//
//  1. add/change strings in guidedsetup folder that use d.T() or d.say().
//
//  2. run `go generate` in the cmd/curio/internal/translations/ folder.
//
//  3. ChatGPT 3.5 can translate the ./locales/??/out.gotext.json files'
//     which ONLY include the un-translated messages.
//     APPEND to the messages.gotext.json files's array.
//
//     ChatGPT fuss:
//     - on a good day, you may need to hit "continue generating".
//     - > 60? you'll need to give it sections of the file.
//
//  4. Re-import with `go generate` again.
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
