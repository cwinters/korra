package korra

import (
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
)

type BucketCollection struct {
	buckets        []*PathBucket
	catchAllBucket *PathBucket
}

func NewBucketCollection() BucketCollection {
	return BucketCollection{make([]*PathBucket, 0), nil}
}

func (bc *BucketCollection) Buckets() []*PathBucket {
	return bc.buckets
}

func (bc *BucketCollection) AddResults(results Results) {
	for _, result := range results {
		pathPieces := pathToPieces(result.Path)
		matchedBucket := bc.findPathBucket(pathPieces, result)
		if matchedBucket == nil {
			bc.buckets = append(bc.buckets, NewPathBucketFromResult(pathPieces, result))
		} else {
			matchedBucket.AddResult(result)
		}
	}
}

func (bc *BucketCollection) CatchAllBucket() *PathBucket {
	return bc.catchAllBucket
}

func (bc *BucketCollection) CreateBucketsFromSpecs(lines []string) error {
	for _, line := range lines {
		pieces := strings.SplitN(line, " ", 2)
		if len(pieces) != 2 {
			if pieces[0] == "*" {
				bc.catchAllBucket = NewPathBucketCatchAll()
			} else {
				return fmt.Errorf("Bad bucket definition, expect METHOD PATH, got '%s'", line)
			}
		}

		bucket := NewPathBucketFromStrings(pieces[0], pieces[1])
		if bucket == nil {
			return fmt.Errorf("Bad bucket definition: %s", line)
		} else {
			bc.buckets = append(bc.buckets, bucket)
		}
	}

	// see if there's a catch-all bucket, and if not add one
	if bc.catchAllBucket == nil {
		bc.catchAllBucket = NewPathBucketCatchAll()
	}

	return nil
}

func (bc *BucketCollection) findPathBucket(pathPieces []string, result *Result) *PathBucket {
	for _, bucket := range bc.buckets {
		if bucket.Match(pathPieces, result) {
			return bucket
		}
	}
	return bc.catchAllBucket // either actual catch-all or nil is fine as return
}

// PathBucket is a grouping of results by their path and method; each
// one may have variance for certain parts of the path -- for example,
// '/foo/details/12' and '/foo/details/18591' may be in the same bucket
// because they vary only by the last piece of the path.
type PathBucket struct {
	Results       Results           // all the results maching this method + path
	method        string            // HTTP method for these results
	pieces        []string          // path broken into pieces
	variantPieces []bool            // true/false for each piece of the path; true means it can vary
	Urls          map[string]uint32 // track URL counts in this bucket
}

var (
	digitsPiece = regexp.MustCompile("^\\d+$")
	trimQuery   = regexp.MustCompile("\\?.*$")
)

func NewPathBucketCatchAll() *PathBucket {
	return &PathBucket{make([]*Result, 0), "*", []string{"*"}, []bool{true}, make(map[string]uint32)}
}

func NewPathBucketFromStrings(method string, path string) *PathBucket {
	pathPieces := pathToPieces(path)
	variantPieces := make([]bool, len(pathPieces))
	for idx, pathPiece := range pathPieces {
		variantPieces[idx] = pathPiece == "*"
	}
	bucket := PathBucket{Results{}, method, pathPieces, variantPieces, make(map[string]uint32)}
	//fmt.Printf("Created new Path bucket: [URL: %s] => [Bucket: %s]\n", pathPieces, bucket.String())
	return &bucket
}

// NewPathBucketFromResult creates a new bucket from the path in the
// given Result, which becomes the first member; we assume that every
// part of the path consisting only of digits is one that may vary.
func NewPathBucketFromResult(pathPieces []string, result *Result) *PathBucket {
	results := Results{result}
	variantPieces := make([]bool, len(pathPieces))
	for idx, pathPiece := range pathPieces {
		variantPieces[idx] = digitsPiece.MatchString(pathPiece)
	}
	bucket := PathBucket{results, result.Method, pathPieces, variantPieces, make(map[string]uint32)}
	bucket.Urls["/"+strings.Join(pathPieces, "/")] = 1
	//fmt.Printf("Created new Path bucket: [URL: %s] => [Bucket: %s]\n", pathPieces, bucket.String())
	return &bucket
}

func pathToPieces(path string) []string {
	withoutQuery := trimQuery.ReplaceAllString(path, "")
	normalized := strings.Trim(withoutQuery, "/")
	return strings.Split(normalized, "/")
}

func (b *PathBucket) AddResult(result *Result) {
	b.Results = append(b.Results, result)
	// there's a race condition here when multiple goroutines are
	// adding a result with the same url to the bucket and the url
	// hasn't been seen before; result is off-by-one error in the urls
	// mapped to the bucket, which isn't a big deal right now (IMO)
	if val, ok := b.Urls[result.Path]; ok {
		b.Urls[result.Path] = atomic.AddUint32(&val, 1)
	} else {
		b.Urls[result.Path] = 1
	}
}

func (b *PathBucket) Match(checkPieces []string, result *Result) bool {
	if b.method != result.Method {
		return false
	}
	if len(checkPieces) != len(b.pieces) {
		return false
	}
	for idx, toCheck := range checkPieces {
		if !b.variantPieces[idx] && toCheck != b.pieces[idx] {
			return false
		}
	}
	return true
}

// String represents the method and path of this bucket as a string
// which should be parseable by NewPathBucketFromStrings
func (b *PathBucket) String() string {
	toDisplay := make([]string, len(b.pieces))
	for idx, piece := range b.pieces {
		if b.variantPieces[idx] {
			toDisplay[idx] = "*"
		} else {
			toDisplay[idx] = piece
		}
	}
	return fmt.Sprintf("%s /%s", b.method, strings.Join(toDisplay, "/"))
}
