package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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
	"sort"
)

func main() {
//	os.Setenv("DEBUG", "1")
	fmt.Printf("Main %s\n", os.Args)
	flEngine := flag.Bool("e", false, "Engine mode")
	flag.Parse()
	if *flEngine {
		if err := engineMain(flag.Args()); err != nil {
			Fatalf("Failed to execute engine command '%s': %s",
				strings.Join(flag.Args(), " "), err)
		}
		os.Exit(0)
	}
	if flag.NArg() < 1 {
		fmt.Printf("Usage: mk CMD [ARGS...]\n")
		os.Exit(1)
	}

	c, err  := NewContainer("0", ".")
	if err != nil {
		Fatalf("Failed to setup root container: %s", err)
	}
	eng, err := NewEngine(c) // Pass the root container to the engine
	if err != nil {
		Fatalf("Failed to initialize engine: %s", err)
	}
	defer eng.Cleanup()
	ready := make(chan bool)
	go func() {
		if err := eng.ListenAndServe(ready); err != nil {
			Fatal(err)
		}
	}()
	<-ready
	if err := eng.Ctl(
		[]string{"in", "0"},
		flag.Args(),
		[]string{"die"},
	); err != nil {
		Fatalf("Error sending engine startup commands: %s", err)
	}
}

func log(format string, a...interface{}) (int, error) {
	return fmt.Printf(fmt.Sprintf("[%d] %s", os.Getpid(), format), a...)
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

// ls returns the contents of a directory as a list of filenames.
// If the directory doesn't exist, it returns an empty list. Otherwise,
// it returns the first error encountered.
func ls(dir string) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return []string{}, nil
	} else if err != nil {
		return nil, err
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name())
	}
	// FIXME: sort by date
	// FIXME: configurable sorting
	sort.Strings(names)
	return names, nil
}

func (c *Container) LS(dir string) ([]string, error) {
	return ls(c.Path(dir))
}


