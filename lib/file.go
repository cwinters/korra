package korra

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func File(name string, create bool) (*os.File, error) {
	switch name {
	case "stdin":
		return os.Stdin, nil
	case "stdout":
		return os.Stdout, nil
	default:
		if create {
			return os.Create(name)
		}
		return os.Open(name)
	}
}

func GlobInputs(spec string) []string {
	info, err := os.Stat(spec)
	if err == nil && info.IsDir() {
		spec = fmt.Sprintf("%s/*.txt", spec)
	}
	var files []string
	if strings.Contains(spec, "*") {
		if files, err = filepath.Glob(spec); err != nil {
			panic(fmt.Sprintf("Bad glob %s: %s", spec, err))
		}
	} else {
		files = strings.Split(spec, ",")
	}
	return files
}

func GlobResults(spec string) []string {
	info, err := os.Stat(spec)
	if err == nil && info.IsDir() {
		spec = fmt.Sprintf("%s/*.bin", spec)
	}
	var files []string
	if strings.Contains(spec, "*") {
		if files, err = filepath.Glob(spec); err != nil {
			panic(fmt.Sprintf("Bad glob %s: %s", spec, err))
		}
	} else {
		files = strings.Split(spec, ",")
	}
	return files
}
