package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	korra "github.com/cwinters/korra/lib"
)

func dumpCmd() command {
	fs := flag.NewFlagSet("korra dump", flag.ExitOnError)
	dumper := fs.String("dumper", "", "Dumper [json, csv]")
	inputs := fs.String("inputs", "", "Input files as glob")
	output := fs.String("output", "stdout", "Output file")
	include := fs.String("include", "", "(TBD) Filter expression(s) to include only certain transactions")
	exclude := fs.String("exclude", "", "(TBD) Filter expression(s) to exclude transactions")

	return command{fs, func(args []string) error {
		fs.Parse(args)
		return dump(*dumper, *inputs, *output)
	}}
}

func dump(dumper, inputs, output string) error {
	dump, ok := dumpers[dumper]
	if !ok {
		return fmt.Errorf("unsupported dumper: %s", dumper)
	}

	files := globResults(inputs)
	srcs := make([]io.Reader, len(files))
	for i, f := range files {
		in, err := file(f, false)
		if err != nil {
			return err
		}
		defer in.Close()
		srcs[i] = in
	}

	out, err := file(output, true)
	if err != nil {
		return err
	}
	defer out.Close()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	res, errs := korra.Collect(srcs...)

	for {
		select {
		case _ = <-sig:
			return nil
		case r, ok := <-res:
			if !ok {
				return nil
			}
			dmp, err := dump.Dump(r)
			if err != nil {
				return err
			} else if _, err = out.Write(dmp); err != nil {
				return err
			}
		case err, ok := <-errs:
			if !ok {
				return nil
			}
			return err
		}
	}
}

var dumpers = map[string]korra.Dumper{
	"csv":  korra.DumpCSV,
	"json": korra.DumpJSON,
}
