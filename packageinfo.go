package main

import (
	"path"
)

type PackageInfo struct {
	StageFile   string
	Name        string
	Version     string
	Compression string
	Uid         uint32
}

func NewPackageInfo(filename string, uid uint32) *PackageInfo {
	basename := path.Base(filename)
	matched := packageInfoRegex.FindStringSubmatch(basename)

	if matched == nil {
		return nil
	}

	return &PackageInfo{
		StageFile:   filename,
		Name:        matched[1],
		Version:     matched[2],
		Compression: matched[4],
		Uid:         uid,
	}
}

func (x *PackageInfo) MatchStageFile(filename string) bool {
	if x == nil {
		return false
	}

	return path.Base(x.StageFile) == filename
}
