package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func file(name string, create bool) (*os.File, error) {
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

func globInputs(spec string) (files []string) {
	info, err := os.Stat(spec)
	if err == nil && info.IsDir() {
		spec = fmt.Sprintf("%s/*.txt", spec)
	}
	if strings.Contains(spec, "*") {
		if files, err := filepath.Glob(spec); err != nil {
			panic(fmt.Sprintf("Bad glob %s: %s", spec, err))
		}
	} else {
		files = strings.Split(spec, ",")
	}
	return files
}

func globResults(spec string) (files []string) {
	info, err := os.Stat(spec)
	if err == nil && info.IsDir() {
		spec = fmt.Sprintf("%s/*.bin", spec)
	}
	if strings.Contains(spec, "*") {
		if files, err := filepath.Glob(spec); err != nil {
			panic(fmt.Sprintf("Bad glob %s: %s", spec, err))
		}
	} else {
		files = strings.Split(spec, ",")
	}
	return files
}
