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

So each concurrent session that __Korra__ processes is backed by a *script*,
which is just an ordered sequence of simple actions. How you generate this
script is up to you. It's a plain text format and in a straightforward format
very similar to Vegeta, allowing custom headers and body per request along with
additional directives to pause between steps, or poll a URL until a specified
halt condition.

It walks through these actions one at a time in a *session*, logging
information about each transaction.  Every session is separate from every other
session, and we use Go's concurrency model to potentially represent many
thousands of users on a single node.

Once all sessions are complete you can report on the results. Transaction
results can be dumped to JSON or CSV formats so you can process with your
favorite tools, and there are some simple reports __Korra__ can generate out of
the box.

## TO DO

* Sample scripts
* Discuss script generation strategy
* Reporting rethink
    * Do histograms and time based trends for URL buckets -- that is, use
      them in places other than text reporter
    * Time based trends across URLs
    * Filters for reporting and dump (keep this?)
    * Be able to output to file for pg `COPY`
    * Add request count distribution for text reporters
* Tests (currently just brought over from Vegeta, boo)
* Point sessions to URL to get tarball with data and to upload results
   * Start 'leader' process with pointer to directory of tarballs + common key
     followers must provide to get + report data
   * Doles them out one at a time to followers via `/data`; follower must
     include some unique name + common key
   * Takes results via `/data` along with unique name + common key
* godoc, other stuff I don't know about

## Install and Run: CLI

