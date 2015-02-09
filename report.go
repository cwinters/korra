package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	korra "github.com/cwinters/korra/lib"
)

func reportCmd() command {
	fs := flag.NewFlagSet("korra report", flag.ExitOnError)
	reporter := fs.String("reporter", "text", "Reporter [text, json, plot, dump, hist[buckets]]")
	inputs := fs.String("inputs", "", "Input files (comma separated, glob, or dir with .bin files)")
	output := fs.String("output", "stdout", "Output file")
	filters := fs.String("filters", "", "One or more space-separated filters to operate on subsets of the inputs")
	return command{fs, func(args []string) error {
		fs.Parse(args)
		return report(*reporter, *inputs, *output, *filters)
	}}
}

// report validates the report arguments, sets up the required resources
// and writes the report
func report(reporter, inputs, output, filters string) error {
	var rep korra.Reporter
	switch reporter {
	case "text":
		rep = korra.ReportText
	case "json":
		rep = korra.ReportJSON
	case "plot":
		rep = korra.ReportPlot
	case "hist":
		if len(reporter) < 6 {
			return fmt.Errorf("bad buckets: '%s'", reporter[4:])
		}
		var hist korra.HistogramReporter
		if err := hist.Set(reporter[4:len(reporter)]); err != nil {
			return err
		}
		rep = hist
	}

	if rep == nil {
		return fmt.Errorf("unknown reporter: %s", reporter)
	}
	var (
		err error
		in  *os.File
		out *os.File
	)
	files := korra.GlobResults(inputs)
	srcs := make([]io.Reader, len(files))
	for i, f := range files {
		if in, err = korra.File(f, false); err != nil {
			return err
		}
		defer in.Close()
		srcs[i] = in
	}
	if out, err = korra.File(output, true); err != nil {
		return err
	}
	defer out.Close()

	var results korra.Results
	res, errs := korra.Collect(srcs...)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

outer:
	for {
		select {
		case _ = <-sig:
			break outer
		case r, ok := <-res:
			if !ok {
				break outer
			}
			results = append(results, r)
		case err, ok := <-errs:
			if !ok {
				break outer
			}
			return err
		}
	}

	sort.Sort(results)

	results = filterResults(results, filters)
	data, err := rep.Report(results)
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

func filterResults(results korra.Results, filters string) korra.Results {
	trimmed := strings.TrimSpace(filters)
	if trimmed == "" {
		return results
	}
	filterGroup := newFilterGroup(trimmed, results)
	var filtered korra.Results
	for _, result := range results {
		if filterGroup.Matches(result) {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

type ResultFilterGroup struct {
	filters []func(*korra.Result) bool
}

func newFilterGroup(filterSpecs string, results korra.Results) ResultFilterGroup {
	group := ResultFilterGroup{}
	for _, filterSpec := range strings.Split(filterSpecs, " ") {
		pieces := strings.Split(filterSpec, "=")
		switch pieces[0] {
		case "Method":
			group.filters = append(group.filters, func(result *korra.Result) bool {
				return result.Method == pieces[1]
			})
		case "Path":
			group.filters = append(group.filters, func(result *korra.Result) bool {
				return strings.Contains(result.Path, pieces[1])
			})
		// Examples:
		//    Time=1m  => Include results from start to 1 minute after start
		//    Time=-1m  => (same as above)
		//    Time=+1m => Include results from 1 minute after start to end
		case "Time":
			durationText := pieces[1]
			lookback := true
			direction := durationText[0:1]
			if direction == "-" || direction == "+" {
				lookback = direction == "-"
				durationText = durationText[1:]
			}
			duration, err := time.ParseDuration(durationText)
			if err != nil {
				panic(fmt.Errorf("Bad 'Time' filter specification: %s", err))
			}
			anchorTime := results[0].Timestamp.Add(duration)
			group.filters = append(group.filters, func(result *korra.Result) bool {
				if lookback {
					return result.Timestamp.Before(anchorTime)
				}
				return result.Timestamp.After(anchorTime)
			})
		}
	}
	return group
}

func (g *ResultFilterGroup) Matches(result *korra.Result) bool {
	for _, filter := range g.filters {
		if !filter(result) {
			return false
		}
	}
	return true
}
