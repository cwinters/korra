package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	korra "github.com/cwinters/korra/lib"
)

type scanOpts struct {
	scanf string
}

func scanCmd() command {
	fs := flag.NewFlagSet("korra scan", flag.ExitOnError)
	opts := &scanOpts{}
	fs.StringVar(&opts.scanf, "file", "", "File to scan and check targets")
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

	fmt.Println("CHUNKS")
	for idx, chunk := range korra.ScanFileToChunks(f) {
		fmt.Printf("%d:\n%s\n", idx, chunk)
	}
	f.Close()

	fmt.Println("TARGETS")
	f, _ = file(opts.scanf, false)
	for idx, target := range korra.ScanFileToTargets(f) {
		fmt.Printf("%d:\n  Method %s\n  URL %s\n", idx, target.Method, target.URL)
	}
	f.Close()
	return nil
}

func usersCmd() command {
	fs := flag.NewFlagSet("korra users", flag.ExitOnError)

	opts := &usersOpts{
		laddr: localAddr{&korra.DefaultLocalAddr},
	}
	fs.StringVar(&opts.usersd, "users", "", "Directory with user scripts, each one with .txt extension")
	fs.StringVar(&opts.certf, "cert", "", "x509 Certificate file")
	fs.IntVar(&opts.redirects, "redirects", korra.DefaultRedirects, "Number of redirects to follow")
	fs.Var(&opts.laddr, "laddr", "Local IP address")
	fs.BoolVar(&opts.keepalive, "keepalive", true, "Use persistent connections")
	return command{fs, func(args []string) error {
		fs.Parse(args)
		return users(opts)
	}}
}

type usersOpts struct {
	certf     string
	keepalive bool
	laddr     localAddr
	redirects int
	timeout   time.Duration
	usersd    string
}

func users(opts *usersOpts) error {
	var (
		err       error
		userFiles []string
	)
	tlsc := *korra.DefaultTLSConfig
	if opts.certf != "" {
		var cert []byte
		if cert, err = ioutil.ReadFile(opts.certf); err != nil {
			return fmt.Errorf("error reading %s: %s", opts.certf, err)
		}
		if opts.certf != "" {
			if tlsc.RootCAs, err = certPool(cert); err != nil {
				return err
			}
		}
	}

	userOptions := []func(*korra.Attacker){
		korra.Redirects(opts.redirects),
		korra.Timeout(opts.timeout),
		korra.LocalAddr(*opts.laddr.IPAddr),
		korra.TLSConfig(&tlsc),
		korra.KeepAlive(opts.keepalive),
	}

	userPattern := fmt.Sprintf("%s/*.txt", opts.usersd)
	if userFiles, err = filepath.Glob(userPattern); err != nil {
		return fmt.Errorf("error reading user files %s: %s", opts.usersd, err)
	}

	var users []*korra.User
	for _, userFile := range globInputs(opts.usersd) {
		reader, err := file(userFile, false)
		if err != nil {
			return fmt.Errorf("error reading user file %s: %s", userFile, err)
		}
		users = append(users, korra.NewUser(userFile, reader, userOptions))
	}

	var wg sync.WaitGroup

	for _, user := range users {
		wg.Add(1)
		go func(user *korra.User) {
			defer wg.Done()
			user.Run()
		}(user)
	}

	// catch completion of all users, and from the OS
	var done = make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt)
	go func() {
		wg.Wait()
		done <- os.Interrupt
	}()

	for {
		select {
		case <-done:
			for _, user := range users {
				user.Stop() // wait for each user to finish up?
			}
			return nil
		}
	}

	return nil
}
