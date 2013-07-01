package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"log"
	"net"
	"flag"
	"strings"
	"io"
	"io/ioutil"
	"os/exec"
	"path"
	"bufio"
	"reflect"
	"crypto/rand"
	"encoding/hex"
	"runtime"
)

func main() {
	flag.Parse()
	var (
		cmd string
		args []string
	)
	if flag.NArg() < 1 {
		fmt.Printf("Usage: mk CMD [ARGS...]\n")
		os.Exit(1)
	}
	cmd = flag.Arg(0)
	if flag.NArg() > 1 {
		args = flag.Args()[1:]
	}

	c, err  := newRootContainer(".")
	if err != nil {
		log.Fatal(err)
	}
	e, err := NewEngine(c.Path(".docker/engine"))
	if err != nil {
		log.Fatal(err)
	}
	defer e.Cleanup()
	ready := make(chan bool)
	go func() {
		if err := e.ListenAndServe(ready); err != nil {
			log.Fatal(err)
		}
	}()
	<-ready
	s, err := net.Dial("unix", e.Path("ctl"))
	if err != nil {
		log.Fatal(err)
	}
	commands := []string{
		"in " + cmd,
		"start " + strings.Join(args, "\x00"),
		"wait",
		"die",
	}
	if _, err := io.Copy(s, strings.NewReader(strings.Join(commands, "\n"))); err != nil {
		log.Fatal(err)
	}
	resp, err := ioutil.ReadAll(s)
	if err != nil {
		log.Fatal(err)
	}
	if len(resp) != 0 {
		log.Fatal("Engine error: " + string(resp))
	}
}


func newRootContainer(root string) (*Container, error) {
	abspath, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	c := &Container{
		Root: abspath,
	}
	// If it already exists, don't touch it
	if st, err := os.Stat(c.Path(".docker")); err == nil && st.IsDir() {
		return c, nil
	}
	if err := os.MkdirAll(c.Path(".docker"), 0700); err != nil {
		return nil, err
	}
	// ROOT/.docker didn't exist: set it up
	defer func() { if err != nil { os.RemoveAll(c.Path(".docker")) } }()
	// Generate an engine ID
	if err := writeFile(c.Path(".docker/engine/id"), GenerateID() + "\n"); err != nil {
		return nil, err
	}
	// Setup .docker/bin/docker
	if err := os.MkdirAll(c.Path(".docker/bin"), 0700); err != nil {
		return nil, err
	}
	// FIXME: create hardlink if possible
	if err := exec.Command("cp", SelfPath(), c.Path(".docker/bin/docker")).Run(); err != nil {
		return nil, err
	}
	// Setup .docker/bin/*
	for _, cmd := range []string {
		"exec",
		"start",
		"stop",
		"commit",
	} {
		if err := os.Symlink("docker", c.Path(".docker/bin", cmd)); err != nil {
			return nil, err
		}
	}
	// Setup .docker/run/main
	if err := writeFile(c.Path(".docker/run/main/cmd"), "docker\x00--engine"); err != nil {
		return nil, err
	}
	return c, nil
}




// Container

type Container struct {
	Id   string
	Root string
}

func (c *Container) Path(p ...string) string {
	return path.Join(append([]string{c.Root}, p...)...)
}


// Engine


type Engine struct {
	Root string
}

func NewEngine(root string) (*Engine, error) {
	abspath, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Engine{
		Root: abspath,
	}, nil
}

func (eng *Engine) Cleanup() {
	Debugf("Cleaning up engine")
	os.Remove(eng.Path("ctl"))
}

func (eng *Engine) ListenAndServe(ready chan bool) (err error) {
	defer close(ready)
	l, err := net.Listen("unix", eng.Path("ctl"))
	if err != nil {
		if c, dialErr := net.Dial("unix", eng.Path("ctl")); dialErr != nil {
			fmt.Printf("Cleaning up leftover unix socket\n")
			os.Remove(eng.Path("ctl"))
			l, err = net.Listen("unix", eng.Path("ctl"))
			if err != nil {
				return err
			}
		} else {
			c.Close()
			return err
		}
	}
	Debugf("Setting up signals")
	signals := make(chan os.Signal, 128)
	signal.Notify(signals)
	go func() {
		for sig := range signals {
			fmt.Printf("Caught %s. Closing socket\n", sig)
			l.Close()
		}
	}()

	if ready != nil {
		Debugf("Synchronizing")
		ready <- true
	}
	// FIXME: do we need to remove the socket?
	for {
		Debugf("Listening on %s\n", eng.Path("ctl"))
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		Debugf("Received connection: %s", conn)
		go eng.Serve(conn)
	}
}

