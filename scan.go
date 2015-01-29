package main

import (
	"flag"
	"fmt"

	"github.com/cwinters/korra/lib"
)

type scanOpts struct {
	scanf   string
	verbose bool
}

func scanCmd() command {
	fs := flag.NewFlagSet("korra scan", flag.ExitOnError)
	opts := &scanOpts{}
	fs.StringVar(&opts.scanf, "file", "", "File to scan and check targets")
	fs.BoolVar(&opts.verbose, "verbose", false, "Display all targets, not just errored ones")

	return command{fs, func(args []string) error {
		fs.Parse(args)
		return scan(opts)
	}}
}

func scan(opts *scanOpts) error {
	f, err := file(opts.scanf, false)
	if err != nil {
		return fmt.Errorf("Error opening file to scan: %s", err)
	}
	script, err := korra.CheckScript(f)
	if err != nil {
		return fmt.Errorf("Error reading file: %s", err)
	}
	for idx, action := range *script.Actions {
		if opts.verbose {
			target := action.Target
			var message string
			if target.Comment != "" {
				message = fmt.Sprintf("  COMMENT: %s", target.Comment)
			} else if target.PauseTime > 0 {
				message = fmt.Sprintf("  PAUSE %d ms", target.PauseTime)
			} else {
				pollingMessage := "NO"
				if target.Poller.Active {
					pollingMessage = fmt.Sprintf("YES, %s", target.Poller.ToString())
				}
				message = fmt.Sprintf("  Method %s\n  URL %s\n  Polling? %s", target.Method, target.URL, pollingMessage)
			}
			validMessage := "YES"
			if action.Error != nil {
				validMessage = fmt.Sprintf("NO; %s", action.Error)
			}
			fmt.Printf("%d (line %d) - Valid? %s\n%s\n", idx, action.Line, validMessage, message)
		} else if action.Error != nil {
			fmt.Printf("Line %d: %s\n-----\n%s\n-----\n", action.Line, action.Error, action.Raw)
		}
	}
	return nil
}
