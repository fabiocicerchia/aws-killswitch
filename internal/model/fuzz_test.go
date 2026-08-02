package model

import (
	"encoding/json"
	"testing"
	"time"
)

// FuzzSnapshotDecode exercises the restore path's trust boundary: a snapshot is
// read back from S3 (or a local dir) and then drives real AWS mutations. The
// blob is only as trustworthy as the bucket, so decoding it and reading the
// prior state out of it must never panic — a crash mid-restore leaves the
// account half-down, which is the failure this tool exists to avoid.
func FuzzSnapshotDecode(f *testing.F) {
	f.Add(`{"plan_id":"p","entries":[{"kind":"rds-instance","id":"db","result":"changed","prior":{"desired_count":3}}]}`)
	f.Add(`{"entries":[{"prior":{"desired_count":1e309,"name":null}}]}`)
	f.Add(`{"created_at":"0000-01-01T00:00:00Z","fired_at":null,"entries":null}`)
	f.Add(`{}`)

	f.Fuzz(func(t *testing.T, blob string) {
		var s Snapshot
		if err := json.Unmarshal([]byte(blob), &s); err != nil {
			return // malformed JSON is rejected upstream; only valid decodes matter
		}
		for _, e := range s.Changed() {
			e.RestoreDeadline(time.Unix(0, 0))
			PriorInt(e.Prior, "desired_count")
			PriorString(e.Prior, "name")
			IsNeverTouch(e.ARN)
		}
	})
}

// FuzzIsNeverTouch guards the deny list that decides whether a resource may be
// acted on at all. It splits ARNs by hand, so a malformed ARN must still return
// an answer rather than panic — and "unrecoverable resource" is the one call it
// cannot get wrong.
func FuzzIsNeverTouch(f *testing.F) {
	f.Add("arn:aws:s3:::bucket")
	f.Add("arn::")
	f.Add("ebs-volume")
	f.Add("")

	f.Fuzz(func(t *testing.T, s string) {
		IsNeverTouch(s)
	})
}
