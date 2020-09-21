package pointer

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDerefBoolOr(t *testing.T) {
	b := false

	type args struct {
		b    *bool
		dflt bool
	}
	cases := map[string]struct {
		reason string
		args   args
		want   bool
	}{
		"NilValue": {
			reason: "Passing nil value should return the supplied default",
			args:   args{b: nil, dflt: true},
			want:   true,
		},
		"NonNilValue": {
			reason: "Passing non-nil value should return the supplied value",
			args:   args{b: &b, dflt: true},
			want:   false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := DerefBoolOr(tc.args.b, tc.args.dflt)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("\n%s\nDerefBoolOr(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestInt64OrNil(t *testing.T) {
	cases := map[string]struct {
		reason string
		i      int
		want   *int64
	}{
		"ZeroValue": {
			reason: "Passing zero should return nil",
			i:      0,
			want:   nil,
		},
		"NonZeroValue": {
			reason: "Passing a non-zero should return a non-nil *int64",
			i:      42,
			want: func() *int64 {
				var i int64 = 42
				return &i
			}(),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Int64OrNil(tc.i)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("\n%s\nInt64OrNil(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestBool(t *testing.T) {
	cases := map[string]struct {
		reason string
		b      bool
		want   *bool
	}{
		"True": {
			reason: "Passing true should return *true",
			b:      true,
			want: func() *bool {
				t := true
				return &t
			}(),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Bool(tc.b)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("\n%s\nBool(...): -want, +got: \n%s\n", tc.reason, diff)
			}
		})
	}
}
