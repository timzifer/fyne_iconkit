package iconupdate

import (
	"regexp"
	"strings"
)

var pathData = regexp.MustCompile(`\sd="([^"]*)"`)

// normalize rewrites SVG path data that oksvg (and with it Fyne) cannot
// parse: arc flags written without separators ("a1 1 0 00-2 1" for
// "a1 1 0 0 0 -2 1"), which minifiers produce and SVG allows.
func normalize(svg []byte) []byte {
	return pathData.ReplaceAllFunc(svg, func(m []byte) []byte {
		sub := pathData.FindSubmatch(m)
		d := string(sub[1])
		fixed := expandArcFlags(d)
		if fixed == d {
			return m
		}
		return []byte(strings.Replace(string(m), d, fixed, 1))
	})
}

// expandArcFlags separates the flags of arc commands in path data.
func expandArcFlags(d string) string {
	if !strings.ContainsAny(d, "aA") {
		return d
	}
	var out strings.Builder
	i := 0
	for i < len(d) {
		c := d[i]
		if c != 'a' && c != 'A' {
			out.WriteByte(c)
			i++
			continue
		}
		out.WriteByte(c)
		i++
		// Arguments: rx ry rotation large-arc sweep x y, repeated.
		for arg := 0; ; arg++ {
			start := i
			for i < len(d) && (d[i] == ' ' || d[i] == ',' || d[i] == '\n' || d[i] == '\t' || d[i] == '\r') {
				i++
			}
			if i >= len(d) || !isNumberStart(d[i]) {
				out.WriteString(d[start:i])
				break
			}
			if arg > 0 {
				out.WriteByte(' ')
			}
			if pos := arg % 7; pos == 3 || pos == 4 {
				out.WriteByte(d[i]) // a flag is a single "0" or "1"
				i++
				continue
			}
			n := numberLen(d[i:])
			out.WriteString(d[i : i+n])
			i += n
		}
	}
	return out.String()
}

func isNumberStart(c byte) bool {
	return c == '-' || c == '+' || c == '.' || (c >= '0' && c <= '9')
}

// numberLen returns the length of the number at the start of s.
func numberLen(s string) int {
	i := 0
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		i++
	}
	dot := false
	for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || (s[i] == '.' && !dot)) {
		if s[i] == '.' {
			dot = true
		}
		i++
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '-' || s[j] == '+') {
			j++
		}
		if j < len(s) && s[j] >= '0' && s[j] <= '9' {
			i = j
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
		}
	}
	return i
}
