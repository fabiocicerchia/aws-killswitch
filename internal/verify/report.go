package verify

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fabiocicerchia/aws-killswitch/internal/model"
)

// Text renders the report as something that can be pasted into an issue.
//
// The redaction is structural, not a filter: nothing in the Report type holds
// an ARN, an account id or a resource name in the first place, so there is
// nothing here that could accidentally print one. That is deliberate — a
// formatter that had access to the identifiers and chose not to print them
// would be one edit away from leaking an account inventory into a public
// tracker.
func (rep *Report) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "aws-killswitch verification run\n")
	fmt.Fprintf(&b, "started %s, took %s\n\n",
		rep.StartedAt.Format(time.RFC3339), rep.Duration.Round(time.Second))

	fmt.Fprintf(&b, "%-24s %9s %8s %8s\n", "kind", "found", "planned", "refused")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 52))
	for _, k := range sortedKinds(rep.Discovered, rep.Planned, rep.Refused) {
		fmt.Fprintf(&b, "%-24s %9d %8d %8d\n", k, rep.Discovered[k], rep.Planned[k], rep.Refused[k])
	}

	fmt.Fprintf(&b, "\nfire     %d changed, %d failed, %d verified by a second read\n",
		rep.FireChanged, rep.FireFailed, rep.FireVerified)
	fmt.Fprintf(&b, "restore  %d restored, %d failed\n", rep.RestoreOK, rep.RestoreFailed)

	if len(rep.Unexercised) > 0 {
		names := make([]string, 0, len(rep.Unexercised))
		for _, k := range rep.Unexercised {
			names = append(names, string(k))
		}
		fmt.Fprintf(&b, "\nNOT EXERCISED — the account held none of these, so this run says\n"+
			"nothing about them:\n  %s\n", strings.Join(names, ", "))
	}

	if len(rep.Findings) == 0 {
		fmt.Fprintf(&b, "\nNo divergence between what was planned and what the account read back.\n")
	} else {
		fmt.Fprintf(&b, "\n%d DIVERGENCE(S) — each of these is its own bug to file:\n", len(rep.Findings))
		for _, f := range rep.Findings {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
	}

	if len(rep.Errors) > 0 {
		fmt.Fprintf(&b, "\nErrors during the run (a per-service denial degrades that source\n"+
			"rather than failing the run, so some of these are expected):\n")
		for _, e := range rep.Errors {
			fmt.Fprintf(&b, "  - %s\n", e)
		}
	}
	return b.String()
}

// OK is whether the run is evidence the tool works: every action landed, every
// resource came back, and nothing diverged. A run with unexercised kinds is
// still OK — it is just narrower, and Text() says which.
func (rep *Report) OK() bool {
	return len(rep.Findings) == 0 && rep.FireFailed == 0 && rep.RestoreFailed == 0 &&
		rep.FireVerified == rep.FireChanged
}

func sortedKinds(maps ...map[model.Kind]int) []model.Kind {
	seen := map[model.Kind]bool{}
	for _, m := range maps {
		for k := range m {
			seen[k] = true
		}
	}
	// Every supported kind is listed even at zero: a row of zeroes is the
	// evidence that a kind was not covered, and omitting it hides that.
	for _, k := range AllKinds {
		seen[k] = true
	}
	out := make([]model.Kind, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
