package main

import (
	"fmt"
	"os"
	"path/filepath"
	"log"
)

main() {
	c := newRootContainer(".")
	e := NewEngine(c.Path(".docker/engine"))
	go e.ListenAndServe()
	s, err := net.Connect("unix", e.socketPath)
	if err != nil {
		log.Fatal(err)
	}
	commands := []string{
		"in " + flag.Arg(0),
		"start " + "\0".Join(flag.Args()[1:]),
		"wait",
		"die",
	}
	if _, err := io.Copy(s, strings.Join(commands)); err != nil {
		log.Fatal(err)
	}
	resp, err := ioutil.ReadAll(s)
	if err != nil {
		log.Fatal(err)
	}
	if resp != "" {
		log.Fatal("Engine error: " + resp)
	}
}


func newRootContainer(root string) (*Container, error) {
	c := &Container{
		Root: filepath.Abs(root),
	}
	// If it already exists, don't touch it
	err := os.MkdirAll(c.Path(".docker"), 0700)
	if os.IsExist(err) {
		// FIXME: optionally upgrade it
		return c, nil
	} else if err != nil {
		return nil, err
	}
	// ROOT/.docker didn't exist: set it up
	defer func() { if err != nil { os.RemoveAll(c.Path(".docker")) } }
	// Generate an engine ID
	if err := writeFile(c.Path(".docker/engine/id"), GenerateID()); err != nil {
		return nil, err
	}
	// Setup .docker/bin/docker
	if err := os.MkdirAll(c.Path(".docker/bin"), 0700); err != nil {
		return nil, err
	}
	// FIXME: create hardlink if possible
	if err := exec.Command("cp", []string{SelfPath(), c.Path(".docker/bin/docker")).Run(); err != nil {
		return nil, err
	}
	// Setup .docker/bin/*
	for cmd in range []string {
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
	if err := writeFile(c.Path(".docker/run/main/cmd", "docker\0--engine"); err != nil {
		return nil, err
	}
}




// Container

type Container struct {
	Root string
}

func (c *Container) Path(p ...string) string {
	return path.Join(c.Root, p...)
}

func (c *Container) Chain(commands ...*Command) {
	context := c
}


// Engine


type Engine struct {
	root string
}

func NewEngine(root string) *Engine {
	return &Engine{
		root: filepath.Abspath(root)
	}
}


func (eng *Engine) ListenAndServe() error {
	l, err := net.Listen("unix", eng.Path("ctl"))
	if err != nil {
		return err
	}
	// FIXME: do we need to remove the socket?
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go eng.Serve(conn)
	}
}

func (eng *Engine) Serve(conn net.Conn) (err error) {
	defer func() {
		if err != nil {
			fmt.Fprintf(conn, "%s\n", err)
		}
		conn.Close()
	}
	var context string // name of the current container
	lines := bufio.NewReader(conn)
	for {
		line, err := lines.ReadString("\n")
		if err != nil && err != io.EOF {
			return err
		}
		if err := eng.Cmd(line, context); err != nil {
			return err
		}
	}
	return nil
}

func (eng *Engine) Path(p ...string) string {
	return path.Join(eng.Root, p...)
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
		return nil, error
	} else if !st.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", name)
	}
	return &Container{
		id: name,
		root: cRoot,
	}, nil
}


func (chain *Chain) Cmd(input string) error {
	cmd, err := chain.parse(input)
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

func (chain *chain) parse(input string) (*Command, error) {
	parts := strings.SplitN(input, " ", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%s: invalid format", input)
	}
	return &Command{
		Op: parts[0],
		Args: strings.Split(parts[1], "\0"),
	}
}

func (chain *Chain) getMethod(name string) (reflect.Method, bool) {
	methodName := "Cmd" + strings.ToUpper(name[:1]) + strings.ToLower(name[1:])
	return reflect.TypeOf(chain).MethodByName(methodName)
}

func (chain *Chain) CmdIn(args ...string) (err error) {
	chain.context, err = chain.engine.Get(arg[0])
	return err
}

func (chain *Chain) CmdStart(args ...string) (err error) {
	if chain.context == nil {
		return fmt.Errorf("No context set")
	}
	// Iterate on commands
	// For each command, call CmdExec
	// Check if already running
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
}



