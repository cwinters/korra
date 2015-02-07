package korra

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
)

type SessionProgress struct {
	Complete   bool
	Actions    int
	Current    int
	Percentage float32
}

type SessionScript struct {
	Actions []*SessionAction
	Current int
}

func (script *SessionScript) ActionCount() int {
	return len(script.Actions)
}

func (script *SessionScript) ActionsRemain() bool {
	return len(script.Actions) > script.Current
}

func (script *SessionScript) IsValid() bool {
	for _, action := range script.Actions {
		if action.Error != nil {
			return false
		}
	}
	return true
}

func (script *SessionScript) NextAction() *SessionAction {
	action := script.Actions[script.Current]
	script.Current += 1
	return action
}

func (script *SessionScript) Progress() SessionProgress {
	return SessionProgress{
		Actions:    script.ActionCount(),
		Current:    script.Current,
		Complete:   script.Current == script.ActionCount(),
		Percentage: (float32(script.Current) / float32(script.ActionCount())) * 100,
	}
}

func (script *SessionScript) ProgressLabel() string {
	return fmt.Sprintf("%d/%d", script.Current, script.ActionCount())
}

// NewScript creates a new script of SessionAction objects from the given
// reader, ensuring that each action is a valid one -- see `CreateTarget`
// for the validation checks it performs. Therefore the returned error will be either
// an IO error (from reading the script) or the first invalid action it
// encounters.
func NewScript(scriptPath string) (*SessionScript, error) {
	var (
		err            error
		scannedActions []*SessionAction
		script         io.Reader
		validActions   []*SessionAction
	)

	if script, err = scriptFile(scriptPath); err != nil {
		return nil, err
	}
	if scannedActions, err = ScanActions(script); err != nil {
		return nil, err
	}
	scriptDir := path.Dir(scriptPath)
	for _, action := range scannedActions {
		if err := action.CreateTarget(scriptDir); err != nil {
			return nil, err
		}
		validActions = append(validActions, action)
	}
	return &SessionScript{Actions: validActions, Current: 0}, nil
}

// CheckScript creates a new script of SessionAction objects from
// the given reader and validates all the targets similar to `NewScript`,
// but it does not halt processing at the first invalid action. Instead it
// parses and returns all of them and the ones with an error have
// their `Error` property set. You can also check the validity of
// the entire script with `IsValid()`
func CheckScript(scriptPath string) (*SessionScript, error) {
	script, err := scriptFile(scriptPath)
	if err != nil {
		return nil, err
	}
	if actions, err := ScanActions(script); err != nil {
		return nil, err
	} else {
		scriptDir := path.Dir(scriptPath)
		for _, action := range actions {
			action.CreateTarget(scriptDir)
		}
		return &SessionScript{Actions: actions, Current: 0}, nil
	}
}

func scriptFile(scriptPath string) (io.Reader, error) {
	fi, err := os.Stat(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("error reading session file %s: %s", scriptPath, err)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("error reading session file %s: is directory", scriptPath)
	}
	if fi.Size() == 0 {
		return nil, fmt.Errorf("error reading session file %s: empty", scriptPath)
	}
	script, err := os.Open(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("error opening session file: %s: %s", scriptPath, err)
	}
	return script, nil
}

var (
	// TODO support additional methods via config? environment variable with added?
	supportedMethods = []string{"HEAD", "GET", "PUT", "POST", "PATCH", "OPTIONS"}
	httpMethod       = regexp.MustCompile(fmt.Sprintf("^(%s)$", strings.Join(supportedMethods, "|")))
	httpMethodLine   = regexp.MustCompile(fmt.Sprintf("^(POLL )?(%s)", strings.Join(supportedMethods, "|")))
)

type SessionAction struct {
	Raw    string
	Line   int
	Error  error
	Target *Target
}

func (action *SessionAction) BadLine(offset int, message string) error {
	action.Error = fmt.Errorf("Line %d: %s", action.Line+offset, message)
	return action.Error
}

// CreateTarget parses the string stored in the `SessionAction.Raw`
// property and checks:
// * if it's a valid action
// * that the action's URL is valid (if the action has a URL)
// * that a header value is specified (if the action lists any request headers)
// * that the file with the request body exists (if one is specified)
// * that the polling parameters are valid ones (if polling is being used)
func (action *SessionAction) CreateTarget(scriptDir string) error {
	tgt := NewTarget()
	lines := strings.Split(action.Raw, "\n")
	firstLine := strings.TrimSpace(lines[0])
	var tokens []string
	if strings.HasPrefix(firstLine, "PAUSE") {
		tokens = strings.SplitN(firstLine, " ", 2)
		pauseTime, err := strconv.Atoi(strings.TrimSpace(tokens[1]))
		if err != nil {
			return action.BadLine(0, fmt.Sprintf("Expected int as argument to PAUSE, got '%s'", tokens[1]))
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
	if matches = httpMethodLine.FindStringSubmatch(firstLine); matches == nil || len(matches) == 0 {
		return action.BadLine(0, fmt.Sprintf("Invalid HTTP method: %s", tokens[0]))
	}
	var checkUrl string
	if matches[1] == "POLL " {
		tgt.Poller.Active = true // we'll get polling config in a later line
		tgt.Method = tokens[1]
		checkUrl = tokens[2]
	} else {
		tgt.Method = tokens[0]
		checkUrl = tokens[1]
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
			bodyFile := path.Join(scriptDir, line[1:])
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
			pollingConfig := line[1 : len(line)-1]
			if err := tgt.Poller.FillFromLine(pollingConfig); err != nil {
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

func (action *SessionAction) String() string {
	return fmt.Sprintf("[%d] %s", action.Line, action.Target)
}

var (
	externalCommentCommand = regexp.MustCompile("^COMMENT")
	internalCommentCommand = regexp.MustCompile("^//")
	pauseCommand           = regexp.MustCompile("^PAUSE")
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
func ScanActions(reader io.Reader) ([]*SessionAction, error) {
	var actions []*SessionAction
	lineNumber := 0

	sc := peekingScanner{src: bufio.NewScanner(reader)}
	for sc.Scan() {
		if sc.Err() != nil {
			return nil, sc.Err()
		}
		lineNumber += 1
		startLine := lineNumber
		line := strings.TrimSpace(sc.Text())
		if line == "" || internalCommentCommand.MatchString(line) {
			continue
		}
		current := []string{line}
		if !isSingleLineCommand(line) {
			for {
				nextLine := sc.Peek()
				if nextLine == "" || internalCommentCommand.MatchString(nextLine) {
					sc.Text() // discard and finish the action
					break
				} else if httpMethodLine.MatchString(nextLine) || isSingleLineCommand(nextLine) {
					break // done with this target but keep the scanner at the line
				} else {
					sc.Scan() // everything else is an HTTP command, just keep appending
					current = append(current, sc.Text())
					lineNumber += 1
				}
			}
		}
		action := &SessionAction{Raw: strings.Join(current, "\n"), Line: startLine}
		actions = append(actions, action)
	}
	return actions, nil
}

func isSingleLineCommand(line string) bool {
	return pauseCommand.MatchString(line) || externalCommentCommand.MatchString(line)
}
