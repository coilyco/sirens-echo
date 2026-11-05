package community

import "testing"

func TestToolArgNamesListsNamesNeverValues(t *testing.T) {
	got := toolArgNames(map[string]any{"limit": 0.0, "item": "a member's words", "currency": "Credit"})

	if got != "currency,item,limit" {
		t.Fatalf("toolArgNames = %q, want the sorted names only", got)
	}
	if toolArgNames(nil) != "" {
		t.Errorf("toolArgNames(nil) = %q, want empty", toolArgNames(nil))
	}
}

func TestToolLimitArgReportsTheNumberAModelSent(t *testing.T) {
	cases := map[string]struct {
		arguments map[string]any
		want      any
	}{
		"zero":     {map[string]any{"limit": 0.0}, 0.0},
		"fifty":    {map[string]any{"limit": 50.0}, 50.0},
		"absent":   {map[string]any{"item": "x"}, nil},
		"a string": {map[string]any{"limit": "all"}, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := toolLimitArg(tc.arguments); got != tc.want {
				t.Fatalf("toolLimitArg = %v, want %v", got, tc.want)
			}
		})
	}
}
