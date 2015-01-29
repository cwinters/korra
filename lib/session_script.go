package korra

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type SessionScript struct {
	Actions *[]SessionAction
	Current int
}

func (script *SessionScript) ActionCount() int {
	return len(*script.Actions)
}

func (script *SessionScript) IsValid() bool {
	for _, action := range *script.Actions {
		if action.Error != nil {
			return false
		}
	}
	return true
}

func (script *SessionScript) Progress() float32 {
	return float32(script.Current+1) / float32(script.ActionCount())
}

func (script *SessionScript) ProgressLabel() string {
	return fmt.Sprintf("%d of %d", script.Current+1, script.ActionCount())
}

// NewScript creates a new script of SessionAction objects from the given
// reader, ensuring that each action is a valid one -- see `CreateTarget`
// for the validation checks it performs. Therefore the returned error will be either
// an IO error (from reading the script) or the first invalid action it
// encounters.
func NewScript(script io.Reader) (*SessionScript, error) {
	actions, err := ScanActions(script)
	if err != nil {
		return nil, err
	}
	var scriptActions []SessionAction
	for _, action := range *actions {
		if err := action.CreateTarget(); err != nil {
			return nil, err
		}
		scriptActions = append(scriptActions, action)
	}
	return &SessionScript{Actions: &scriptActions, Current: 0}, nil
}

// CheckScript creates a new script of SessionAction objects from
// the given reader and validates all the targets similar to `NewScript`,
// but it does not halt processing at the first invalid action. Instead it
// parses and returns all of them and the ones with an error have
// their `Error` property set. You can also check the validity of
// the entire script with `IsValid()`
func CheckScript(script io.Reader) (*SessionScript, error) {
	if actions, err := ScanActions(script); err != nil {
		return nil, err
	} else {
		for _, action := range *actions {
			action.CreateTarget()
		}
		return &SessionScript{Actions: actions, Current: 0}, nil
	}
}

var (
	supportedMethods = []string{"HEAD", "GET", "PUT", "POST", "PATCH", "OPTIONS"}
	httpMethod       = regexp.MustCompile(fmt.Sprintf("^(%s)$", strings.Join(supportedMethods, "|")))
	httpMethodLine   = regexp.MustCompile(fmt.Sprintf("^(POLL\\s+)?(%s)", strings.Join(supportedMethods, "|")))
)

type SessionAction struct {
	Raw    string
	Line   int
	Error  error
	Target *Target
}

func (action *SessionAction) BadLine(offset int, message string) error {
	action.Error = fmt.Errorf("[Line: %d] %s", action.Line+offset, message)
	return action.Error
}

// CreateTarget parses the string stored in the `SessionAction.Raw`
// property and checks:
// * if it's a valid action
// * that the action's URL is valid (if the action has a URL)
// * that a header value is specified (if the action lists any request headers)
// * that the file with the request body exists (if one is specified)
// * that the polling parameters are valid ones (if polling is being used)
func (action *SessionAction) CreateTarget() error {
	tgt := NewTarget()
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
		action.Target = tgt
		return nil
	} else if strings.HasPrefix(firstLine, "COMMENT") {
		tgt.Comment = strings.SplitN(firstLine, " ", 2)[1]
		action.Target = tgt
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
		tgt.Poller.Active = true
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
			if err != nil || bodyInfo.IsDir() {
				var display string
				if err != nil {
					display = fmt.Sprintf("%s", err) // TODO: fix when online, dummy!
				} else {
					display = "is a directory, not a file"
				}
				return action.BadLine(idx, fmt.Sprintf("Invalid request body reference '%s': %s", bodyFile, display))
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
	action.Target = tgt
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
func ScanActions(reader io.Reader) (*[]SessionAction, error) {
	var actions []SessionAction
	lineNumber := 0

	sc := peekingScanner{src: bufio.NewScanner(reader)}
	for sc.Scan() {
		if sc.Err() != nil {
			return nil, sc.Err()
		}
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
	return &actions, nil
}
