package spec

import (
	"testing"

	"../config"
)

func TestSum(t *testing.T) {
	for i := 0; i < 100; i++ {
		if to := ElectTimeout(); to < int64(config.C.ElectTimeoutMin) || to > int64(config.C.ElectTimeoutMax) {
			t.Fatalf(
				"ElectTimeout() returned value outside accepted range got: %d, want: [%d,%d].",
				to,
				config.C.ElectTimeoutMin,
				config.C.ElectTimeoutMax,
			)
		}
	}
}