func (eng *Engine) Serve(conn net.Conn) (err error) {
	defer func() {
		if err != nil {
			fmt.Fprintf(conn, "%s\n", err)
		}
		conn.Close()
	}()
	lines := bufio.NewReader(conn)
	chain := eng.Chain()
	for {
		Debugf("Reading command...")
		line, err := lines.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		Debugf("Processing command: %s", line)
		if err := chain.Cmd(line); err != nil {
			return err
		}
	}
	return nil
}

func (eng *Engine) Path(p ...string) string {
	return path.Join(append([]string{eng.Root}, p...)...)
}

func (eng *Engine) Chain() *Chain {
	return &Chain{
		engine: eng,
	}
}

func (eng *Engine) Get(name string) (*Container, error) {
	// FIXME: index containers by name, with nested names etc.
	cRoot := eng.Path("/containers", name)
	if st, err := os.Stat(cRoot); err != nil {
		return nil, err
	} else if !st.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", name)
	}
	return &Container{
		Id: name,
		Root: cRoot,
	}, nil
}


func (chain *Chain) Cmd(input string) error {
	cmd, err := ParseCommand(input)
	if err != nil {
		return err
	}
	// FIXME: insert pre-hooks here
	// FIXME: insert default commands here
	method, exists := chain.getMethod(cmd.Op)
	if !exists {
		return fmt.Errorf("No such command: %s", cmd.Op)
	}
	ret := method.Func.CallSlice([]reflect.Value{
		reflect.ValueOf(chain),
		reflect.ValueOf(cmd.Args),
	})[0].Interface()
	if ret == nil {
		return nil
	}
	return ret.(error)
	// FIXME: insert post-hooks here
}


// Command

type Command struct {
	Op	string
	Args	[]string
}

func ParseCommand(input string) (*Command, error) {
	parts := strings.SplitN(input, " ", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%s: invalid format", input)
	}
	return &Command{
		Op: parts[0],
		Args: strings.Split(parts[1], "\x00"),
	}, nil
}

// Chain

type Chain struct {
	context	*Container
	engine	*Engine
}

func (chain *Chain) getMethod(name string) (reflect.Method, bool) {
	methodName := "Cmd" + strings.ToUpper(name[:1]) + strings.ToLower(name[1:])
	return reflect.TypeOf(chain).MethodByName(methodName)
}

func (chain *Chain) CmdIn(args ...string) (err error) {
	chain.context, err = chain.engine.Get(args[0])
	return err
}

func (chain *Chain) CmdStart(args ...string) (err error) {
	if chain.context == nil {
		return fmt.Errorf("No context set")
	}
	// Iterate on commands
	// For each command, call CmdExec
	// Check if already running
	return fmt.Errorf("No yet implemented") // FIXME
}


// Utils

// Figure out the absolute path of our own binary
func SelfPath() string {
	path, err := exec.LookPath(os.Args[0])
	if err != nil {
		panic(err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		panic(err)
	}
	return path
}

func GenerateID() string {
	id := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, id)
	if err != nil {
		panic(err) // This shouldn't happen
	}
	return hex.EncodeToString(id)
}

// Write `content` to the file at path `dst`, creating it if necessary,
// as well as any missing directories.
// The file is truncated if it already exists.
func writeFile(dst, content string) error {
	// Create subdirectories if necessary
	if err := os.MkdirAll(path.Dir(dst), 0700); err != nil && !os.IsExist(err) {
		return err
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0700)
	if err != nil {
		return err
	}
	// Write content (truncate if it exists)
	if _, err := io.Copy(f, strings.NewReader(content)); err != nil {
		return err
	}
	return nil
}


// Debug function, if the debug flag is set, then display. Do nothing otherwise
// If Docker is in damon mode, also send the debug info on the socket
func Debugf(format string, a ...interface{}) {
	if os.Getenv("DEBUG") != "" {

		// Retrieve the stack infos
		_, file, line, ok := runtime.Caller(1)
		if !ok {
			file = "<unknown>"
			line = -1
		} else {
			file = file[strings.LastIndex(file, "/")+1:]
		}

		fmt.Fprintf(os.Stderr, fmt.Sprintf("[debug] %s:%d %s\n", file, line, format), a...)
	}
}
