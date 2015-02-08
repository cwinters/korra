# Korra

<!-- [Build Status](https://secure.travis-ci.org/cwinters/korra.png)](http://travis-ci.org/cwinters/korra) -->

![Korra looks over Republic City](http://upload.wikimedia.org/wikipedia/en/5/5f/Legend_of_Korra_concept_art.png)

__Korra__ builds on [Vegeta](http://github.com/tsenart/vegeta)
to process sessions that simulate one or many users moving through a prescribed
workflow. This is quite different from typical load testing tools, and if
you're trying to flood your site with customers randomly browsing your product
catalog you should use something like:

* [Vegeta](http://github.com/tsenart/vegeta)
* [wrk](https://github.com/wg/wrk)
* [http-perf](http://www.hpl.hp.com/research/linux/httperf/)
* [Siege](http://www.joedog.org/siege-home/)

or any of a number of other, far more mature tools.

So __Korra__ works with sessions you've scripted, walking through actions one
at a time until they're complete and waiting until all the sessions are
complete to finish up. Every session is separate from every other session, and
we use Go's concurrency model to potentially represent many thousands of users
on a single node.

__Korra__ doesn't care how the sessions are generated, it's just concerned with
moving users through the flow and reporting on them. User scripts are
represented in plain text in a format very similar to Vegeta, allowing custom
headers and body per request along with additional directives to pause between
steps, or poll a URL until a specified halt condition.

## TO DO

* Reporting rethink (be able to output to file for pg `COPY`?)
    * Put URLs into buckets that you specify (or we infer) and do histograms,
      time based trends, etc.
    * Filters for reporting and dump
    * Slice by user, URL, timespan
* Tests (currently just brought over from Vegeta, boo)
* Document in README: sample scripts, more discussion of generating script
* godoc, other stuff I don't know about
* Add thing about configuring Linux with higher ulimits

## Install

You need [go](http://golang.org/) installed and `GOBIN` in your `PATH`. Once
that is done, run:

```shell
$ go get github.com/cwinters/korra
$ go install github.com/cwinters/korra
```

After that run `korra` from the command-line to see if you've got everything
setup.

## Scripts

Scripts are text files that specify a few commands:

* Execute an HTTP request
* Pause execution
* Output a comment to the log

Some commands comprise a single line, but they can also use successive lines
for additional context.

### HTTP commands

An HTTP command looks like:

    http-method url
    [header-key: header-value]
    [@request-body-reference]

The first line is common to pretty much every load testing tool -- an HTTP
method and URL to hit. __Korra__ supports the HTTP methods: GET, HEAD, OPTIONS,
PATCH, POST, and PUT. Adding more is fairly trivial but we need a good use case.

Similar to [Vegeta](https://github.com/tsenart/vegeta) __Korra__ supports both
per-request headers and request bodies.  Headers are sent as-is, though we trim
any leading and trailing whitespace from both the key and value. Empty values
are not allowed.

Request body references are paths relative to the script file with the content
of the body, and __Korra__ will refuse to start if any body reference is
invalid.

For example, here's a sequence of two HTTP commands separated by a pause:

    GET http://link.to/your/self
    PAUSE 4850
    GET http://link.to/your/team

And here's the same thing but with additional context for each HTTP command:

    GET http://link.to/your/self
    Authorization: Token ABCDEFG
    PAUSE 4850
    GET http://link.to/your/team
    Authorization: Token ABCDEFG
    Accept: image/*

Here's another example, this time with headers and a body reference:

    POST http://link.to/your/team
    Authorization: Token ABCDEFG
    Content-Type: application/json
    @post/user_4512/1.json
    PATCH http://link.to/your/team/55
    Authorization: Token ABCDEFG
    Content-Type: application/json-patch+json
    @post/user_4512/2.json

Each HTTP request will result in an entry in the performance data. You'll see
entries in the log like:

    15:36:53.542024 user_110213.txt 8/23: 200 => GET /pages/students/answers/4321, 374 ms
    15:36:53.543519 user_112635.txt 11/19: 201 => POST /api/assignments/7654/share, 53 ms

### Polling HTTP commands

Polling HTTP commands are only slightly different from HTTP commands -- prefix
the method with `POLL` and allow an optional set of parameters to configure
polling:

    POLL http-method url
    [header-key: header-value]
    [@request-body-reference]
    [[Wait=time-in-ms Count=max-polls Status=regex-indicating-stop]]

By default the polling parameters are:

    [Wait=1000 Count=5 Status=^2\d\d$]

which means we'll poll the given method + URL, waiting 1 second between polls,
until the first of:

* we've polled five times, or
* we get a 200 status

So say we've got a `GET` that will return a `204` until our resource is fully
baked, at which point it will return a `200`. To poll for that we might do
this:

    POLL GET http://api.com/my/baked/resource
    Accept: application/json
    Authorization: Token ABCDEF
    [Wait=1500 Status=200]

Each poll will result in a separate entry in the performance data.

You might see the above command result in the log entries:

    15:36:52.26492 user_112762.txt 12/31: 204 => GET /my/baked/resource, 29 ms
    15:36:52.264939 user_112762.txt 12/31: Attempt 1 requires retry, 2000 ms pause until next poll
    15:36:54.289954 user_112762.txt 12/31: 204 => GET /my/baked/resource, 24 ms
    15:36:54.289974 user_112762.txt 12/31: Attempt 2 requires retry, 2000 ms pause until next poll
    15:36:56.312294 user_112762.txt 12/31: 204 => GET /my/baked/resource, 22 ms
    15:36:56.312315 user_112762.txt 12/31: Attempt 3 requires retry, 2000 ms pause until next poll
    15:36:58.385529 user_112762.txt 12/31: 200 => GET /my/baked/resource, 73 ms

which would turn up in four separate entries in the performance data.

Additionally every result has the `RequestCount` attribute; for non-polled
requests this will always be `1`. For polled requests it will request the poll
number (starting at 1) so you can report on a distribution of how many polls it
takes to retrieve a particular resource.

### Pauses

A `PAUSE` does what it says, pauses that session a given number of
milliseconds:

    PAUSE 5918

If you watch the log you'll see this logged as:

    15:36:21.668284 user_112762.txt 10/31: Sleeping (30453 ms)...

Pausing has no impact on any other session.

### Comments

A `COMMENT` just results in a message sent to the log, with the message as
everything after the `COMMENT` dierective. So from:

    COMMENT ===== Start assignment 'Thinking about the emotions of color'

You'll see a logging message:

    15:36:24.912834 user_112762.txt 11/31: ===== Start assignment 'Thinking about the emotions of color'

Comments have no functional impact on the session.

## Commands

### Arguments

#### Globs and directories

Arguments that take a single file can typically take a 
[glob](http://golang.org/pkg/path/filepath/#Glob). But you need to ensure that
__Korra__ gets the glob, not the shell. So you'll need to single-quote it:

    # OK
    $ korra validate -file 'data/*.bin'
    $ korra report -inputs 'data/user_1*.bin'

    
    # NOT OK
    $ korra validate -file data/*.bin
    $ korra report -inputs data/user_1*.bin

Additionally we'll try to do the right thing if you provide a directory
instead. An argument defining scripts will expand a `{directory}` to
`{directory}/*.txt`; one defining results will expand it to
`{directory}/*.bin`.

### Sessions

The `sessions` command is the heart of __Korra__. It takes a directory of
session files and executes them all, recording performance data for each. (It
also takes a series of arguments to configure HTTP client behavior, but we'll
deal with that later.)


### Validate

The `validate` command tells you as much as it can about whether your scripts
can be processed without actually running them. It will check all of the
following:

* HTTP invocations are valid (e.g., not just `POLL GET`)
* HTTP methods are valid
* HTTP URLs can be parsed
* HTTP body file references exist
* Headers have values
* `PAUSE` has an integer argument
* Polling parameters are integers or valid regular expressions

These checks are done for all actions in the specified file and default
behavior is to display only problems. Passing in `-verbose` will display a
summary of every action, which can be a useful sanity check.

Examples against a single file and glob:

    $ korra validate -file user_19949.txt
    ===== FILE user_19949.txt OK

    $ korra validate -file user_19950.txt
    ===== FILE user_19950.txt FAIL 2
    Line 2: Invalid HTTP method: POLLGET
    Line 10: Expected int as argument to PAUSE, got 'a-while'

    $ korra validate -file 'scripts/*.txt'
    ===== FILE scripts/user_105967.txt FAIL 1
    Line 10: Expected int as argument to PAUSE, got 'a-while'
    ===== FILE scripts/user_105968.txt OK
    ===== FILE scripts/user_105969.txt OK

### Dump

The `dump` command just serializes every performance result from the Go
serialization format ([gob](http://golang.org/pkg/encoding/gob/)) to either CSV
or JSON.

### Report


## Use

```shell
$ korra -h
Usage: korra [globals] <command> [options]

sessions command:
  -cert="": x509 Certificate file
  -keepalive=true: Use persistent connections
  -laddr=0.0.0.0: Local IP address
  -redirects=10: Number of redirects to follow. -1 will not follow but marks as success
  -dir=path/to/scripts: Directory full of scripts and bodies
  -timeout=0: Requests timeout

report command:
  -inputs='glob-of-input-files': Glob of .bin files to include in the report; if directory will append '*.bin'
  -output="stdout": Output file
  -reporter="text": Reporter [text, json, plot, hist[buckets]]
  -exclude='': Filter expression to exclude transactions from the report and include all others
  -include='': Filter expression to include transactions from the report and exclude all others

dump command:
  -dumper="": Dumper [json, csv]
  -inputs='glob-of-input-files': Glob of .bin files to include in the dump; if directory will append '*.bin'
  -output="stdout": Output file
  -exclude='': Filter expression to exclude transactions from the report and include all others
  -include='': Filter expression to include transactions from the report and exclude all others

global flags:
  -cpus=8 Number of CPUs to use

examples:
  korra sessions -dir=student_devices/ -log=student_devices.log
  korra report -inputs='student_devices/*.bin' -reporter=json > metrics.json
  korra report -inputs='student_devices/' -reporter=plot > plot.html
  korra report -inputs='student_devices/student_01*.bin' -reporter="hist[0,100ms,200ms,300ms]"
```

* __-cpus__: Specifies the number of CPUs to be used internally. It defaults to
  the amount of CPUs available in the system.

### sessions

```shell
$ korra sessions -h
Usage of korra sessions:
  -cert="": x509 Certificate file
  -dir="some-dir": Directory with .txt files
  -keepalive=true: Use persistent connections
  -laddr=0.0.0.0: Local IP address
  -output="stdout": Output log of overall status
  -redirects=10: Number of redirects to follow
  -timeout=30s: Requests timeout
```

* __-cert__: Specifies the x509 TLS certificate to be used with HTTPS requests.
* __-dir__: Specifies a directory with session scripts, see examples above.
* -keepalive: Specifies whether to reuse TCP connections between HTTP requests.
* __-laddr__:  Specifies the local IP address to be used.
* __-output__: Specifies the output log you can monitor to see overall status. Defaults to stdout.  (TODO: format of log output)
* __-redirects__: Specifies the max number of redirects followed on each request.
  The default is 10. When the value is -1, redirects are not followed but the
  response is marked as successful.
* __-timeout__: Specifies the timeout for each request. The default is 0 which disables
  timeouts.

### report
```
$ korra report -h
Usage of korra report:
  -inputs="": Input files (comma separated, dir with *.bin files, or glob)
  -output="stdout": Output file
  -reporter="text": Reporter [text, json, plot, hist[buckets]]
  -
```

* __-inputs__: Specifies the input files from which we'll generate the report.
  These are the output of korra sessions. You can specify more than one (comma
  separated, as a glob, or as a directory with *.bin files) and they will be
  merged and sorted before being used by the reports.
* __-output__: Specifies the output file to which the report will be written to.
* __-reporter__: Specifies the kind of report to be generated. It defaults to text.

##### text

```
Requests      [total]                   1200
Duration      [total, attack, wait]     10.094965987s, 9.949883921s, 145.082066ms
Latencies     [mean, 50, 95, 99, max]   113.172398ms, 108.272568ms, 140.18235ms, 247.771566ms, 264.815246ms
Bytes In      [total, mean]             3714690, 3095.57
Bytes Out     [total, mean]             0, 0.00
Success       [ratio]                   55.42%
Status Codes  [code:count]              0:535  200:665
Error Set:
Get http://localhost:6060: dial tcp 127.0.0.1:6060: connection refused
Get http://localhost:6060: read tcp 127.0.0.1:6060: connection reset by peer
Get http://localhost:6060: dial tcp 127.0.0.1:6060: connection reset by peer
Get http://localhost:6060: write tcp 127.0.0.1:6060: broken pipe
Get http://localhost:6060: net/http: transport closed before response was received
Get http://localhost:6060: http: can't write HTTP request on broken connection
```

##### json

```json
{
  "latencies": {
    "mean": 9093653647,
    "50th": 2401223400,
    "95th": 12553709381,
    "99th": 12604629125,
    "max": 12604629125
  },
  "bytes_in": {
    "total": 782040,
    "mean": 651.7
  },
  "bytes_out": {
    "total": 0,
    "mean": 0
  },
  "duration": 9949883921,
  "wait": 145082066,
  "requests": 1200,
  "success": 0.11666666666666667,
  "status_codes": {
    "0": 1060,
    "200": 140
  },
  "errors": [
    "Get http://localhost:6060: dial tcp 127.0.0.1:6060: operation timed out"
  ]
}
```

##### plot

Generates an HTML5 page with an interactive plot based on
[Dygraphs](http://dygraphs.com).
Click and drag to select a region to zoom into. Double click to zoom
out.
Input a different number on the bottom left corner input field
to change the moving average window size (in data points).

![Plot](http://i.imgur.com/oi0cgGq.png)

##### hist

Computes and prints a text based histogram for the given buckets.
Each bucket upper bound is non-inclusive.
```
korra report -inputs=path/to/results -reporter='hist[0,2ms,4ms,6ms]'
Bucket         #     %       Histogram
[0,     2ms]   6007  32.65%  ########################
[2ms,   4ms]   5505  29.92%  ######################
[4ms,   6ms]   2117  11.51%  ########
[6ms,   +Inf]  4771  25.93%  ###################
```

### dump

```
$ korra dump -h
Usage of korra dump:
  -dumper="": Dumper [json, csv]
  -inputs="": Input files (comma separated, glob, or directory with *.bin files)
  -output="stdout": Output file
```

#### -inputs

Specifies the input files to be dumped. These are the output of korra sessions.
You can specify more than one (comma separated, as a glob, or as a directory
with *.bin files) and they will be merged and sorted before being dumped.

#### -output

Specifies the output file to which the dump will be written to.

#### -dumper

Specifies the dump format.

##### json

Dumps attack results as JSON objects.

##### csv

Dumps attack results as CSV records with six columns.
The columns are: unix timestamp in ns since epoch, http status code,
request latency in ns, bytes out, bytes in, and lastly the error.

## Limitations

You could be CPU bound (unlikely), memory bound (more likely) or
have system resource limits being reached which ought to be tuned for
the process execution. The important limits for us are file descriptors
and processes. On a UNIX system you can get and set the current
soft-limit values for a user.

```shell
$ ulimit -n # file descriptors
2560
$ ulimit -u # processes / threads
709
```
Just pass a new number as the argument to change it.


## Why "Korra"?

[Vegeta](http://nicktoons.nick.com/shows/dragon-ball-z-kai/characters/vegeta.html)
is a character on Dragonball Z who attacks a lot, so it's appropriate. I
thought another cartoon character would be fun to use for this, and the last
episode of [Legend of Korra](http://en.wikipedia.org/wiki/The_Legend_of_Korra)
had recently ended so it was on my mind.

![Balance, you say](http://imageserver.moviepilot.com/header-these-actors-need-to-be-in-a-live-action-legend-of-korra-movie.jpeg?width=500&height=281)

First, I thought it would be great to pick a female character. But more
specifically, Korra as the Avatar strives to bring balance to the world. She
doesn't always know how to do it, or even make it possible once she does. But
she tries. Load testing is something people rarely do, and when it's done we're
often trying to explore the balance between features and performance, or
between features and scaling.

It's a stretch, but I'll stick with it.

## License

```
The MIT License (MIT)

Copyright (c) 2015 Chris Winters

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
the Software, and to permit persons to whom the Software is furnished to do so,
subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
```
