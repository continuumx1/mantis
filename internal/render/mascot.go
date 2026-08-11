package render

import "fmt"

// KNW neon-green brand colour, used for terminal art (24-bit "truecolor").
const (
	brandR = 160
	brandG = 232
	brandB = 42
)

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

// Mascot returns the KNW mantis mascot as ASCII art. When color is true it is
// wrapped in a 24-bit ANSI escape so it renders in the KNW neon green; callers
// should pass true only when writing to a terminal that supports it, so piped
// output stays clean and free of escape codes.
func Mascot(color bool) string {
	if !color {
		return mantis
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", brandR, brandG, brandB, mantis)
}
