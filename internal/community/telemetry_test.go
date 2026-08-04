package community

import (
	"testing"
)

func TestOTLPSignalURLAppendsProtocolPath(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		base   string
		signal string
		want   string
	}{
		{
			name:   "collector root",
			base:   "http://collector:4318",
			signal: "traces",
			want:   "http://collector:4318/v1/traces",
		},
		{
			name:   "collector base path",
			base:   "https://collector.example/otlp/",
			signal: "metrics",
			want:   "https://collector.example/otlp/v1/metrics",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := otlpSignalURL(test.base, test.signal)
			if err != nil {
				t.Fatalf("otlpSignalURL: %v", err)
			}
			if got != test.want {
				t.Fatalf("URL = %q, want %q", got, test.want)
			}
		})
	}
}

func TestOTLPSignalURLRejectsInvalidBase(t *testing.T) {
	t.Parallel()
	if _, err := otlpSignalURL("collector:4318", "traces"); err == nil {
		t.Fatal("expected invalid endpoint error")
	}
}
