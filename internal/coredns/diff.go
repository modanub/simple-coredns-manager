package coredns

import (
	"fmt"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
)

func GenerateDiff(filename, original, modified string) string {
	edits := myers.ComputeEdits(span.URIFromPath(filename), original, modified)
	unified := gotextdiff.ToUnified(
		fmt.Sprintf("a/%s", filename),
		fmt.Sprintf("b/%s", filename),
		original,
		edits,
	)
	result := fmt.Sprint(unified)
	if result == "" {
		return ""
	}
	return result
}
