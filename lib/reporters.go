package korra

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

type PathBucket struct {
	Results   Results
	pieces    []string
	variances []int
}

func NewPathBucket(pathPieces []string, result *Result) PathBucket {
	results := Results{result}
	bucket := PathBucket{results, pathPieces, make([]int, len(pathPieces))}
	fmt.Printf("Created new Path bucket [%s], variances: %s\n", pathPieces, bucket.variances)
	return bucket
}

func PathToPieces(path string) []string {
	normalized := strings.Trim(path, "/")
	return strings.Split(normalized, "/")
}

func (b *PathBucket) Track(checkPieces []string, result *Result) bool {
	if len(checkPieces) != len(b.pieces) {
		return false
	}
	variantCount, variantIdx := 0, -1
	for idx, toCheck := range checkPieces {
		if toCheck != b.pieces[idx] {
			variantCount += 1
			variantIdx = idx
		}
	}
	if variantCount > 1 {
		return false
	}
	b.variances[variantIdx] += 1
	b.Results = append(b.Results, result)
	return true
}

func (b *PathBucket) String() string {
	toDisplay := make([]string, len(b.pieces))
	for idx, piece := range b.pieces {
		if b.variances[idx] == 0 {
			toDisplay[idx] = piece
		} else {
			toDisplay[idx] = "*"
		}
	}
	return "/" + strings.Join(toDisplay, "/")
}

func CreateBuckets(results Results) []PathBucket {
	var buckets []PathBucket
	for _, result := range results {
		pathPieces := PathToPieces(result.Path)
		foundBucket := -1
		for idx, bucket := range buckets {
			if bucket.Track(pathPieces, result) {
				foundBucket = idx
				break
			}
		}
		if foundBucket == -1 {
			buckets = append(buckets, NewPathBucket(pathPieces, result))
		}
	}
	return buckets
}

// Reporter is an interface defining Report computation.
type Reporter interface {
	Report(Results) ([]byte, error)
}

// ReporterFunc is an adapter to allow the use of ordinary functions as
// Reporters. If f is a function with the appropriate signature, ReporterFunc(f)
// is a Reporter object that calls f.
type ReporterFunc func(Results) ([]byte, error)

// Report implements the Reporter interface.
func (f ReporterFunc) Report(r Results) ([]byte, error) { return f(r) }

// HistogramReporter is a reporter that computes latency histograms with the
// given buckets.
type HistogramReporter []time.Duration

// Report implements the Reporter interface.
func (h HistogramReporter) Report(r Results) ([]byte, error) {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 8, 2, ' ', tabwriter.StripEscape)

	bucket := func(i int) string {
		if i+1 >= len(h) {
			return fmt.Sprintf("[%s,\t+Inf]", h[i])
		}
		return fmt.Sprintf("[%s,\t%s]", h[i], h[i+1])
	}

	fmt.Fprintf(w, "Bucket\t\t#\t%%\tHistogram\n")
	for i, count := range Histogram(h, r) {
		ratio := float64(count) / float64(len(r))
		fmt.Fprintf(w, "%s\t%d\t%.2f%%\t%s\n",
			bucket(i),
			count,
			ratio*100,
			strings.Repeat("#", int(ratio*75)),
		)
	}

	err := w.Flush()
	return buf.Bytes(), err
}

// Set implements the flag.Value interface.
func (h *HistogramReporter) Set(value string) error {
	for _, v := range strings.Split(value[1:len(value)-1], ",") {
		d, err := time.ParseDuration(v)
		if err != nil {
			return err
		}
		*h = append(*h, d)
	}
	if len(*h) == 0 {
		return fmt.Errorf("bad buckets: %s", value)
	}
	return nil
}

// String implements the fmt.Stringer interface.
func (h HistogramReporter) String() string {
	strs := make([]string, len(h))
	for i := range strs {
		strs[i] = strconv.FormatInt(int64(h[i]), 10)
	}
	return "[" + strings.Join(strs, ",") + "]"
}

