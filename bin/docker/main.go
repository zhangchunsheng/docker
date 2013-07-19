package main

import (
	"fmt"
	"os"
	"flag"
	"strings"
	"io"
	"os/exec"
	"bufio"
	"github.com/dotcloud/docker"
)


func main() {
//	os.Setenv("DEBUG", "1")
	fmt.Printf("Main %s\n", os.Args)
	flEngine := flag.Bool("e", false, "Engine mode")
	flag.Parse()
	if *flEngine {
		if err := engineMain(flag.Args()); err != nil {
			docker.Fatalf("Failed to execute engine command '%s': %s",
				strings.Join(flag.Args(), " "), err)
		}
		os.Exit(0)
	}
	if flag.NArg() < 1 {
		fmt.Printf("Usage: mk CMD [ARGS...]\n")
		os.Exit(1)
	}

	c, err  := docker.NewContainer("0", ".")
	if err != nil {
		docker.Fatalf("Failed to setup root container: %s", err)
	}
	// FIXME: NewEngine should initialize the root container instead of receiving
	// it as an argument.
	eng, err := docker.NewEngine(c) // Pass the root container to the engine
	if err != nil {
		docker.Fatalf("Failed to initialize engine: %s", err)
	}
	defer eng.Cleanup()
	ready := make(chan bool)
	go func() {
		if err := eng.ListenAndServe(ready); err != nil {
			docker.Fatal(err)
		}
	}()
	<-ready
	if err := eng.Ctl(
		[]string{"in", "0"},
		flag.Args(),
		[]string{"die"},
	); err != nil {
		docker.Fatalf("Error sending engine startup commands: %s", err)
	}
}

// This runs in a separate process in c0, chdired to the target container
// NOTE: we may not be chrooted, so don't assume / is the root of c0
// FIXME: do we need access to the root of c0?
func engineMain(args []string) error {
	self, err := docker.CurrentContainer() // This needs access to c0
	if err != nil {
		return err
	}
	eng := self.Engine()
	if args[0] == "import" {
		docker.Log("Importing from %s", args[1])
		/*
		// FIXME: pseudo-code
		src := args[1]
		var data io.Reader
		if src == "-" {
			data = os.Stdin
		} else {
			data = http.Get(src)
		}
		Untar(data, ".")
		*/
	} else if args[0] == "start" {
		commands, err := docker.LS(self.Path(".docker/run/exec"))
		if err != nil {
			return err
		}
		for _, cmd := range commands {
			go eng.Ctl([]string{"exec", cmd})
		}
		// Wait for all execs to return
	} else if args[0] == "exec" {
		cmd := exec.Command(args[1], args[2:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		// Execute a process into a container, using chroot
	} else if args[0] == "info" {
		fmt.Printf("Current container = %s\n", self.Root)
	} else if args[0] == "serve" {
		// Expose engine functionalities over the remote http api
	} else if args[0] == "echo" {
		fmt.Println(strings.Join(args[1:], " "))
	} else if args[0] == "build" {
		dockerfile, err := os.Open("./Dockerfile")
		if err != nil {
			return err
		}
		lines := bufio.NewReader(dockerfile)
		chain := [][]string{}
		var eof bool
		for {
			if eof {
				break
			}
			line, err := lines.ReadString('\n')
			if err == io.EOF {
				eof = true
			} else if err != nil {
				return err
			}
			line = strings.Trim(line, "\n")
			if len(line) == 0 || line[0] == '#' {
				continue
			}
			docker.Log("build op '%s'\n", line)
			// FIXME: split in different number of parts depending on dockerfile command
			// (this is to respect backwards compatibility with the current dockerfile format)
			parts := strings.SplitN(line, " ", 2)
			parts[0] = strings.ToLower(parts[0])
			if parts[0] == "run" {
				if len(parts) < 2 {
					return fmt.Errorf("RUN build operation requires at least one argument")
				}
				parts = []string{"exec", "/bin/sh", "-c", parts[1]}
			}
			chain = append(chain, parts)
		}
		if len(chain) == 0 {
			fmt.Printf("Empty Dockerfile. Nothing to do.\n")
			return nil
		}
		fmt.Printf("Parsed %d operations from Dockerfile. Sending to engine.\n", len(chain))
		if err := eng.Ctl(chain...); err != nil {
			return err
		}
		return nil
	} else if args[0] == "expose" {
		// Expose a TCP port
	} else if args[0] == "connect" {
		// Discover a TCP port to connect to
	} else if args[0] == "prompt" {
		// Prompt the user for a value
	} else if args[0] == "commit" {
		// Commit a new snapshot of this image
	} else {
		return fmt.Errorf("Unknown command: '%s'", args[0])
	}
	return nil
}

