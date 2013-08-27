// +build darwin

package main

import (
	"os/exec"
)

func (x *WebQueueCommand) openBrowser(url string) {
	go exec.Command("open", url).Run()
}
