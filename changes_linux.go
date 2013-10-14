package docker

import (
	"syscall"
)

func (info *FileInfo) addChanges(oldInfo *FileInfo, changes *[]Change) {
	if oldInfo == nil {
		// add
		change := Change{
			Path: info.path(),
			Kind: ChangeAdd,
		}
		*changes = append(*changes, change)
	}

	// We make a copy so we can modify it to detect additions
	// also, we only recurse on the old dir if the new info is a directory
	// otherwise any previous delete/change is considered recursive
	oldChildren := make(map[string]*FileInfo)
	if oldInfo != nil && info.isDir() {
		for k, v := range oldInfo.children {
			oldChildren[k] = v
		}
	}

	for name, newChild := range info.children {
		oldChild, _ := oldChildren[name]
		if oldChild != nil {
			// change?
			oldStat := &oldChild.stat
			newStat := &newChild.stat
			// Note: We can't compare inode or ctime or blocksize here, because these change
			// when copying a file into a container. However, that is not generally a problem
			// because any content change will change mtime, and any status change should
			// be visible when actually comparing the stat fields. The only time this
			// breaks down is if some code intentionally hides a change by setting
			// back mtime
			oldMtime := syscall.NsecToTimeval(oldStat.Mtim.Nano())
			newMtime := syscall.NsecToTimeval(oldStat.Mtim.Nano())
			if oldStat.Mode != newStat.Mode ||
				oldStat.Uid != newStat.Uid ||
				oldStat.Gid != newStat.Gid ||
				oldStat.Rdev != newStat.Rdev ||
				// Don't look at size for dirs, its not a good measure of change
				(oldStat.Size != newStat.Size && oldStat.Mode&syscall.S_IFDIR != syscall.S_IFDIR) ||
				oldMtime.Sec != newMtime.Sec ||
				oldMtime.Usec != newMtime.Usec {
				change := Change{
					Path: newChild.path(),
					Kind: ChangeModify,
				}
				*changes = append(*changes, change)
			}

			// Remove from copy so we can detect deletions
			delete(oldChildren, name)
		}

		newChild.addChanges(oldChild, changes)
	}
	for _, oldChild := range oldChildren {
		// delete
		change := Change{
			Path: oldChild.path(),
			Kind: ChangeDelete,
		}
		*changes = append(*changes, change)
	}

}
