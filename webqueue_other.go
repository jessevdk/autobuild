// +build !darwin

package main

import (
	"os/exec"
)

func (x *WebQueueCommand) openBrowser(url string) {
	go exec.Command("x-www-browser", url).Run()
}
