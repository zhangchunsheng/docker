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
	os.Setenv("DEBUG", "1")
	Debugf("Main")
	flEngine := flag.Bool("e", false, "Engine mode")
	flag.Parse()
	writeFile("last-call", strings.Join(flag.Args(), " "))
	if *flEngine {
		if err := engineMain(flag.Args()); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}
	if flag.NArg() < 1 {
		fmt.Printf("Usage: mk CMD [ARGS...]\n")
		os.Exit(1)
	}

	c, err  := newRootContainer(".")
	if err != nil {
		log.Fatal(err)
	}
	eng, err := NewEngine(c) // Pass the root container to the engine
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Cleanup()
	ready := make(chan bool)
	go func() {
		if err := eng.ListenAndServe(ready); err != nil {
			log.Fatal(err)
		}
	}()
	<-ready
	if err := eng.Ctl(
		"in 0",
		strings.Join(flag.Args(), "\x00"),
		"wait",
		"die",
	); err != nil {
		log.Fatal(err)
	}
}

func CurrentContainer() (*Container, error) {
	root := os.Getenv("DOCKER_ROOT")
	if root == "" {
		root = "/"
	}
	Debugf("Loading current container, root=%s", root)
	return &Container{
		Root: root,
	}, nil
}

func (c *Container) Engine() (*Engine) {
	return &Engine{
		c0: c,
	}
}


