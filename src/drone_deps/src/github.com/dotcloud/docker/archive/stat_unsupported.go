// +build !linux

package archive

import "syscall"

func getLastAccess(stat *syscall.Stat_t) syscall.Timespec {
	return stat.Atimespec
}

func getLastModification(stat *syscall.Stat_t) syscall.Timespec {
	return stat.Mtimespec
}

func LUtimesNano(path string, ts []syscall.Timespec) error {
	return ErrNotImplemented
}

func UtimesNano(path string, ts []syscall.Timespec) error {
	return ErrNotImplemented
}
