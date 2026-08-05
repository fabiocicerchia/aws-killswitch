package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

// S3 is the durable home for snapshots.
//
// S3 is in the never-touch set, which is not a coincidence: the kill switch
// must not be able to destroy its own restore record. Versioning on the bucket
// is worth turning on for the same reason.
type S3 struct {
	Client *s3.Client
	Bucket string
	Prefix string
}

// ParseURI accepts s3://bucket/prefix.
func ParseURI(uri string) (bucket, prefix string, ok bool) {
	if !strings.HasPrefix(uri, "s3://") {
		return "", "", false
	}
	rest := strings.TrimPrefix(uri, "s3://")
	bucket, prefix, _ = strings.Cut(rest, "/")
	return bucket, strings.Trim(prefix, "/"), bucket != ""
}

func (s S3) key(planID string) string {
	if s.Prefix == "" {
		return planID + ".json"
	}
	return s.Prefix + "/" + planID + ".json"
}

func (s S3) Put(ctx context.Context, snap model.Snapshot) error {
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	_, err = s.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.Bucket), Key: aws.String(s.key(snap.PlanID)),
		Body: bytes.NewReader(b), ContentType: aws.String("application/json"),
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	})
	return err
}

func (s S3) Get(ctx context.Context, planID string) (model.Snapshot, error) {
	out, err := s.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket), Key: aws.String(s.key(planID)),
	})
	if err != nil {
		return model.Snapshot{}, fmt.Errorf("%w: %s", ErrNotFound, planID)
	}
	defer func() { _ = out.Body.Close() }()
	b, err := io.ReadAll(out.Body)
	if err != nil {
		return model.Snapshot{}, err
	}
	var snap model.Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return model.Snapshot{}, fmt.Errorf("snapshot %s is corrupt: %w", planID, err)
	}
	return snap, nil
}

func (s S3) List(ctx context.Context) ([]model.Snapshot, error) {
	var out []model.Snapshot
	pager := s3.NewListObjectsV2Paginator(s.Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.Bucket), Prefix: aws.String(s.Prefix),
	})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if !strings.HasSuffix(key, ".json") {
				continue
			}
			id := strings.TrimSuffix(key[strings.LastIndex(key, "/")+1:], ".json")
			snap, err := s.Get(ctx, id)
			if err != nil {
				continue
			}
			out = append(out, snap)
		}
	}
	sort.Slice(out, func(i, j int) bool { return stamp(out[i]).After(stamp(out[j])) })
	return out, nil
}

func (s S3) Describe() string { return "s3://" + s.Bucket + "/" + s.Prefix }
