package fswatch

import "testing"

func TestFSEventsGiven(t *testing.T) {
	s := &fsevents{
		real: [][2]string{
			{"/private/tmp/sub", "/link-sub"},
			{"/private/tmp", "/link-tmp"},
			{"/", "/link-root"},
		},
	}
	cases := []struct {
		in, want string
	}{
		{"/private/tmp/sub/a.go", "/link-sub/a.go"},
		{"/private/tmp/b.go", "/link-tmp/b.go"},
		{"/foo/bar", "/link-root/foo/bar"},
		{"/", "/link-root"},
	}
	for _, tc := range cases {
		if got := s.given(tc.in); got != tc.want {
			t.Errorf("given(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
