package korra

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type SessionAction struct {
	Raw    string
	Line   int
	Target *Target
}

func (action *SessionAction) BadLine(offset int, message string) error {
	return fmt.Errorf("[Line: %d] %s", action.Line+offset, message)
}

type SessionScript struct {
	Actions []SessionAction
	Current int
}

func (script *SessionScript) ActionCount() int {
	return len(script.Actions)
}

func (script *SessionScript) Progress() float32 {
	return float32(script.Current+1) / float32(script.ActionCount())
}

func (script *SessionScript) ProgressLabel() string {
	return fmt.Sprintf("%d of %d", script.Current+1, script.ActionCount())
}

var (
	supportedMethods = []string{"HEAD", "GET", "PUT", "POST", "PATCH", "OPTIONS"}
	httpMethod       = regexp.MustCompile(fmt.Sprintf("^(%s)$", strings.Join(supportedMethods, "|")))
	httpMethodLine   = regexp.MustCompile(fmt.Sprintf("^(POLL\\s+)?(%s)", strings.Join(supportedMethods, "|")))
)

func NewScript(script io.Reader) (*SessionScript, error) {
	var actions []SessionAction
	for _, action := range ScanActions(script) {
		if err := action.CreateTarget(); err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	return &SessionScript{Actions: actions, Current: 0}, nil
}

func (action *SessionAction) CreateTarget() error {
	tgt := Target{Header: http.Header{}}
	lines := strings.Split(action.Raw, "\n")
	firstLine := strings.TrimSpace(lines[0])
	var tokens []string
	if strings.HasPrefix(firstLine, "PAUSE") {
		tokens = strings.SplitN(firstLine, " ", 2)
		pauseTime, err := strconv.Atoi(strings.TrimSpace(tokens[1]))
		if err != nil {
			return action.BadLine(0, fmt.Sprintf("Expected int as argument to PAUSE, got %s", tokens[1]))
		}
		tgt.PauseTime = pauseTime
		action.Target = &tgt
		return nil
	} else if strings.HasPrefix(firstLine, "COMMENT") {
		tgt.Comment = strings.SplitN(firstLine, " ", 2)[1]
		action.Target = &tgt
		return nil
	}

	// everything else starts with a URL action, possibly preceded by POLL
	tokens = strings.SplitN(firstLine, " ", 3)
	if len(tokens) < 2 || (tokens[0] == "POLL" && len(tokens) == 2) {
		return action.BadLine(0, "Invalid number of arguments for URL command")
	}
	var matches []string
	if matches = httpMethodLine.FindStringSubmatch(firstLine); matches == nil {
		return action.BadLine(0, fmt.Sprintf("Invalid HTTP method: %s", tokens[0]))
	}

	var checkUrl string
	if len(matches) == 2 {
		tgt.Method = tokens[0]
		checkUrl = tokens[1]
	} else {
		// we'll get polling config in a later line
		tgt.Method = tokens[1]
		checkUrl = tokens[2]
	}
	if _, err := url.ParseRequestURI(checkUrl); err != nil {
		return action.BadLine(0, fmt.Sprintf("Invalid URL: %s", checkUrl))
	}
	tgt.URL = checkUrl

	for idx, line := range lines[1:] {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		if strings.HasPrefix(line, "@") {
			bodyFile := line[1:]
			bodyInfo, err := os.Stat(bodyFile)
			if err != nil {
				return action.BadLine(idx, fmt.Sprintf("Invalid body file reference '%s': %s", bodyFile, err))
			}
			tgt.BodyPath = bodyFile
		} else if strings.HasPrefix(line, "[") {
			if err := tgt.Poller.FillFromLine(line[1 : len(line)-1]); err != nil {
				return action.BadLine(idx, fmt.Sprintf("Bad poll params '%s': %s", line, err))
			}
		} else {
			headerTokens := strings.SplitN(line, ":", 2)
			if len(headerTokens) < 2 {
				return action.BadLine(idx, fmt.Sprintf("Bad header '%s': Expected two colon-delimited values", line))
			}
			for i := range headerTokens {
				if headerTokens[i] = strings.TrimSpace(headerTokens[i]); headerTokens[i] == "" {
					return action.BadLine(idx, fmt.Sprintf("Bad header '%s': Expected non-blank value", line))
				}
			}
			tgt.Header.Add(headerTokens[0], headerTokens[1])
		}
	}
	return nil
}

var (
	extensionCommand       = regexp.MustCompile("^=>")
	internalCommentCommand = regexp.MustCompile("^//")
)

// Given a file with:
//   GET /foo/bar
//   Header:Value
//   // this is a comment and will not echo in the output
//   POST /foo/bar/baz
//   Header:Value
//   Header-Two:Value
//   @path/to/body
//
//   PAUSE 12345
//   COMMENT - this line will be ignored
//
//   POLL GET {url}
//   Header-Three:Value
//   [status=200 count=5 wait=2500]
//
// Generate a series of SessionAction objects whose 'Raw'
// attribute includes the contents of each
// [
//   "GET /foo/bar\nHeader:Value",
//   "POST /foo/bar/baz\nHeader:Value\nHeader-Two:Value\n@path/to/body",
//   "POLL GET /foo/bar?created=true\nHeader-Three:Value\n[status=200 count=5 wait=2500]",
//   "=> PAUSE 12345",
//   "=> COMMENT - this line will be ignored"
// ]
func ScanActions(reader io.Reader) []SessionAction {
	var actions []SessionAction
	lineNumber := 0

	sc := peekingScanner{src: bufio.NewScanner(reader)}
	for sc.Scan() {
		lineNumber += 1
		line := strings.TrimSpace(sc.Text())
		if line == "" || internalCommentCommand.MatchString(line) {
			continue
		}
		current := []string{line}
		for {
			nextLine := sc.Peek()
			if nextLine == "" || internalCommentCommand.MatchString(nextLine) {
				sc.Text() // discard and finish the action
				break
			} else if extensionCommand.MatchString(nextLine) || httpMethodLine.MatchString(nextLine) {
				break // done with this target but keep the scanner at the line
			} else {
				sc.Scan()
				current = append(current, sc.Text())
			}
		}
		action := SessionAction{Raw: strings.Join(current, "\n"), Line: lineNumber}
		actions = append(actions, action)
	}
	return actions
}
