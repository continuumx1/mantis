package render

import "fmt"

// ColorMode selects how much colour a terminal can render.
type ColorMode int

const (
	// ColorNone emits no ANSI colour (piped output, NO_COLOR, or non-terminal).
	ColorNone ColorMode = iota
	// Color256 uses the 256-colour palette, supported almost everywhere
	// including macOS Terminal.app.
	Color256
	// ColorTrue uses 24-bit "truecolor" for the exact brand colour; supported
	// by terminals that advertise COLORTERM=truecolor (e.g. iTerm2).
	ColorTrue
)

// KNW neon-green brand colour, used for terminal art (24-bit "truecolor").
const (
	brandR = 160
	brandG = 232
	brandB = 42
)

// brand256 is the closest 256-palette neon green (xterm 154, ~#afff00), used
// when the terminal cannot render truecolor.
const brand256 = 154

// mantis is the KNW mascot as ASCII art.
const mantis = `
             ...                              ....
                   .                     ..
                     ..               .
                       ..          ..
                        ..        .
                         .      .
                    ,.    .    .    *..,
                 .@@@@@@.%.%%%.%%.@@@@@@@.
                .@@@@..@@%%%%%%.@@@@@..@@@.
                .@@@....&%%%%%%*@@@@....@@.
                .@@@@ @@.%%%%%% @@@@...@@.
                  ..@@.#%%%%%%%%.@@@@@.,
                      .%%%%%%%%%%(.%%.
                       *.%%#%%%..%%%%%.
                          ...   (#%%%%%.,
                                  %%%%%%%.
                  .#%%.           .,%.%%.%.
                 .%%%%%%** .%%%.  .(..%%%.%..
                ,%%%%.%%%%.%%%%%%%.((.%%%.%%(,/.
                .%%%%..%%%.%%%.%%%%%..%%%.%%.#%%%%.,
                .%%%.,  .%.%%%.,/%%%%*%%%.%%%..%%%%%%%.
                 .%%..    .%%#,. ,#%%%%%%.%%%.(.%%%%%%%%%..
                  .%%,     .%%.     .*%%...%%%.((.%%%%%%%%%%%.
                    .%.     .%.             .%/(((..%%%**#.%%%%%.
                             ,..    ..      ....((.((.#%.#%,%%%%%%%.
                                   ..  ... ..   .(..%%.((.#.%%%%%%%%%.
                                  .              .#.,.#(((.%.(..%%%%%%,.
                                 .               .         .%,..,
                                .                .          ..
                               .                ..           *.
                                                               .`

// Mascot returns the KNW mantis mascot as ASCII art, coloured according to mode.
// ColorNone returns plain art with no escape codes, so piped output stays clean;
// Color256 renders the neon green on any 256-colour terminal; ColorTrue renders
// the exact brand green on truecolor terminals.
func Mascot(mode ColorMode) string {
	var prefix string
	switch mode {
	case ColorTrue:
		prefix = fmt.Sprintf("\x1b[38;2;%d;%d;%dm", brandR, brandG, brandB)
	case Color256:
		prefix = fmt.Sprintf("\x1b[38;5;%dm", brand256)
	default:
		return mantis
	}
	return prefix + mantis + "\x1b[0m"
}
