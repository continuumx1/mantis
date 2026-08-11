package render

import (
	"strings"
	"testing"
)

func TestMascot_ColorGating(t *testing.T) {
	plain := Mascot(ColorNone)
	if strings.Contains(plain, "\x1b[") {
		t.Errorf("plain mascot must not contain ANSI escape codes")
	}
	if !strings.Contains(plain, "@") {
		t.Errorf("mascot art appears to be missing")
	}

	c256 := Mascot(Color256)
	if !strings.HasPrefix(c256, "\x1b[38;5;154m") {
		t.Errorf("256-color mascot must start with the neon-green palette escape")
	}
	if !strings.HasSuffix(c256, "\x1b[0m") {
		t.Errorf("256-color mascot must reset the color at the end")
	}

	trueColor := Mascot(ColorTrue)
	if !strings.HasPrefix(trueColor, "\x1b[38;2;160;232;42m") {
		t.Errorf("truecolor mascot must start with the KNW neon-green truecolor escape")
	}
	if !strings.HasSuffix(trueColor, "\x1b[0m") {
		t.Errorf("truecolor mascot must reset the color at the end")
	}
}
