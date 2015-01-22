# Korra [![Build Status](https://secure.travis-ci.org/cwinters/korra.png)](http://travis-ci.org/cwinters/korra)

Korra builds on [Vegeta](http://github.com/tsenart/vegeta)
to process sessions that simulate one or many users moving through a workflow.
While Vegeta focuses on maintaining a constant request rate of HTTP requests,
Korra focuses on representing scripted user sessions, using Go's concurrency
model to potentially represent many thousands of users on a single node.

Korra doesn't care how the sessions are generated, it's just concerned with
moving users through and reporting on them. User scripts are represented in
plain text in a format very similar to Vegeta, allowing custom headers and body
per request along with additional directives to pause between steps, or poll a
URL until either a particular status is received or a certain number of
requests sent.

## Install

You need go installed and `GOBIN` in your `PATH`. Once that is done, run the
command:
```shell
$ go get github.com/cwinters/korra
$ go install github.com/cwinters/korra
```

## Usage manual
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

#### -cpus
Specifies the number of CPUs to be used internally.
It defaults to the amount of CPUs available in the system.

### sessions
```shell
$ korra sessions -h
Usage of korra sessions:
  -cert="": x509 Certificate file
  -keepalive=true: Use persistent connections
  -laddr=0.0.0.0: Local IP address
  -output="stdout": Output log of overall status
  -redirects=10: Number of redirects to follow
  -targets="some-dir": Directory with .txt files
  -timeout=30s: Requests timeout
```

#### -cert
Specifies the x509 TLS certificate to be used with HTTPS requests.

#### -keepalive
Specifies whether to reuse TCP connections between HTTP requests.

#### -laddr
Specifies the local IP address to be used.

#### -output
Specifies the output log you can monitor to see overall status. Defaults to stdout.

#### -redirects
Specifies the max number of redirects followed on each request. The
default is 10. When the value is -1, redirects are not followed but
the response is marked as successful.

#### -targets
Specifies a directory with session scripts, each a line separated file.
The format should be as follows, combining any or all of the following:

Simple targets
```
GET http://goku:9090/path/to/dragon?item=balls
GET http://user:password@goku:9090/path/to
HEAD http://goku:9090/path/to/success
```

Targets with custom headers
```
GET http://user:password@goku:9090/path/to
X-Account-ID: 8675309

DELETE http://goku:9090/path/to/remove
Confirmation-Token: 90215
Authorization: Token DEADBEEF
```

Targets with custom bodies
```
POST http://goku:9090/things
@/path/to/newthing.json

PATCH http://goku:9090/thing/71988591
@/path/to/thing-71988591.json
```

Targets with custom bodies and headers
```
POST http://goku:9090/things
X-Account-ID: 99
@/path/to/newthing.json
```

#### -timeout
Specifies the timeout for each request. The default is 0 which disables
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

#### -inputs

Specifies the input files from which we'll generate the report.  These are the
output of korra sessions. You can specify more than one (comma
separated, as a glob, or as a directory with *.bin files) and they will be
merged and sorted before being used by the reports.

#### -output

Specifies the output file to which the report will be written to.

#### -reporter

Specifies the kind of report to be generated. It defaults to text.

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

## More examples

### Sample scripts

### Sample filters


#### Limitations

There will be an upper bound of the supported `rate` which varies on the
machine being used.

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

## Licence
```
The MIT License (MIT)

Copyright (c) 2014 Chris Winters

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
