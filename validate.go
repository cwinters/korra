package main

import (
	"flag"
	"fmt"
	"strings"

	korra "github.com/cwinters/korra/lib"
)

type validateOpts struct {
	validateg string
	verbose   bool
}

func validateCmd() command {
	fs := flag.NewFlagSet("korra validate ", flag.ExitOnError)
	opts := &validateOpts{}
	fs.StringVar(&opts.validateg, "file", ".", "File or glob of files to validate")
	fs.BoolVar(&opts.verbose, "verbose", false, "Display all targets, not just errored ones")

	return command{fs, func(args []string) error {
		fs.Parse(args)
		validate(opts)
		return nil
	}}
}

func validate(opts *validateOpts) {
	for _, scriptFile := range korra.GlobInputs(opts.validateg) {
		messages, failures := validateScript(scriptFile, opts.verbose)
		status := "OK"
		if failures > 0 {
			status = fmt.Sprintf("FAIL %d", failures)
		}
		fmt.Printf("===== FILE %s %s\n", scriptFile, status)
		fmt.Print(strings.Join(messages, "\n"))
		if len(messages) > 0 {
			fmt.Println("")
		}
	}
}

func validateScript(scriptFile string, verbose bool) ([]string, int) {
	script, err := korra.CheckScript(scriptFile)
	if err != nil {
		return []string{err.Error()}, 1
	}
	var messages []string
	errors := 0
	for _, action := range script.Actions {
		if action.Error != nil {
			errors += 1
		}
		if verbose {
			message := fmt.Sprintf("%d: ", action.Line)
			if action.Error != nil {
				message += fmt.Sprintf("INVALID %s", action.Error)
			} else {
				target := action.Target
				if target.Comment != "" {
					message += fmt.Sprintf("INFO => %s", target.Comment)
				} else if target.PauseTime > 0 {
					message += fmt.Sprintf("PAUSE for %d ms", target.PauseTime)
				} else {
					pollingMessage := "NO"
					if target.Poller.Active {
						pollingMessage = fmt.Sprintf("YES, %s", target.Poller)
					}
					message += fmt.Sprintf("%s %s [Headers: %d] [Body? %t] [Polling? %s]",
						target.Method, target.URL, len(target.Header), target.BodyPath != "", pollingMessage)
				}
			}
			messages = append(messages, message)
		} else if action.Error != nil {
			messages = append(messages, action.Error.Error())
		}
	}
	return messages, errors
}
