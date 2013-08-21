package docker

import (
	"fmt"
	"github.com/dotcloud/docker/0.x/term"
	redis "github.com/dotcloud/go-redis-server"
	"github.com/kr/pty"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

type MyHandler struct {
	redis.DefaultHandler
	eng           *Engine
	session       *Session
	cmd           Cmd
	master, slave *os.File
	br            map[string][][]byte
	brC           chan bool
}

var r, w = io.Pipe()
var r2, w2 = io.Pipe()

func (h *MyHandler) Write(buf []byte) (int, error) {
	n2, err := h.RPUSH("cmd-write", buf)
	if n2 != 1 {
		println("not alone!!")
	}
	n, err := w.Write(buf)
	if err != nil {
		return n, err
	}
	if n != len(buf) {
		println("write cmd-write mismatch:", n, len(buf))
	}
	return len(buf), err
}

func (h *MyHandler) Read(buf []byte) (int, error) {
	buf2 := make([]byte, len(buf), cap(buf))
	buf3 := make([]byte, len(buf), cap(buf))

	b, err := h.BLPOP([]byte("cmd-read"))

	n, err := r2.Read(buf2)
	if err != nil {
		return n, err
	}

	// Make a FIFO stack to avoid issue with buf cap
	if len(b[1]) > cap(buf) {
		panic("------------------ READ 1")
	}
	a1 := copy(buf3[:n], buf2[:n])
	if n != len(b[1]) {
		println("____________ LEN MISMATCH1")
	}
	n = len(b[1])
	a2 := copy(buf[:n], b[1])
	if a1 != a2 {
		println("copy mismatch!!!!!!", a1, a2)
	}

	for i, c := range buf3[:n] {
		if i > len(buf) {
			println("out of range!")
		}
		if buf[i] != c {
			fmt.Printf("MISMATCH!(%d):  %#v, %#v", n, buf[:n], buf3[:n])
			break
		}
	}

	if n != len(b[1]) {
		println("read cmd-read mismatch:", n, len(b[1]))
	}

	if len(buf3) != len(buf) || cap(buf3) != cap(buf) {
		println("read mismatch!!!", len(buf), len(buf3), cap(buf), cap(buf3))
	}

	return len(buf[:n]), err
}

type testWriter struct{ *MyHandler }

func (h *testWriter) Write(buf []byte) (int, error) {
	n2, err := h.RPUSH("cmd-read", buf)
	if n2 != 1 {
		println("not alone2!!!!!!!")
	}
	n, err := w2.Write(buf)
	if err != nil {
		return n, err
	}
	if n != len(buf) {
		println("write cmd-read mismatch:", n, len(buf))
	}
	return len(buf), err
}

func (h *testWriter) Read(buf []byte) (int, error) {
	buf2 := make([]byte, len(buf), cap(buf))
	buf3 := make([]byte, len(buf), cap(buf))

	b, err := h.BLPOP([]byte("cmd-write"))
	// Make a FIFO stack to avoid issue with buf cap
	if len(b[1]) > cap(buf) {
		panic("------------------ READ 2")
	}

	n, err := r.Read(buf2)
	if err != nil {
		return n, err
	}

	a1 := copy(buf3[:n], buf2[:n])

	if n != len(b[1]) {
		println("____________ LEN MISMATCH22")
	}
	n = len(b[1])

	a2 := copy(buf[:n], b[1])
	if a1 != a2 {
		println("copy2 mistach!!", a1, a2)
	}

	for i, c := range buf3[:n] {
		if i > len(buf) {
			println("out of range22!")
		}
		if buf[i] != c {
			fmt.Printf("MISMATCH22!(%d) [:  >>>%s<<<<, >>>>%s<<<<<", n, buf[:n], buf3[:n])
			break
		}
	}

	if len(buf3) != len(buf) || cap(buf3) != cap(buf) {
		println("read mismatch!!!", len(buf), len(buf3), cap(buf), cap(buf3))
	}

	// if n != len(b[1]) {
	// 	println("read cmd-write mismatch:", n, len(b[1]))
	// }

	return len(buf[:n]), err
}

func (h *MyHandler) HSET(key, subkey string, value []byte) (int, error) {
	switch key {
	case "cmd":
		switch subkey {
		case "path":
			h.cmd.Path = string(value)
		case "dir":
			h.cmd.Dir = string(value)
		case "args":
			if h.cmd.Args == nil {
				h.cmd.Args = []string{string(value)}
			} else {
				h.cmd.Args = append(h.cmd.Args, string(value))
			}
		case "env":
			if h.cmd.Env == nil {
				h.cmd.Env = []string{string(value)}
			} else {
				h.cmd.Env = append(h.cmd.Env, string(value))
			}
		case "tty":
			if v, err := strconv.ParseBool(string(value)); err == nil && v {
				m, s, err := pty.Open()
				if err != nil {
					return 0, err
				}
				h.master = m
				h.slave = s

				win, _ := term.GetWinsize(os.Stdin.Fd())
				term.SetWinsize(m.Fd(), win)
			}
		case "start":
			if v, err := strconv.ParseBool(string(value)); err == nil && v {
				cmd := exec.Command(h.cmd.Path)
				if h.slave != nil {
					cmd.Stdin = h.slave
					cmd.Stdout = h.slave
					cmd.Stderr = h.slave
					cmd.SysProcAttr = &syscall.SysProcAttr{Setctty: true, Setsid: true}
				}

				if h.master != nil {
					go io.Copy(h, h.master)
					go io.Copy(h.master, h)
				}

				if err := cmd.Start(); err != nil {
					return 0, err
				}

				go func() {
					cmd.Wait()
					h.RPUSH("CMDEND", []byte{})
				}()

			}
		case "attach":
			if v, err := strconv.ParseBool(string(value)); err == nil && v {
				old, err := term.SetRawTerminal(os.Stdin.Fd())
				if err != nil {
					panic(err)
				}
				defer term.RestoreTerminal(os.Stdin.Fd(), old)

				go io.Copy(os.Stdout, &testWriter{h})
				go io.Copy(&testWriter{h}, os.Stdin)
				h.BRPOP([]byte("CMDEND"))
			}
		case "run":
			if v, err := strconv.ParseBool(string(value)); err == nil && v {
				old, err := term.SetRawTerminal(os.Stdin.Fd())
				if err != nil {
					return 0, err
				}
				defer term.RestoreTerminal(os.Stdin.Fd(), old)

				ps, err := h.cmd.Run(h.session.root.Root)
				if err != nil {
					return 0, err
				}
				if h.cmd.in == nil {
					ps.Stdin = os.Stdin
				} else {
					ps.Stdin = h.cmd.in
					go io.Copy(h.master, os.Stdin)
				}
				if h.cmd.out == nil {
					ps.Stdout = os.Stdout
				} else {
					ps.Stdout = h.cmd.out
					go io.Copy(os.Stdout, h.master)
				}
				if h.cmd.err == nil {
					ps.Stderr = ps.Stderr
				} else {
					ps.Stderr = h.cmd.err
				}
				if err := ps.Run(); err != nil {
					return 0, err
				}
			}
		}
	default:
		return h.DefaultHandler.HSET(key, subkey, value)
	}
	return 0, nil
}

func (h *MyHandler) SET(key string, value []byte) error {
	redis.Debugf("Setting key [%s] (%s)", key, value)

	if h.session == nil {
		s, err := h.eng.NewSession(nil, h.eng.c0)
		if err != nil {
			fmt.Printf("Error creating new session: %s\n", err)
			return err
		}
		h.session = s
	}
	switch key {
	case "cd":
		if err := h.session.CD(string(value)); err != nil {
			return err
		}
	case "ps":
		containers, err := h.session.context.ListChildren()
		if err != nil {
			return err
		}
		for _, cName := range containers {
			c, err := h.session.context.GetChild(cName)
			if err != nil {
				Debugf("Can't load container %s\n", cName)
				continue
			}
			Debugf("Child = %s", c)
			commands, err := LS(c.Path(".docker/run/exec"))
			for _, cmdName := range commands {
				cmd, err := c.GetCommand(cmdName)
				if err != nil {
					Debugf("Can't load command %s:%s\n", cName, cmdName)
					continue
				}
				fmt.Printf("%s:%s\t%s %s\n", c.Id, cmdName, cmd.Path, strings.Join(cmd.Args, " "))
			}
		}
	case "ls":
		containers, err := h.session.context.ListChildren()
		if err != nil {
			return err
		}
		for _, cName := range containers {
			fmt.Println(cName)
		}
	case "name":
		if err := h.session.root.NameChild(string(value), h.session.contextPath); err != nil {
			return err
		}
	case "exec":
		Debugf("Preparing to execute command in context %s", h.session.context.Id)
		cmd := exec.Command("/bin/bash", "-c", string(value))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	case "die":
		// FIXME: End of transaction ? Do we really need this?
	default:
		return h.DefaultHandler.SET(key, value)
	}
	return nil
}

func NewHandler(eng *Engine) *MyHandler {
	return &MyHandler{
		eng: eng,
	}
}