// This runs in a separate process in c0, chdired to the target container
// NOTE: we may not be chrooted, so don't assume / is the root of c0
// FIXME: do we need access to the root of c0?
func engineMain(args []string) error {
	eng := NewReflector() // This needs access to c0
	if args[0] == "import" {
		// FIXME: pseudo-code
		src := args[1]
		var data io.Reader
		if src == "-" {
			data = os.Stdin
		} else {
			data = http.Get(src)
		}
		Untar(data, ".")
	} else if args[0] == "start" {
		for _, cmd := eng.List("/commands") {
			go eng.Ctl("exec " + cmd)
		}
		// Wait for all execs to return
	} else if args[0] == "exec" {
		// Execute a process into a container, using chroot
	} else if args[0] == "serve" {
		// Expose engine functionalities over the remote http api
	} else if args[0] == "build" {
		dockerfile := os.Open("./Dockerfile")
		for line := dockerfile.lines() {
			eng.Ctl(line)
		}
	} else if args[0] == "expose" {
		// Expose a TCP port
	} else if args[0] == "connect" {
		// Discover a TCP port to connect to
	} else if args[0] == "prompt" {
		// Prompt the user for a value
	} else if args[0] == "commit" {
		// Commit a new snapshot of this image
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
	if err := os.MkdirAll(c.Path(".docker"), 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	// ROOT/.docker didn't exist: set it up
	defer func() { if err != nil { os.RemoveAll(c.Path(".docker")) } }()
	// Generate an engine ID
	if err := writeFile(c.Path(".docker/engine/id"), GenerateID() + "\n"); err != nil {
		return nil, err
	}
	// Setup .docker/bin/docker
	if err := os.MkdirAll(c.Path(".docker/bin"), 0700); err != nil && !os.IsExist(err) {
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
		if err := symlink("docker", c.Path(".docker/bin", cmd)); err != nil {
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

func (c *Container) NewCommand(name, path string, arg ...string) (*Cmd, error) {
	Debugf("NewCommand()")
	cmd := &Cmd{
		Name:		name,
		container:	c,
		Cmd:		exec.Command(path, arg...),
	}
	if err := cmd.store(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (c *Container) GetCommand(name string) (*Cmd, error) {
	cmd := &Cmd{
		Name:		name,
		container:	c,
		Cmd:		exec.Command(""), // This gets filled in by load()
	}
	if err := cmd.load(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (cmd *Cmd) store() error {
	Debugf("store()")
	Debugf("Storing %s:%s on %s", cmd.container.Id, cmd.Name, cmd.path("/"))
	if err := cmd.lockName(); err != nil {
		return err
	}
	// Store command-line on disk
	cmdline := []string{cmd.Path}
	cmdline = append(cmdline, cmd.Args...)
	if err := writeFile(cmd.path("cmd"), strings.Join(cmdline, "\x00")); err != nil {
		return err
	}
	// Store env on disk
	for _, kv := range cmd.Env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) < 2 {
			parts = append(parts, "")
		}
		if err := writeFile(cmd.path("env", parts[0]), parts[1]); err != nil {
			return err
		}
	}
	// Store working directory on disk
	if err := writeFile(cmd.path("wd"), cmd.Dir); err != nil {
		return err
	}
	return nil
}

func (cmd *Cmd) load() error {
	// Load command-line
	cmdline, err := readFile(cmd.path("cmd"))
	if err != nil {
		return err
	}
	cmdlineParts := strings.Split(cmdline, "\x00")
	cmd.Path = cmdlineParts[0]
	if len(cmdlineParts) > 1 {
		cmd.Args = cmdlineParts[1:]
	} else {
		cmd.Args = nil
	}
	// FIXME: load env
	// Load working directory
	if wd, err := readFile(cmd.path("wd")); err != nil {
		Debugf("No working directory")
	} else {
		cmd.Dir = wd
	}
	return nil
}

func (cmd *Cmd) path(p ...string) string {
	prefix := []string{".docker/run/exec", cmd.Name}
	return cmd.container.Path(append(prefix, p...)...)
}

func (c *Cmd) lockName() error {
	if c.Name != "" {
		return os.MkdirAll(c.path("/"), 0700)
	}
	// If no name is defined, allocate one
	name, err := mkUniqueDir(c.container.Path(".docker/exec"))
	if err != nil {
		return nil
	}
	c.Name = name
	return nil
}

func (c *Container) baseEnv() []string {
	var paths []string
	for _, a := range []string{"/usr/local", "/usr", "/"} {
		for _, b := range []string{"bin", "sbin"} {
			paths = append(paths, path.Join(c.Root, a, b))
		}
	}
	return []string{
		"HOME=" + c.Root,
		"PATH=" + strings.Join(paths, ":"),
	}
}

type Cmd struct {
	*exec.Cmd
	Name		string
	container	*Container
}

func (cmd *Cmd) Run() error {
	cmd.Dir = cmd.container.Path(cmd.Dir)
	cmd.Env = append(cmd.container.baseEnv(), cmd.Env...)
	return cmd.Cmd.Run()
}


// Engine


type Engine struct {
	c0   *Container // container 0, aka the root container
}

func NewEngine(c0 *Container) (*Engine, error) {
	return &Engine{
		c0: c0,
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

// Ctl connects to the engine's control socket and issues standard operations on it.
// This is used to pass user commands to the engine, and also for further introspection
// by the containers themselves.
func (eng *Engine) Ctl(ops ...string) error {
	s, err := net.Dial("unix", eng.Path("ctl"))
	if err != nil {
		return err
	}
	commands := []string{
		// Execute this in container zero
		"in 0",
		strings.Join(flag.Args(), "\x00"),
		"wait",
		"die",
	}
	if _, err := io.Copy(s, strings.NewReader(strings.Join(ops, "\n"))); err != nil {
		return err
	}
	resp, err := ioutil.ReadAll(s)
	if err != nil {
		return err
	}
	if len(resp) != 0 {
		return fmt.Errorf("Engine error: " + string(resp))
	}
	return nil
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
		// FIXME: commit the current container before each command
		Debugf("Reading command...")
		line, err := lines.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		line = strings.Trim(line, "\n")
		Debugf("Processing command: %s", line)
		op, err := ParseOp(line)
		if err != nil {
			return err
		}
		if op.Name == "in" {
			ctx, err := eng.Get(op.Args[0])
			if err != nil {
				return err
			}
			chain.context = ctx
		} else if op.Name == "from" {
			src, err := eng.Get(op.Args[0])
			if err != nil {
				return err
			}
			// FIXME: implement actual COMMIT of src into ctx
			ctx, err := eng.Create()
			if err != nil {
				return err
			}
			Debugf("Committed %s to %s (not really)", src.Id, ctx.Id)
			chain.context = ctx
		} else if chain.context == nil {
			ctx, err := eng.Create()
			if err != nil {
				return err
			}
			chain.context = ctx
		} else {
			Debugf("Preparing to execute commnad in context %s", chain.context.Id)
			// Execute command as a process inside the root container...
			cmd, err := eng.c0.NewCommand("", "docker", append([]string{"--engine", op.Name}, op.Args...)...)
			if err != nil {
				return err
			}
			// ...with the current context as cwd
			cmd.Dir = chain.context.Root
			Debugf("Container will be run in %s", cmd.Dir)
			// DOCKER_ROOT points to the root of the container
			// In a chrooted environment, this would default to /
			cmd.Env = append(cmd.Env, "DOCKER_ROOT=" + eng.c0.Root
			Debugf("Attaching to stdout and stderr")
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				return err
			}
			go io.Copy(os.Stdout, stdout)
			stderr, err := cmd.StderrPipe()
			if err != nil {
				return err
			}
			go io.Copy(os.Stderr, stderr)
			Debugf("Starting command")
			if err := cmd.Start(); err != nil {
				return err
			}
			Debugf("Waiting for command")
			if err := cmd.Wait(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (eng *Engine) Path(p ...string) string {
	//  <c0_root>/.docker/engine/<p>
	return eng.c0.Path(append([]string{".docker/engine"}, p...)...)
}

func (eng *Engine) Chain() *Chain {
	return &Chain{
		engine: eng,
	}
}

func (eng *Engine) Get(name string) (*Container, error) {
	if name == "0" {
		return eng.c0, nil
	}
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

func (eng *Engine) Create() (*Container, error) {
	// FIXME: create from a parent, nested naming etc.
	id, err := mkUniqueDir(eng.Path("/containers"))
	if err != nil {
		return nil, err
	}
	Debugf("Created new container: %s at root %s", id, eng.Path("/containers", id))
	return &Container{
		Id:	id,
		Root:	eng.Path("/containers", id),
	}, nil
}


// Cmd parses and execute a command in the chain.
// Examples of valid command input:
//	"PULL ubuntu"
//	"START"
//	"EXEC ls\x00-l"
func (chain *Chain) Op(input string) error {
	op, err := ParseOp(input)
	if err != nil {
		return err
	}
	// FIXME: insert pre-hooks here
	// FIXME: insert default commands here
	method, exists := chain.getMethod(op.Name)
	if !exists {
		return fmt.Errorf("No such command: %s", op.Name)
	}
	ret := method.Func.CallSlice([]reflect.Value{
		reflect.ValueOf(chain),
		reflect.ValueOf(op.Args),
	})[0].Interface()
	if ret == nil {
		return nil
	}
	return ret.(error)
	// FIXME: insert post-hooks here
}


// Command

type Op struct {
	Name	string
	Args	[]string
}

func ParseOp(input string) (*Op, error) {
	parts := strings.SplitN(input, " ", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%s: invalid format", input)
	}
	return &Op{
		Name: strings.ToLower(parts[0]),
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


func (chain *Chain) CmdStart(args ...string) (err error) {
	if chain.context == nil {
		return fmt.Errorf("No context set")
	}
	// Iterate on commands
	// For each command, call CmdExec
	// Check if already running
	return fmt.Errorf("No yet implemented") // FIXME
}

func (chain *Chain) CmdImport(args ...string) (err error) {
	fmt.Printf("Importing %s...\n", args[0])
	return nil
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

func symlink(newname, oldname string) error {
	// If it already exists, remove it. This emulates 'ln -s -f' behavior
	// FIXME: this is prone to race condition.
	if err := os.Remove(oldname); err != nil && !os.IsNotExist(err) {
		return err
	}
	// Create subdirectories if necessary
	if err := os.MkdirAll(path.Dir(oldname), 0700); err != nil && !os.IsExist(err) {
		return err
	}
	return os.Symlink(newname, oldname)
}

// Return the contents of file at path `src`.
// Call t.Fatal() at the first error (including if the file doesn't exist)
func readFile(src string) (content string, err error) {
	f, err := os.Open(src)
	if err != nil {
		return "", err
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func mkUniqueDir(parent string) (dir string, err error) {
	var i int64
	// FIXME: store a hint on disk to avoid scanning from 1 everytime
	for i=0; i<1<<63 - 1; i+= 1 {
		name := fmt.Sprintf("%d", i)
		err := os.MkdirAll(path.Join(parent, name), 0700)
		if os.IsExist(err) {
			continue
		} else if err != nil {
			return "", err
		}
		return name, nil
	}
	return "", fmt.Errorf("Cant allocate anymore children in %s", parent)
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


//
// ENGINE COMMANDS
//


