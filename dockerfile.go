package docker

import (
	"bufio"
	"strings"
	"io"
	"fmt"
)


func ParseDockerfile(input io.Reader) (chain [][]string, err error) {
	lines := bufio.NewReader(input)
	var eof bool
	for {
		if eof {
			break
		}
		var line string
		line, err = lines.ReadString('\n')
		if err == io.EOF {
			eof = true
			err = nil
		} else if err != nil {
			return
		}
		line = strings.Trim(line, "\n")
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		Log("build op '%s'\n", line)
		// FIXME: split in different number of parts depending on dockerfile command
		// (this is to respect backwards compatibility with the current dockerfile format)
		parts := strings.SplitN(line, " ", 2)
		parts[0] = strings.ToLower(parts[0])
		if parts[0] == "run" {
			if len(parts) < 2 {
				err = fmt.Errorf("RUN build operation requires at least one argument")
				return
			}
			parts = []string{"exec", "/bin/sh", "-c", parts[1]}
		}
		chain = append(chain, parts)
	}
	return
}