// This runs in a separate process in c0, chdired to the target container
// NOTE: we may not be chrooted, so don't assume / is the root of c0
// FIXME: do we need access to the root of c0?
func engineMain(args []string) error {
	self, err := CurrentContainer() // This needs access to c0
	if err != nil {
		return err
	}
	eng := self.Engine()
	if args[0] == "import" {
		log("Importing from %s", args[1])
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
		commands, err := self.LS(".docker/run/exec")
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
			log("build op '%s'\n", line)
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

func NewContainer(id, root string) (*Container, error) {
	abspath, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	c := &Container{
		Root: abspath,
		Id: id,
	}
	// Create /.docker
	if err := os.MkdirAll(c.Path(".docker"), 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	// /.docker didn't exist: set it up
	defer func() { if err != nil { os.RemoveAll(c.Path(".docker")) } }()
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

func (c *Container) GetCommand(name string) (*Cmd, error) {
	cmd := new(Cmd)
	// Load command-line
	cmdline, err := readFile(c.Path("/.docker/run/exec", name, "cmd"))
	if err != nil {
		return nil, err
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
	if wd, err := readFile(c.Path("/.docker/run/exec", name, "wd")); err != nil {
		Debugf("No working directory")
	} else {
		cmd.Dir = wd
	}
	return cmd, nil
}



func (c *Container) SetCommand(name string, cmd *Cmd) (string, error) {
	var err error
	name, err = mkUniqueDir(c.Path("/.docker/run/exec"), name)
	if err != nil {
		return "", err
	}
	Debugf("Storing %s:%s on %s", c.Id, name, c.Path("/.docker/run/exec", name))
	// Store command-line on disk
	cmdline := []string{cmd.Path}
	cmdline = append(cmdline, cmd.Args...)
	if err := writeFile(c.Path("/.docker/run/exec", name, "cmd"), strings.Join(cmdline, "\x00")); err != nil {
		return "", err
	}
	// Store env on disk
	for _, kv := range cmd.Env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) < 2 {
			parts = append(parts, "")
		}
		if err := writeFile(c.Path("/.docker/run/exec", name, "env", parts[0]), parts[1]); err != nil {
			return "", err
		}
	}
	// Store working directory on disk
	if err := writeFile(c.Path("/.docker/run/exec", name, "wd"), cmd.Dir); err != nil {
		return "", err
	}
	return name, nil
}


var BaseEnv = []string{
	"HOME=/",
	"PATH=/.docker/bin:/usr/local/bin:/usr/local/sbin:/usr/bin:/usr/sbin:/bin:/sbin",
	// DOCKER_ROOT points to the root of the container
	// In a chrooted environment, this would default to /
	"DOCKER_ROOT=/",
}

// NewEnv creates a new environment for use by a containerized process.
func NewEnv(prefix string, override ...string) (env []string) {
	for _, kv := range append(BaseEnv, override...) {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) < 2 {
			parts = append(parts, "")
		}
		key, value := parts[0], parts[1]
		if key == "HOME" || key == "DOCKER_ROOT" {
			value = path.Join(prefix, value)
		} else if key == "PATH" {
			searchPath := strings.Split(value, ":")
			for i := range strings.Split(value, ":") { // Don't use filepath.SplitList, it depends on host system
				searchPath[i] = path.Join(prefix, searchPath[i])
			}
			value = strings.Join(searchPath, ":")
		}
		env = append(env, key + "=" + value)
	}
	return
}


func getenv(key string, env []string) (value string) {
	for _, kv := range env {
		if strings.Index(kv, "=") == -1 {
			continue
		}
		parts := strings.SplitN(kv, "=", 2)
		if parts[0] != key {
			continue
		}
		if len(parts) < 2 {
			value = ""
		} else {
			value = parts[1]
		}
	}
	return
}

func lookPath(target string, env []string) (string, error) {
	if filepath.IsAbs(target) {
		return target, nil
	}
	for _, searchPath := range filepath.SplitList(getenv("PATH", env)) {
		Debugf("Searching for %s in %s", target, searchPath)
		p := path.Join(searchPath, target)
		if st, err := os.Stat(p); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return "", err
		} else {
			if st.IsDir() {
				continue
			}
			Debugf("Found it! %s", p)
			// FIXME: check for executable bit
			return p, nil
		}
	}
	return "", fmt.Errorf("executable file not found in $PATH")
}




type Cmd struct {
	Path		string
	Args		[]string
	Env		[]string
	Dir		string
}


func (cmd *Cmd) Run(root string) (*exec.Cmd, error) {
	realEnv := NewEnv(root, cmd.Env...)
	realPath, err := lookPath(cmd.Path, realEnv)
	if err != nil {
		return nil, err
	}
	ps := exec.Command(realPath, cmd.Args...)
	ps.Env = realEnv
	ps.Dir = path.Join(root, cmd.Dir) // FIXME: this is vulnerable to untrusted input, ../.. etc.
	Debugf("Running %s in %s with PATH=%s", ps.Path, ps.Dir, getenv("PATH", ps.Env))
	return ps, nil
}

// Engine


type Engine struct {
	c0   *Container // container 0, aka the root container
}

func NewEngine(c0 *Container) (*Engine, error) {
	// Generate an engine ID
	if err := writeFile(c0.Path(".docker/engine/id"), GenerateID() + "\n"); err != nil {
		return nil, err
	}
	// Link containers/0 to the root container
	if err := symlink("../../..", c0.Path(".docker/engine/containers/0")); err != nil {
		return nil, err
	}
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
			Debugf("Cleaning up leftover unix socket\n")
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
	signal.Notify(signals, os.Interrupt, os.Kill)
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
			Fatal(err)
		}
		Debugf("Received connection: %s", conn)
		go eng.Serve(conn)
	}
}

