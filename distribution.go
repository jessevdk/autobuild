package main

import (
	"fmt"
)

type Distribution struct {
	Os            string   `json:"os"`
	CodeName      string   `json:"codename"`
	Architectures []string `json:"architectures"`
}

func (x *Distribution) SourceName() string {
	return fmt.Sprintf("%s/%s", x.Os, x.CodeName)
}

func (x *Distribution) BinaryName(arch string) string {
	return fmt.Sprintf("%s/%s/%s", x.Os, x.CodeName, arch)
}

func (x *Distribution) IsSource() bool {
	return len(x.Architectures) == 1 && x.Architectures[0] == "source"
}
