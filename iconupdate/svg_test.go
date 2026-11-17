package iconupdate

import "testing"

func TestExpandArcFlags(t *testing.T) {
	for in, want := range map[string]string{
		"M15.914 4a1.5 1.5 0 00-2.474-1.561l-9 9":         "M15.914 4a1.5 1.5 0 0 0 -2.474 -1.561l-9 9",
		"M19 6.708a8 8 0 100 10.584":                      "M19 6.708a8 8 0 1 0 0 10.584",
		"M2 21a5 5 0 012.911-4.544C7.6 15.2 8.3 15 11 13": "M2 21a5 5 0 0 1 2.911 -4.544C7.6 15.2 8.3 15 11 13",
		"M5 12a2 2 0 0 1 2 2":                             "M5 12a2 2 0 0 1 2 2",
		"M1 1a1 1 0 0 1 2 2 1 1 0 1 0 3 3":                "M1 1a1 1 0 0 1 2 2 1 1 0 1 0 3 3",
		"M1 1a1 1 0 0112 2":                               "M1 1a1 1 0 0 1 12 2",
		"M1 1A.5.5 0 01.5 1e-2z":                          "M1 1A.5 .5 0 0 1 .5 1e-2z",
		"M5 12l-2 0":                                      "M5 12l-2 0",
	} {
		if got := expandArcFlags(in); got != want {
			t.Errorf("expandArcFlags(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	in := `<svg><path d="M19 6.708a8 8 0 100 10.584" /><path d="M5 12l-2 0"/></svg>`
	want := `<svg><path d="M19 6.708a8 8 0 1 0 0 10.584" /><path d="M5 12l-2 0"/></svg>`
	if got := string(normalize([]byte(in))); got != want {
		t.Errorf("normalize = %s", got)
	}
}
