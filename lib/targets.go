package korra

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type TargetPoller struct {
	Active           bool
	UntilCount       int
	UntilStatus      *Regexp // cache these?
	WaitBetweenPolls int
}

func (poller *TargetPoller) FillFromLine(line string) error {
	poller.Active = true
	for _, piece := range strings.Split(strings.TrimSpace(line), " ") {
		param := strings.SplitN(piece, "=", 2)
		if len(param) != 2 {
			return fmt.Errorf("Expected key=value for poll param, got: %s", piece)
		}
		name := strings.ToLower(param[0])
		if name == "status" {
			poller.UntilStatus = regexp.MustCompile(param[1])
		} else {
			if num, err := strconv.Atoi(param[1]); err != nil {
				switch name {
				case "count":
					poller.UntilCount = num
				case "wait":
					poller.WaitBetweenPolls = num
				}
			}
		}
	}
	return nil
}

func (poller *TargetPoller) ShouldRetry(requestCount int, statusCode int) bool {
	return poller.Active &&
		requestCount <= poller.UntilCount &&
		!poller.MatchString(strconv.Itoa(statusCode))

}

// Target is an HTTP request blueprint.
type Target struct {
	PauseTime int
	Comment   string
	Method    string
	URL       string
	BodyPath  string
	Header    http.Header
	Poller    TargetPoller
}

// Body reads the full body specified by the BodyPath and returns a Reader; if
// there is a blank BodyPath it returns a nil Reader
func (t *Target) Body() (io.Reader, error) {
	if t.BodyPath == "" {
		return nil, nil
	}
	if bodyBytes, err := ioutil.ReadFile(t.BodyPath); err != nil {
		return nil, err
	} else {
		return bytes.NewReader(bodyBytes), nil
	}
}

func (t *Target) IsComment() bool {
	return t.Comment != ""
}

func (t *Target) IsPause() bool {
	return t.PauseTime > 0
}

// NewTarget creates a new target from an array of strings representing a single target.
// Four examples:

// 1. A command to pause for 5819 ms
//    PAUSE 5819

// 2. A command to fetch a URL
//    GET http://foo/bar

// 3. A command to send a body + header to a URL
//    POST http://foo/baz
//    Content-Type: application/json
//    @scripts/post/1234/1.json

// 4. A command to poll a URL until status 201 or 5 requests made, waiting 1.5 sec between each
//    POLL GET http://ray/bans
//    [Status=201 Count=5 Wait=1500]
// Request creates an *http.Request out of Target and returns it along with an
// error in case of failure.
func (t *Target) Request() (*http.Request, error) {
	var (
		body io.Reader
		err  error
		req  *http.Request
	)
	if body, err = t.Body(); err != nil {
		return nil, err
	}
	if req, err := http.NewRequest(t.Method, t.URL, body); err != nil {
		return nil, err
	}
	for k, vs := range t.Header {
		req.Header[k] = make([]string, len(vs))
		copy(req.Header[k], vs)
	}
	if host := req.Header.Get("Host"); host != "" {
		req.Host = host
	}
	return req, nil
}

// Wrap a Scanner so we can cheat and look at the next value and react accordingly,
// but still have it be around the next time we Scan() + Text()
type peekingScanner struct {
	src    *bufio.Scanner
	peeked string
}

func (s *peekingScanner) Err() error {
	return s.src.Err()
}

func (s *peekingScanner) Peek() string {
	if !s.src.Scan() {
		return ""
	}
	s.peeked = s.src.Text()
	return s.peeked
}

func (s *peekingScanner) Scan() bool {
	if s.peeked == "" {
		return s.src.Scan()
	}
	return true
}

func (s *peekingScanner) Text() string {
	if s.peeked == "" {
		return s.src.Text()
	}
	t := s.peeked
	s.peeked = ""
	return t
}