You need [go](http://golang.org/) installed and `GOBIN` in your `PATH`. Once
that is done, run:

```shell
$ go get github.com/cwinters/korra
$ go install github.com/cwinters/korra
```

After that run `korra` from the command-line to see if you've got everything
setup.

## Install and Run: Docker

You can also reference a Docker container and mount your scripts. You'll need
to mount your directory of scripts to `/app/scripts`, which is where the output
will be written as well:

    $ docker run -v /path/to/myscripts:/app/scripts cwinters/korra

You can also use the container to run other commands -- for example, you can
validate your scripts:

    $ docker run -v /path/to/myscripts:/app/scripts cwinters/korra validate

Or generate reports:

    $ docker run -v /path/to/myscripts:/app/scripts cwinters/korra report

These simple commands work because many commands default to the current working
directory for inputs, and the container sets its `WORKDIR` to `/app/scripts`.

Here's a simple example executing a script and running a report on it:

    $ mkdir sample
    
    $ echo 'GET http://cwinters.com' > sample/one.txt
    
    $ ls sample
    one.txt
    
    $ docker run -v $(pwd)/sample:/app/scripts cwinters/korra
      ...no output...
    
    $ ls sample
    one.bin one.txt
    
    $ docker run -v $(pwd)/sample:app/scripts cwinters/korra report
    OVERALL: 1 results
    Requests	[total]				1
    Duration	[total, attack, wait]		136.790285ms, 0, 136.790285ms
    Latencies	[mean, 50, 95, 99, max]		136.790285ms, 136.790285ms, 136.790285ms, 136.790285ms, 136.790285ms
    Bytes In	[total, mean]			0, 0.00
    Bytes Out	[total, mean]			0, 0.00
    Success		[ratio]				100.00%
    Status Codes	[code:count]			200:1
    Error Set:
    GET /: 1 results
    Requests	[total]				1
    Duration	[total, attack, wait]		136.790285ms, 0, 136.790285ms
    Latencies	[mean, 50, 95, 99, max]		136.790285ms, 136.790285ms, 136.790285ms, 136.790285ms, 136.790285ms
    Bytes In	[total, mean]			0, 0.00
    Bytes Out	[total, mean]			0, 0.00
    Success		[ratio]				100.00%
    Status Codes	[code:count]			200:1
    Error Set:

That doesn't exercise much __Korra__ functionality, but it gives you an idea of
how easy it is to run with containers.

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

Similar to [Vegeta](https://github.com/tsenart/vegeta) __Korra__ supports
custom headers and bodies per-request.  Headers are sent as-is, though we trim
any leading and trailing whitespace from both the key and value. Empty values
are not allowed, and invalid request body references will prevent __Korra__
from starting. You can check both with the `validate` command (see below).

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

Polling HTTP commands have only two differences from HTTP commands:

1. prefix the method with `POLL`,
2. optionally tell __Korra__ when she should stop polling.

Here's what it looks like:

    POLL http-method url
    [header-key: header-value]
    [@request-body-reference]
    [[Wait=time-in-ms Count=max-polls Status=regex-indicating-stop]]

By default the polling parameters are:

    [Wait=1000 Count=5 Status=^2\d\d$]

which means we'll request the given method + URL, waiting 1 second between
polls, until the first of:

* we've polled five times, or
* we get a status between 200 and 299

So say we've got a `GET` that will return a `204` until our resource is fully
baked, at which point it will return a `200`. To poll for that at a two-second
interval we'd do:

    POLL GET http://api.com/my/baked/resource
    Accept: application/json
    Authorization: Token ABCDEF
    [Wait=2000 Status=200]

Each poll will result in a separate transaction entry for reporting.

If you've got verbose logging on you might see the above command result in the
log entries:

    15:36:52.26492 user_112762.txt 12/31: 204 => GET /my/baked/resource, 29 ms
    15:36:52.264939 user_112762.txt 12/31: Attempt 1 requires retry, 2000 ms pause until next poll
    15:36:54.289954 user_112762.txt 12/31: 204 => GET /my/baked/resource, 24 ms
    15:36:54.289974 user_112762.txt 12/31: Attempt 2 requires retry, 2000 ms pause until next poll
    15:36:56.312294 user_112762.txt 12/31: 204 => GET /my/baked/resource, 22 ms
    15:36:56.312315 user_112762.txt 12/31: Attempt 3 requires retry, 2000 ms pause until next poll
    15:36:58.385529 user_112762.txt 12/31: 200 => GET /my/baked/resource, 73 ms

which would turn up in four separate entries in the performance data.

Additionally every transaction result has the `RequestCount` attribute; for
non-polled requests this will always be `1`. For polled requests it will
request the poll number (starting at 1) so you can report on a distribution of
how many polls it takes to retrieve a particular resource.

### Pauses

A `PAUSE` does what it says, pauses that session a given number of
milliseconds:

    PAUSE 5918

If you verbose logging is on you'll see this logged as:

    15:36:21.668284 user_112762.txt 10/31: Sleeping (5918 ms)...

Pausing has no impact on any other session, and doesn't show up in any
transaction result.

### Comments

A `COMMENT` just results in a message sent to the log, with the message as
everything after the `COMMENT` dierective. So from:

    COMMENT ===== Start assignment 'Thinking about the emotions of color'

You'll see a logging message:

    15:36:24.912834 user_112762.txt 11/31: ===== Start assignment 'Thinking about the emotions of color'

Comments show up even if you do not have verbose logging on. They have no
functional impact on the session and do not show up in any transaction result.

## Command arguments

### Globs and directories

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
instead. An argument specifying scripts will expand a `{directory}` to
`{directory}/*.txt`; one defining results will expand it to
`{directory}/*.bin`.

## Sessions command

The `sessions` command is the heart of __Korra__. It takes a directory of
session files and executes them all, recording performance data for each. (It
also takes a series of arguments to configure HTTP client behavior, but we'll
deal with that later.)

### Logging

The overall log, which defaults to STDOUT, will print overall status
periodically; if you provide the `-verbose` argument (or are in `-pretend`
mode) you'll get progress for every session. Even if you're verbosely logging
you can get a high-level view of progress with:

    ubuntu@ip-10-3-2-144:~/loadtest$ tail -f 20150217-962-1x-10sp.log  | grep complete
    15:44:09.074761 29651/30564 actions complete (97.01%); 914/962 sessions complete (95.01%)
    15:44:39.075068 29721/30564 actions complete (97.24%); 916/962 sessions complete (95.22%)
    15:45:09.075379 29787/30564 actions complete (97.46%); 922/962 sessions complete (95.84%)

The period length defaults to 30 seconds, you can change it with the `-status`
option.

## Validate command

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

## Dump command

The `dump` command just serializes every performance result from the Go
serialization format ([gob](http://golang.org/pkg/encoding/gob/)) to either CSV
or JSON.

## Report command

The `report` command takes a set of transaction files and summarizes them in
some way. There some functionality removed that's in Vegeta -- we concentrate
on creating some useful summaries and allowing some configuration of them
(i.e., defining the URL patterns to cut across) and then allowing you to export
data to formats you can use in other tools.

One thing you can do is define URL buckets we'll report on. If you don't
provide them we'll try to infer patterns from what we see -- which is
rudimentary right now, just looking at digit-only path pieces and treating
those as variable.

If you define them yourself just put one method and pattern per line in a file
and reference that file from the `report` command. A pattern is a URL path with
a `*` where paths may vary.

For example, the following would group blog posts by month:

    GET /2015/04/*/*
    GET /2015/03/*/*
    GET /2015/02/*/*
    GET /2015/01/*/*
    GET /2014/12/*/*
    GET /2014/11/*/*
    GET /2014/10/*/*

And if you run a report with `-show-urls` you might see output like this:

    GET /2015/02/*/*: 4 results
    Requests	[total]				4
    Duration	[total, attack, wait]		200.747558ms, 130.282399ms, 70.465159ms
    Latencies	[mean, 50, 95, 99, max]		83.355554ms, 79.039844ms, 85.634748ms, 85.634748ms, 98.282467ms
    Bytes In	[total, mean]			0, 0.00
    Bytes Out	[total, mean]			0, 0.00
    Success		[ratio]				100.00%
    Status Codes	[code:count]			200:4
    Error Set:
    URLs in bucket:
    	/2015/02/01/some-follow-ups.html: 2
    	/2015/02/28/spreadsheets.html: 2
    GET /2015/01/*/*: 21 results
    Requests	[total]				21
    Duration	[total, attack, wait]		1.231406471s, 1.165029385s, 66.377086ms
    Latencies	[mean, 50, 95, 99, max]		77.709283ms, 78.112329ms, 87.614862ms, 87.93501ms, 89.317957ms
    Bytes In	[total, mean]			0, 0.00
    Bytes Out	[total, mean]			0, 0.00
    Success		[ratio]				100.00%
    Status Codes	[code:count]			200:21
    Error Set: (empty)
    URLs in bucket:
    	/2015/01/07/til-aggregate-and-aws.html: 2
    	/2015/01/08/thinking-about-multitasking.html: 2
    	/2015/01/09/til-bash-n-stuff.html: 2
    	/2015/01/13/til-newrelic-venv-hamartia.html: 3
    	/2015/01/21/tir-psql-export-csv.html: 3
    	/2015/01/24/iphone-keyboards.html: 3
    	/2015/01/25/making-waffles.html: 3

You don't need to create patterns that cover every URL that might come up during
your run. Behind the scenes we'll create a 'catch-all' bucket, and every result
that doesn't match your pre-defined patterns will go into that bucket.

## Limitations

Test runs generally don't tax your system too much, unless you're running many
tens or hundreds of thousands of concurrent sessions. While it's possible for
them to be CPU bound or memory bound it's much more likely that you'll run into
limits on file descriptors -- for every concurrent session we have one
filehandle open, plus potentially one network handle.

On a UNIX system you can get and set the current soft-limit values for a user:

```shell
$ ulimit -n # file descriptors
2560
```
Just pass a new number as the argument to change it. You may hit a
system-defined cap that you as a user cannot exceed. In that case you'll have
to talk with your sysadmin to raise them.

For example, on some GNU/Linux systems you'd create a file in
`/etc/security/limits.d` with something like:

    * soft    nofile   32768
    * hard    nofile   32768

to modify the ceiling to 32768 open file descriptors. (See
[How do I increase the open files limit for a non-root user?](http://askubuntu.com/questions/162229/how-do-i-increase-the-open-files-limit-for-a-non-root-user)
for more.)

On OS X you should be able to do the same by editing `/etc/launchd.conf` with:

    limit maxfiles 512 32768

See [Change Mac OS X User Limits](http://www.lecentre.net/blog/archives/686)
for more.

## Why "Korra"?

[Vegeta](http://nicktoons.nick.com/shows/dragon-ball-z-kai/characters/vegeta.html)
is a character on Dragonball Z who attacks a lot, so it's appropriate. I
thought another cartoon character would be fun to use for this, and the last
episode of [Legend of Korra](http://en.wikipedia.org/wiki/The_Legend_of_Korra)
had recently ended so it was on my mind.

![Balance, you say](http://imageserver.moviepilot.com/header-these-actors-need-to-be-in-a-live-action-legend-of-korra-movie.jpeg?width=500&height=281)

First, I thought it would be great to pick a female character. But more
specifically, Korra as the Avatar strives to bring balance to the world. She
doesn't always know how to do it, and even when she does know she struggles to
make it happen and deal with the consequences. But she tries.

Load testing is something people rarely do, and when it's done we're often
trying to explore the balance between features and performance, or between
features and scaling.

Also, since Korra can manipulate the elements you can think of __Korra__ as
flooding your site -- or blowing it down, or burning it... you get the idea.

They're stretches, but I'll stick with it.

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

This is a fork of the excellent [Vegeta](http://github.com/tsenart/vegeta)
project, which is also MIT licensed.

