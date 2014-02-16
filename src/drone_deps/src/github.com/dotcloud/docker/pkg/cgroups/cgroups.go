package cgroups

import (
	"bufio"
	"fmt"
	"github.com/dotcloud/docker/pkg/mount"
	"io"
	"os"
	"strings"
)

// https://www.kernel.org/doc/Documentation/cgroups/cgroups.txt
func FindCgroupMountpoint(subsystem string) (string, error) {
	mounts, err := mount.GetMounts()
	if err != nil {
		return "", err
	}

	for _, mount := range mounts {
		if mount.Fstype == "cgroup" {
			for _, opt := range strings.Split(mount.VfsOpts, ",") {
				if opt == subsystem {
					return mount.Mountpoint, nil
				}
			}
		}
	}

	return "", fmt.Errorf("cgroup mountpoint not found for %s", subsystem)
}

// Returns the relative path to the cgroup docker is running in.
func GetThisCgroupDir(subsystem string) (string, error) {
	f, err := os.Open("/proc/self/cgroup")
	if err != nil {
		return "", err
	}
	defer f.Close()

	return parseCgroupFile(subsystem, f)
}

func parseCgroupFile(subsystem string, r io.Reader) (string, error) {
	s := bufio.NewScanner(r)

	for s.Scan() {
		if err := s.Err(); err != nil {
			return "", err
		}
		text := s.Text()
		parts := strings.Split(text, ":")
		if parts[1] == subsystem {
			return parts[2], nil
		}
	}
	return "", fmt.Errorf("cgroup '%s' not found in /proc/self/cgroup", subsystem)
}
