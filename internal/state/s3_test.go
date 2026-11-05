package state

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

// fakeS3 is an in-memory bucket speaking enough of the S3 REST protocol for
// the real SDK client: PutObject, GetObject and ListObjectsV2. It pages one key
// at a time so the listing has to follow continuation tokens.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	sse     map[string]string
}

type listResult struct {
	XMLName               xml.Name `xml:"ListBucketResult"`
	KeyCount              int      `xml:"KeyCount"`
	IsTruncated           bool     `xml:"IsTruncated"`
	NextContinuationToken string   `xml:"NextContinuationToken,omitempty"`
	Contents              []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if bucket != "restore-records" {
		http.Error(w, "<Error><Code>NoSuchBucket</Code></Error>", http.StatusNotFound)
		return
	}
	switch {
	case r.Method == http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		f.objects[key] = b
		f.sse[key] = r.Header.Get("X-Amz-Server-Side-Encryption")
	case r.Method == http.MethodGet && key != "":
		b, ok := f.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
			return
		}
		_, _ = w.Write(b)
	case r.Method == http.MethodGet:
		var keys []string
		for k := range f.objects {
			if strings.HasPrefix(k, r.URL.Query().Get("prefix")) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		start, _ := strconv.Atoi(r.URL.Query().Get("continuation-token"))
		var res listResult
		if start < len(keys) {
			res.Contents = append(res.Contents, struct {
				Key string `xml:"Key"`
			}{keys[start]})
			res.KeyCount = 1
		}
		if start+1 < len(keys) {
			res.IsTruncated, res.NextContinuationToken = true, strconv.Itoa(start+1)
		}
		w.Header().Set("Content-Type", "application/xml")
		_ = xml.NewEncoder(w).Encode(res)
	default:
		http.Error(w, "unsupported", http.StatusMethodNotAllowed)
	}
}

func newFakeS3Client(t *testing.T) (*fakeS3, *s3.Client) {
	t.Helper()
	f := &fakeS3{objects: map[string][]byte{}, sse: map[string]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c := s3.New(s3.Options{
		Region:       "eu-west-1",
		BaseEndpoint: aws.String(srv.URL),
		UsePathStyle: true,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test"}, nil
		}),
	})
	return f, c
}

// The durable record goes through the real SDK client over HTTP: Build wires
// S3 ahead of the local directory, PutVerified writes and reads it back, and
// the object lands under the prefix, encrypted, where a rebuilt laptop finds it.
func TestS3StoreRoundTripsThroughSDKClient(t *testing.T) {
	ctx := context.Background()
	f, c := newFakeS3Client(t)
	store, err := Build(c, "s3://restore-records/runs/", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := PutVerified(ctx, store, snap(2)); err != nil {
		t.Fatal(err)
	}
	if got := f.sse["runs/p1.json"]; got != "AES256" {
		t.Errorf("server-side encryption header = %q, want AES256", got)
	}

	durable := S3{Client: c, Bucket: "restore-records", Prefix: "runs"}
	got, err := durable.Get(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(got.Entries))
	}
	if v, ok := model.PriorInt(got.Entries[0].Prior, "desired_capacity"); !ok || v != 3 {
		t.Errorf("desired_capacity: got %d ok=%v, want 3", v, ok)
	}

	if _, err := durable.Get(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing object: got %v, want ErrNotFound", err)
	}
}

// List has to follow pagination, skip a corrupt record instead of failing,
// ignore keys that are not snapshots, and return newest first.
func TestS3StoreListPagesAndSkipsCorrupt(t *testing.T) {
	ctx := context.Background()
	f, c := newFakeS3Client(t)
	durable := S3{Client: c, Bucket: "restore-records", Prefix: "runs"}
	older, newer := snap(1), snap(1)
	older.PlanID, newer.PlanID = "old", "new"
	newer.CreatedAt = older.CreatedAt.Add(time.Hour)
	if err := durable.Put(ctx, older); err != nil {
		t.Fatal(err)
	}
	if err := durable.Put(ctx, newer); err != nil {
		t.Fatal(err)
	}
	f.objects["runs/broken.json"] = []byte("{not json")
	f.objects["runs/README.txt"] = []byte("not a snapshot")

	list, err := durable.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].PlanID != "new" || list[1].PlanID != "old" {
		ids := make([]string, len(list))
		for i, s := range list {
			ids[i] = s.PlanID
		}
		t.Errorf("got %v, want [new old]", ids)
	}
}
