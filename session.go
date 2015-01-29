package main

import (
	"bytes"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

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

func sessionCmd() command {
	fs := flag.NewFlagSet("korra session", flag.ExitOnError)
	opts := &sessionOpts{
		headers: headers{http.Header{}},
		laddr:   localAddr{&korra.DefaultLocalAddr},
	}

	fs.StringVar(&opts.certf, "cert", "", "x509 Certificate file")
	fs.Var(&opts.headers, "header", "Request header")
	fs.BoolVar(&opts.keepalive, "keepalive", true, "Use persistent connections")
	fs.Var(&opts.laddr, "laddr", "Local IP address")
	fs.StringVar(&opts.logf, "log", "stdout", "Overall log")
	fs.IntVar(&opts.redirects, "redirects", korra.DefaultRedirects, "Number of redirects to follow. -1 will not follow but marks as success")
	fs.StringVar(&opts.sessiond, "dir", "", "Directory of sessions")
	fs.DurationVar(&opts.timeout, "timeout", korra.DefaultTimeout, "Requests timeout")

	return command{fs, func(args []string) error {
		fs.Parse(args)
		return session(opts)
	}}
}

var (
	errBadCert    = errors.New("bad certificate")
	errMissingDir = errors.New("directory must exist and have at least one .txt file")
)

// sessionOpts aggregates the session function command options
type sessionOpts struct {
	certf     string
	headers   headers
	keepalive bool
	laddr     localAddr
	logf      string
	redirects int
	sessiond  string
	timeout   time.Duration
}

// session validates the attack arguments, sets up the
// required resources, launches the attack and writes the results
func session(opts *sessionOpts) error {
	files := map[string]io.Reader{}
	for _, filename := range []string{opts.certf} {
		if filename == "" {
			continue
		}
		f, err := file(filename, false)
		if err != nil {
			return fmt.Errorf("error opening %s: %s", filename, err)
		}
		defer f.Close()
		files[filename] = f
	}

	var (
		err      error
		sessions []*korra.Session
	)

	var cert []byte
	if certf, ok := files[opts.certf]; ok {
		if cert, err = ioutil.ReadAll(certf); err != nil {
			return fmt.Errorf("error reading %s: %s", opts.certf, err)
		}
	}
	tlsc := *korra.DefaultTLSConfig
	if opts.certf != "" {
		if tlsc.RootCAs, err = certPool(cert); err != nil {
			return err
		}
	}

	sessionOptions := []func(*korra.Attacker){
		korra.Redirects(opts.redirects),
		korra.Timeout(opts.timeout),
		korra.LocalAddr(*opts.laddr.IPAddr),
		korra.TLSConfig(&tlsc),
		korra.KeepAlive(opts.keepalive),
	}
	sessionPattern := fmt.Sprintf("%s/*.txt", opts.sessiond)
	for _, sessionFile := range globInputs(sessionPattern) {
		reader, err := file(sessionFile, false)
		if err != nil {
			return fmt.Errorf("error reading session file %s: %s", sessionFile, err)
		}
		session, err := korra.NewSession(sessionFile, reader, sessionOptions)
		if err != nil {
			return fmt.Errorf("Error reading user script %s: %s", sessionFile, err)
		}
		sessions = append(sessions, session)
	}

	var wg sync.WaitGroup

	for _, session := range sessions {
		wg.Add(1)
		go func(user *korra.Session) {
			defer wg.Done()
			session.Run()
		}(session)
	}

	// catch completion of all sessions, and from the OS
	var done = make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt)
	go func() {
		wg.Wait()
		done <- os.Interrupt
	}()

	for {
		select {
		case <-done:
			for _, session := range sessions {
				session.Stop() // wait for each user to finish up?
			}
			return nil
		}
	}

	return nil
}

// headers is the http.Header used in each target request
// it is defined here to implement the flag.Value interface
// in order to support multiple identical flags for request header
// specification
type headers struct{ http.Header }

func (h headers) String() string {
	buf := &bytes.Buffer{}
	if err := h.Write(buf); err != nil {
		return ""
	}
	return buf.String()
}

func (h headers) Set(value string) error {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("header '%s' has a wrong format", value)
	}
	key, val := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if key == "" || val == "" {
		return fmt.Errorf("header '%s' has a wrong format", value)
	}
	h.Add(key, val)
	return nil
}

// localAddr implements the Flag interface for parsing net.IPAddr
type localAddr struct{ *net.IPAddr }

func (ip *localAddr) Set(value string) (err error) {
	ip.IPAddr, err = net.ResolveIPAddr("ip", value)
	return
}

// certPool returns a new *x509.CertPool with the passed cert included.
// An error is returned if the cert is invalid.
func certPool(cert []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(cert) {
		return nil, errBadCert
	}
	return pool, nil
}
