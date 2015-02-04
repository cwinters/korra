package main

import (
	"flag"
	"fmt"

	korra "github.com/cwinters/korra/lib"
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
	script, err := korra.CheckScript(opts.scanf)
	if err != nil {
		return err
	}
	for _, action := range script.Actions {
		if opts.verbose {
			var message string
			if action.Error != nil {
				message = fmt.Sprintf("INVALID %s", action.Error)
			} else {
				target := action.Target
				if target.Comment != "" {
					message = fmt.Sprintf("INFO => %s", target.Comment)
				} else if target.PauseTime > 0 {
					message = fmt.Sprintf("PAUSE for %d ms", target.PauseTime)
				} else {
					pollingMessage := "NO"
					if target.Poller.Active {
						pollingMessage = fmt.Sprintf("YES, %s", target.Poller.ToString())
					}
					message = fmt.Sprintf("%s %s [Headers: %d] [Body? %t] [Polling? %s]",
						target.Method, target.URL, len(target.Header), target.BodyPath != "", pollingMessage)
				}
			}
			fmt.Printf("Line %d: %s\n", action.Line, message)
		} else if action.Error != nil {
			fmt.Printf("Line %d: %s\n-----\n%s\n-----\n", action.Line, action.Error, action.Raw)
		}
	}
	return nil
}
