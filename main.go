package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
)

func main() {
	commands := map[string]command{
		"dump":     dumpCmd(),
		"report":   reportCmd(),
		"sessions": sessionsCmd(),
		"validate": validateCmd(),
	}

	flag.Usage = func() {
		fmt.Println("Usage: korra [globals] <command> [options]")
		for name, cmd := range commands {
			fmt.Printf("\n%s command:\n", name)
			cmd.fs.PrintDefaults()
		}
		fmt.Printf("\nglobal flags:\n  -cpus=%d Number of CPUs to use\n", runtime.NumCPU())
		fmt.Println(examples)
	}

	cpus := flag.Int("cpus", runtime.NumCPU(), "Number of CPUs to use")
	flag.Parse()

	runtime.GOMAXPROCS(*cpus)

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	if cmd, ok := commands[args[0]]; !ok {
		log.Fatalf("Unknown command: %s", args[0])
	} else if err := cmd.fn(args[1:]); err != nil {
		log.Fatal(err)
	}
}

const examples = `
examples:
  korra sessions -dir=path/to/sessions > overall-status.log
  korra report -inputs='path/to/results/*.bin' -reporter=json > metrics.json
  korra report -inputs='path/to/results' -reporter=plot > plot.html
`

type command struct {
	fs *flag.FlagSet
	fn func(args []string) error
}
