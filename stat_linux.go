// FIXME: Make a new docker.Stat type that will wrap os.FileInfo and handle everything
package docker

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func getRdev(fi os.FileInfo) (int, error) {
	srcSys, ok := fi.Sys().(syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("Imposisble to retrieve sys stats")
	}
	return int(srcSys.Rdev), nil
}

func getUid(fi os.FileInfo) (int, error) {
	srcSys, ok := fi.Sys().(syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("Imposisble to retrieve sys stats")
	}
	return int(srcSys.Uid), nil
}

func getGid(fi os.FileInfo) (int, error) {
	srcSys, ok := fi.Sys().(syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("Imposisble to retrieve sys stats")
	}
	return int(srcSys.Gid), nil
}

func getAtime(fi os.FileInfo) (time.Duration, error) {
	srcSys, ok := fi.Sys().(syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("Imposisble to retrieve sys stats")
	}
	return time.Duration(srcSys.Atim.Nsec), nil
}

func getMtime(fi os.FileInfo) (time.Duration, error) {
	srcSys, ok := fi.Sys().(syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("Imposisble to retrieve sys stats")
	}
	return time.Duration(srcSys.Mtim.Nsec), nil
}
