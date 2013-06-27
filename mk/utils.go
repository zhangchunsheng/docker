
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