// Ctl connects to the engine's control socket and issues a standard operation on it.
// This is used to pass user commands to the engine, and also for further introspection
// by the containers themselves.
// The protocol is inspired by the Redis wire protocol (http://redis.io/topics/protocol).
func (eng *Engine) Ctl(ops ...[]string) error {
	s, err := net.Dial("unix", eng.Path("ctl"))
	if err != nil {
		return err
	}
	defer s.Close()
	reader := bufio.NewReader(s)
	for idx, opArgs := range ops {
		Debugf("Sending step #%d ---> %s\n", idx + 1, strings.Join(opArgs, " "))
		sWriter := io.MultiWriter(s)
		// Send total number of arguments (including op name)
		if _, err := fmt.Fprintf(sWriter, "*%d\r\n", len(opArgs)); err != nil {
			return err
		}
		// Send op name as arg #1, followed by op arguments as args #2-#n
		for _, arg := range opArgs {
			if _, err := fmt.Fprintf(sWriter, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
				return err
			}
		}
		// FIXME: implement redis reply protocol
		Debugf("Reading response...")
		resp, err := reader.ReadBytes('\n')
		if err != nil {
			return err
		}
		if len(resp) == 0 {
			return fmt.Errorf("Engine unexpectedly hung up")
		}
		respCode := resp[0]
		var respData []byte
		if len(resp) > 1 {
			respData = resp[1:]
		}
		if respCode == '-' {
			return fmt.Errorf("Engine error: %s", respData)
		} else if respCode == '+' {
			Debugf("Engine status: %s", respData)
		} else {
			return fmt.Errorf("Engine returned unknown reply code '%c': (\"%s\")", respCode, string(resp))
		}
	}
	return nil
}

