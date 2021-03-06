package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"time"

	korra "github.com/cwinters/korra/lib"
)

type reportOpts struct {
	filters  string
	inputs   string
	output   string
	reporter string
	showurls bool
	urlf     string
}

func reportCmd() command {
	opts := &reportOpts{}

	fs := flag.NewFlagSet("korra report", flag.ExitOnError)
	fs.StringVar(&opts.filters, "filters", "", "One or more space-separated filters to operate on subsets of the inputs")
	fs.StringVar(&opts.inputs, "inputs", ".", "Input files (comma separated, glob, or dir with .bin files; cwd*)")
	fs.StringVar(&opts.output, "output", "stdout", "Report output destination (stdout*)")
	fs.StringVar(&opts.reporter, "reporter", "text", "Reporter [text*, json, plot, dump, hist[buckets]]")
	fs.BoolVar(&opts.showurls, "show-urls", false, "If true show all URLs in bucket -- may be long! (false*)")
	fs.StringVar(&opts.urlf, "urls", "", "File from which I should read URL patterns for analysis; if not given I'll infer them from the results")

	return command{fs, func(args []string) error {
		fs.Parse(args)
		return report(opts)
	}}
}

func chooseReporter(opts *reportOpts) (korra.Reporter, error) {
	var err error
	switch opts.reporter {
	case "text":
		if opts.urlf == "" {
			return korra.TextReporter{ShowUrls: opts.showurls}, nil
		}
		var in io.Reader
		if in, err = korra.File(opts.urlf, false); err != nil {
			return nil, fmt.Errorf("bad URL pattern file: '%s'", err)
		}
		urls := make([]string, 0)
		scanner := bufio.NewScanner(in)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			urls = append(urls, line)
		}
		buckets := korra.BucketCollection{}
		if err = buckets.CreateBucketsFromSpecs(urls); err != nil {
			return nil, err
		}
		return korra.TextReporter{Collection: buckets, ShowUrls: opts.showurls}, nil
	case "json":
		return korra.ReportJSON, nil
	case "hist":
		if len(opts.reporter) < 6 {
			return nil, fmt.Errorf("bad buckets: '%s'", opts.reporter[4:])
		}
		var hist korra.HistogramReporter
		if err := hist.Set(opts.reporter[4:len(opts.reporter)]); err != nil {
			return nil, err
		}
		return hist, nil
	}

	return nil, fmt.Errorf("unknown reporter: %s", opts.reporter)
}

// report validates the report arguments, sets up the required resources
// and writes the report
func report(opts *reportOpts) error {
	var (
		err error
		in  *os.File
		out *os.File
		rep korra.Reporter
	)
	if rep, err = chooseReporter(opts); err != nil {
		return err
	}
	files := korra.GlobResults(opts.inputs)
	srcs := make([]io.Reader, len(files))
	for i, f := range files {
		if in, err = korra.File(f, false); err != nil {
			return err
		}
		defer in.Close()
		srcs[i] = in
	}
	if out, err = korra.File(opts.output, true); err != nil {
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

	results = filterResults(results, opts.filters)
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
		case "Latency":
			latencyPieces := strings.Split(pieces[1], "-")
			min, max := 0, 0
			var err error
			if latencyPieces[0] == "" {
				max, err = strconv.Atoi(latencyPieces[1])
			} else if latencyPieces[1] == "" {
				min, err = strconv.Atoi(latencyPieces[0])
			} else {
				min, err = strconv.Atoi(latencyPieces[0])
				if err == nil {
					max, err = strconv.Atoi(latencyPieces[1])
				}
			}
			if err != nil {
				panic(fmt.Errorf("Bad 'Latency' filter specification [%s]: %s", pieces[1], err))
			}
			group.filters = append(group.filters, func(result *korra.Result) bool {
				resultMillis := int(result.Latency) / 1000
				return resultMillis >= min && resultMillis <= max
			})
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
			timeSpecText := pieces[1]
			lookback := true
			direction := timeSpecText[0:1]
			if direction == "-" || direction == "+" {
				lookback = direction == "-"
				timeSpecText = timeSpecText[1:]
			}
			duration, err := time.ParseDuration(timeSpecText)
			if err != nil {
				panic(fmt.Errorf("Bad 'Time' filter specification [%s]: %s", pieces[1], err))
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
