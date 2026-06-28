package relate

import (
	"fmt"
	"testing"
)

func TestGateGolden(t *testing.T) {
	join := Join{SrcKey: "src_id", DstKey: "dst_id", Card: CardOneToMany, Verb: "relates"}
	joinKey := "src_id -> dst_id"

	cases := []struct {
		name      string
		realJoin  bool
		different bool
		noBridge  bool
		emergent  bool
		want      Verdict
		wantKey   string
		wantRatio float64
	}{
		{
			name:      "door without real join",
			realJoin:  false,
			want:      VerdictDoor,
			wantKey:   "",
			wantRatio: 0.18,
		},
		{
			name:      "standing elucidate",
			realJoin:  true,
			different: true,
			noBridge:  true,
			want:      VerdictStanding,
			wantKey:   joinKey,
			wantRatio: 0.45,
		},
		{
			name:      "standing emergent",
			realJoin:  true,
			emergent:  true,
			want:      VerdictStanding,
			wantKey:   joinKey,
			wantRatio: 0.45,
		},
		{
			name:      "peek real join only",
			realJoin:  true,
			want:      VerdictPeek,
			wantKey:   joinKey,
			wantRatio: 0.18,
		},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s=%s", tc.name, tc.want), func(t *testing.T) {
			got := Gate(join, tc.realJoin, tc.different, tc.noBridge, tc.emergent)
			if got != tc.want {
				t.Fatalf("Gate() = %s; want %s", got, tc.want)
			}
			if gotKey := join.JoinKeyFor(got); gotKey != tc.wantKey {
				t.Fatalf("JoinKeyFor(%s) = %q; want %q", got, gotKey, tc.wantKey)
			}
			if gotRatio := got.Ratio(); gotRatio != tc.wantRatio {
				t.Fatalf("Ratio(%s) = %v; want %v", got, gotRatio, tc.wantRatio)
			}
		})
	}
}
