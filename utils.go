package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"io"
	"io/ioutil"
	"os/exec"
	"path"
	"crypto/rand"
	"encoding/hex"
	"runtime"
	"sort"
	"net/http"
)

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

func symlink(oldname, newname string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("symlink: %s", err)
		}
	}()
	// If it already exists, remove it. This emulates 'ln -s -f' behavior
	// FIXME: this is prone to race condition.
	if err := os.Remove(newname); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Can't remove %s: %s", newname, err)
	}
	// Create subdirectories if necessary
	if err := os.MkdirAll(path.Dir(newname), 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("Can't mkdir %s: %s", newname, err)
	}
	return os.Symlink(oldname, newname)
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

// ls returns the contents of a directory as a list of filenames.
// If the directory doesn't exist, it returns an empty list. Otherwise,
// it returns the first error encountered.
func LS(dir string) ([]string, error) {
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

func containerPath(name string) string {
	return path.Clean(strings.Join(append([]string{"/"}, containerParts(name)...), "/.docker/engine/containers/"))
}

func containerParts(name string) (parts []string) {
	defer func() {
		Debugf("containerParts = %s", parts)
	}()
	name = path.Clean(name)
	var (
		base string
	)
	for {
		name, base = path.Split(name)
		if base != "" {
			parts = append([]string{base}, parts...)
		} else {
			break
		}
	}
	return parts
}

func mkUniqueDir(parent, prefix, name string) (dir string, err error) {
	Debugf("mkUniqueDir(%v, %v, %v)...", parent, prefix, name)
	defer func() {
		Debugf("mkUniqueDir -> (%v, %v)", name, dir, err)
	}()
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	var i int64
	if name != "" {
		if err := os.Mkdir(path.Join(parent, prefix + name), 0700); err != nil {
			return "", err
		}
		return name, nil
	}
	// FIXME: store a hint on disk to avoid scanning from 1 everytime
	for i=1; i<1<<63 - 1; i+= 1 {
		name = fmt.Sprintf("%d", i)
		err := os.Mkdir(path.Join(parent, prefix + name), 0700)
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


func Log(format string, a...interface{}) (int, error) {
	return fmt.Printf(fmt.Sprintf("[%d] %s", os.Getpid(), format), a...)
}

// Request a given URL and return an io.Reader
func Download(url string, stderr io.Writer) (*http.Response, error) {
	var resp *http.Response
	var err error
	if resp, err = http.Get(url); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Got HTTP status code >= 400: " + resp.Status)
	}
	return resp, nil
}
