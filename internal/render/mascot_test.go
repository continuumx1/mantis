package render

import (
	"strings"
	"testing"
)

func TestMascot_ColorGating(t *testing.T) {
	plain := Mascot(false)
	if strings.Contains(plain, "\x1b[") {
		t.Errorf("plain mascot must not contain ANSI escape codes")
	}
	if !strings.Contains(plain, "@") {
		t.Errorf("mascot art appears to be missing")
	}

	colored := Mascot(true)
	if !strings.HasPrefix(colored, "\x1b[38;2;160;232;42m") {
		t.Errorf("colored mascot must start with the KNW neon-green truecolor escape")
	}
	if !strings.HasSuffix(colored, "\x1b[0m") {
		t.Errorf("colored mascot must reset the color at the end")
	}
}
