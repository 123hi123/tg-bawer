package bot

import "testing"

func TestIsGoogleRateLimitError(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{"code429", `API error: {"error":{"code":429,"message":"quota"}}`, true},
		{"resourceExhausted", `API error: {"error":{"status":"RESOURCE_EXHAUSTED"}}`, true},
		{"tooManyRequests", `too many requests`, true},
		{"other", `API error: {"error":{"code":500}}`, false},
	}

	for _, tc := range cases {
		got := isGoogleRateLimitErrorString(tc.msg)
		if got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}