func (eng *Engine) Serve(conn net.Conn) (err error) {
	defer func() {
		if err != nil {
			fmt.Fprintf(conn, "-%s\n", err)
		}
		conn.Close()
	}()
	reader := bufio.NewReader(conn)
	chain := eng.Chain()
	for {
		// FIXME: commit the current container before each command
		Debugf("Reading command...")
		var nArg int
		line, err := reader.ReadString('\r')
		if err != nil {
			return err
		}
		Debugf("line == '%s'", line)
		if len(line) < 1 || line[len(line) - 1] != '\r' {
			return fmt.Errorf("Malformed request: doesn't start with '*<nArg>\\r\\n'. %s", err)
		}
		line = line[:len(line) - 1]
		if _, err := fmt.Sscanf(line, "*%d", &nArg); err != nil {
			return fmt.Errorf("Malformed request: '%s' doesn't start with '*<nArg>'. %s", line, err)
		}
		Debugf("nArg = %d", nArg)
		nl := make([]byte, 1)
		if _, err := reader.Read(nl); err != nil {
			return err
		} else if nl[0] != '\n' {
			return fmt.Errorf("Malformed request: expected '%x', got '%x'", '\n', nl[0])
		}
		var op Op
		for i:=0; i<nArg; i+=1 {
			Debugf("\n-------\nReading arg %d/%d", i + 1, nArg)
			// FIXME: specify int size?
			var argSize int64

			line, err := reader.ReadString('\r')
			if err != nil {
				return err
			}
			Debugf("line == '%s'", line)
			if len(line) < 1 || line[len(line) - 1] != '\r' {
				return fmt.Errorf("Malformed request: doesn't start with '$<nArg>\\r\\n'. %s", err)
			}
			line = line[:len(line) - 1]
			if _, err := fmt.Sscanf(line, "$%d", &argSize); err != nil {
				return fmt.Errorf("Malformed request: '%s' doesn't start with '$<nArg>'. %s", line, err)
			}
			Debugf("argSize= %d", argSize)
			nl := make([]byte, 1)
			if _, err := reader.Read(nl); err != nil {
				return err
			} else if nl[0] != '\n' {
				return fmt.Errorf("Malformed request: expected '%x', got '%x'", '\n', nl[0])
			}


			// Read arg data
			argData, err := ioutil.ReadAll(io.LimitReader(reader, argSize + 2))
			if err != nil {
				return err
			} else if n := int64(len(argData)); n < argSize + 2 {
				return fmt.Errorf("Malformed request: argument data #%d doesn't match declared size (expected %d bytes (%d + \r\n), read %d)", i, argSize + 2, argSize, n)
			} else if string(argData[len(argData) - 2:]) != "\r\n" {
				return fmt.Errorf("Malformed request: argument #%d doesn't end with \\r\\n", i)
			}
			arg := string(argData[:len(argData) - 2])
			Debugf("arg = %s", arg)
			if i == 0 {
				op.Name = strings.ToLower(arg)
			} else {
				op.Args = append(op.Args, arg)
			}
		}
		// Whatever is left of the connection is stdin
		// FIXME: in order to pass stdin, it must be framed..
		//   BECAUSE we need to support chaining of commands on the same connection...
		//   BECAUSE chaining of commands is the only practical way to pass a container between commands
		//op.Stdin = reader
		fmt.Printf("---> %s %s\n", op.Name, op.Args)
		// IN and FROM affect the context
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
			fmt.Printf("Committed %s to %s (not really)\n", src.Id, ctx.Id)
			chain.context = ctx
		} else if op.Name == "die" {
			fmt.Fprintf(conn, "+OK\n")
			return nil
		} else if op.Name == "ls" {
			containers, err := eng.List()
			if err != nil {
				return err
			}
			for _, cName := range containers {
				fmt.Println(cName)
			}
		} else if op.Name == "ps" {
			containers, err := eng.List()
			if err != nil {
				return err
			}
			for _, cName := range containers {
				c, err := eng.Get(cName)
				if err != nil {
					Debugf("Can't load container %s\n", cName)
					continue
				}
				commands, err := c.LS(".docker/run/exec")
				for _, cmdName := range commands {
					cmd, err := c.GetCommand(cmdName)
					if err != nil {
						Debugf("Can't load command %s:%s\n", cName, cmdName)
						continue
					}
					fmt.Printf("%s:%s\t%s\n", c.Id, cmdName, strings.Join(cmd.Args, " "))
				}
			}
		} else {
			// If context is still not set, create an new empty container as context
			if chain.context == nil {
				ctx, err := eng.Create()
				if err != nil {
					return err
				}
				chain.context = ctx
			}
			Debugf("Preparing to execute command in context %s", chain.context.Id)
			cmd := new(Cmd)
			cmd.Path = "docker"
			cmd.Args = []string{"-e", op.Name}
			cmd.Args = append(cmd.Args, op.Args...)
			// ...with the current context as cwd
			// (relative to the container)
			cmd.Dir = "/.docker/engine/containers/" + chain.context.Id
			_, err := chain.context.SetCommand("", cmd)
			if err != nil {
				return err
			}
			// Execute command as a process inside c0
			ps, err := cmd.Run(eng.c0.Root)
			if err != nil {
				return err
			}
			ps.Stdout = os.Stdout
			ps.Stderr = os.Stderr
			Debugf("Starting command")
			if err := ps.Run(); err != nil {
				return err
			}
			Debugf("Command returned")
		}
		// Send a successful reply
		Debugf("Sending OK")
		fmt.Fprintf(conn, "+OK\n")
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
	id, err := mkUniqueDir(eng.Path("/containers"), "")
	if err != nil {
		return nil, err
	}
	Debugf("Created new container: %s at root %s", id, eng.Path("/containers", id))
	return NewContainer(id, eng.Path("/containers", id))
}

func (eng *Engine) List() ([]string, error) {
	return ls(eng.Path("/containers"))
}



// Command

type Op struct {
	Name	string
	Args	[]string
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

func mkUniqueDir(parent string, name string) (dir string, err error) {
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	var i int64
	if name != "" {
		if err := os.Mkdir(path.Join(parent, name), 0700); err != nil {
			return "", err
		}
		return name, nil
	}
	// FIXME: store a hint on disk to avoid scanning from 1 everytime
	for i=0; i<1<<63 - 1; i+= 1 {
		name = fmt.Sprintf("%d", i)
		err := os.Mkdir(path.Join(parent, name), 0700)
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

		fmt.Fprintf(os.Stderr, fmt.Sprintf("[%d] [debug] %s:%d %s\n", os.Getpid(), file, line, format), a...)
	}
}

func Fatalf(format string, a ...interface{}) {
	if len(format) > 0 && format[len(format) - 1] != '\n' {
		format = format + "\n"
	}
	fmt.Fprintf(os.Stderr, fmt.Sprintf("[%d] Fatal: %s", os.Getpid(), format), a...)
	os.Exit(1)
}

func Fatal(err error) {
	Fatalf("%s", err)
}


//
// ENGINE COMMANDS
//


