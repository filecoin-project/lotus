package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
)

func main() {
	for _, arg := range os.Args {
		handleKnowns(arg)
	}
}

func handleKnowns(pathStart string) {
	outpath := path.Join(pathStart, "out.gotext.json")
	b, err := os.ReadFile(outpath)
	if err != nil {
		fmt.Println("cannot open "+outpath+":", err)
		return
	}
	type TMsg struct {
		ID          string          `json:"id"`
		Translation string          `json:"translation"`
		Message     string          `json:"message"`
		Placeholder json.RawMessage `json:"placeholder"`
	}
	type Dataformat struct {
		Language string `json:"language"`
		Messages []TMsg `json:"messages"`
	}
	var outData Dataformat
	err = json.NewDecoder(bytes.NewBuffer(b)).Decode(&outData)
	if err != nil {
		fmt.Println("cannot decode "+outpath+":", err)
		return
	}

	f, err := os.Open(path.Join(pathStart, "messages.gotext.json"))
	if err != nil {
		fmt.Println("cannot open "+path.Join(pathStart, "messages.gotext.json")+":", err)
		return
	}
	defer f.Close()

	var msgData Dataformat
	err = json.NewDecoder(f).Decode(&msgData)
	if err != nil {
		fmt.Println("cannot decode "+path.Join(pathStart, "messages.gotext.json")+":", err)
		return
	}

	knowns := map[string]string{}
	for _, msg := range msgData.Messages {
		knowns[msg.ID] = msg.Translation
	}

	var doneMessages []TMsg
	var toTranslate []TMsg
	for _, msg := range outData.Messages {
		if k, ok := knowns[msg.ID]; ok {
			msg.Translation = k
			doneMessages = append(doneMessages, msg)
		} else {
			toTranslate = append(toTranslate, msg)
		}
	}

	outData.Messages = toTranslate // drop the "done" messages
	var outJSON bytes.Buffer
	enc := json.NewEncoder(&outJSON)
	enc.SetIndent("  ", "  ")
	err = enc.Encode(outData)
	if err != nil {
		fmt.Println("cannot encode "+outpath+":", err)
		return
	}
	err = os.WriteFile(outpath, outJSON.Bytes(), 0644)
	if err != nil {
		fmt.Println("cannot write "+outpath+":", err)
		return
	}
	fmt.Println("rearranged successfully")
}