var ReportTest ReporterFunc = func(r Results) ([]byte, error) {
	buckets := CreateBuckets(r)
	out := &bytes.Buffer{}
	for _, bucket := range buckets {
		fmt.Fprintf(out, "%s: %d\n", bucket.String(), len(bucket.Results))
	}
	return out.Bytes(), nil
}

// ReportText returns a computed Metrics struct as aligned, formatted text.
var ReportText ReporterFunc = func(r Results) ([]byte, error) {
	m := NewMetrics(r)
	out := &bytes.Buffer{}

	w := tabwriter.NewWriter(out, 0, 8, 2, '\t', tabwriter.StripEscape)
	fmt.Fprintf(w, "Requests\t[total]\t%d\n", m.Requests)
	fmt.Fprintf(w, "Duration\t[total, attack, wait]\t%s, %s, %s\n", m.Duration+m.Wait, m.Duration, m.Wait)
	fmt.Fprintf(w, "Latencies\t[mean, 50, 95, 99, max]\t%s, %s, %s, %s, %s\n",
		m.Latencies.Mean, m.Latencies.P50, m.Latencies.P95, m.Latencies.P99, m.Latencies.Max)
	fmt.Fprintf(w, "Bytes In\t[total, mean]\t%d, %.2f\n", m.BytesIn.Total, m.BytesIn.Mean)
	fmt.Fprintf(w, "Bytes Out\t[total, mean]\t%d, %.2f\n", m.BytesOut.Total, m.BytesOut.Mean)
	fmt.Fprintf(w, "Success\t[ratio]\t%.2f%%\n", m.Success*100)
	fmt.Fprintf(w, "Status Codes\t[code:count]\t")
	for code, count := range m.StatusCodes {
		fmt.Fprintf(w, "%s:%d  ", code, count)
	}
	fmt.Fprintln(w, "\nError Set:")
	for _, err := range m.Errors {
		fmt.Fprintln(w, err)
	}

	if err := w.Flush(); err != nil {
		return []byte{}, err
	}
	return out.Bytes(), nil
}

// ReportJSON writes a computed Metrics struct to as JSON
var ReportJSON ReporterFunc = func(r Results) ([]byte, error) {
	return json.Marshal(NewMetrics(r))
}

// ReportPlot builds up a self contained HTML page with an interactive plot
// of the latencies of the requests. Built with http://dygraphs.com/
var ReportPlot ReporterFunc = func(r Results) ([]byte, error) {
	series := &bytes.Buffer{}
	for i, point := 0, ""; i < len(r); i++ {
		point = "[" + strconv.FormatFloat(
			r[i].Timestamp.Sub(r[0].Timestamp).Seconds(), 'f', -1, 32) + ","

		if r[i].Error == "" {
			point += "NaN," + strconv.FormatFloat(r[i].Latency.Seconds()*1000, 'f', -1, 32) + "],"
		} else {
			point += strconv.FormatFloat(r[i].Latency.Seconds()*1000, 'f', -1, 32) + ",NaN],"
		}

		series.WriteString(point)
	}
	// Remove trailing commas
	if series.Len() > 0 {
		series.Truncate(series.Len() - 1)
	}

	return []byte(fmt.Sprintf(plotsTemplate, dygraphJSLibSrc(), series)), nil
}

const plotsTemplate = `<!doctype>
<html>
<head>
  <title>Korra Plots</title>
</head>
<body>
  <div id="latencies" style="font-family: Courier; width: 100%%; height: 600px"></div>
  <a href="#" download="korraplot.png" onclick="this.href = document.getElementsByTagName('canvas')[0].toDataURL('image/png').replace(/^data:image\/[^;]/, 'data:application/octet-stream')">Download as PNG</a>
  <script>
	%s
  </script>
  <script>
  new Dygraph(
    document.getElementById("latencies"),
    [%s],
    {
      title: 'Korra Plot',
      labels: ['Seconds', 'ERR', 'OK'],
      ylabel: 'Latency (ms)',
      xlabel: 'Seconds elapsed',
      showRoller: true,
      colors: ['#FA7878', '#8AE234'],
      legend: 'always',
      logscale: true,
      strokeWidth: 1.3
    }
  );
  </script>
</body>
</html>`
