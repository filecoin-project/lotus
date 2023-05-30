package lotuslog

import logging "github.com/ipfs/go-log/v2"

func SetLevelsFromConfig(l map[string]string) {
	for sys, level := range l {
		if err := logging.SetLogLevel(sys, level); err != nil {
			continue
		}
	}
}
