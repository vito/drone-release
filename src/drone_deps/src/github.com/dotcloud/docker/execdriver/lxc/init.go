package lxc

import (
	"fmt"
	"github.com/dotcloud/docker/execdriver"
	"github.com/dotcloud/docker/pkg/netlink"
	"github.com/dotcloud/docker/pkg/user"
	"github.com/syndtr/gocapability/capability"
	"net"
	"os"
	"strings"
	"syscall"
)

func setupHostname(args *execdriver.InitArgs) error {
	hostname := getEnv(args, "HOSTNAME")
	if hostname == "" {
		return nil
	}
	return setHostname(hostname)
}

// Setup networking
func setupNetworking(args *execdriver.InitArgs) error {
	if args.Ip != "" {
		// eth0
		iface, err := net.InterfaceByName("eth0")
		if err != nil {
			return fmt.Errorf("Unable to set up networking: %v", err)
		}
		ip, ipNet, err := net.ParseCIDR(args.Ip)
		if err != nil {
			return fmt.Errorf("Unable to set up networking: %v", err)
		}
		if err := netlink.NetworkLinkAddIp(iface, ip, ipNet); err != nil {
			return fmt.Errorf("Unable to set up networking: %v", err)
		}
		if err := netlink.NetworkSetMTU(iface, args.Mtu); err != nil {
			return fmt.Errorf("Unable to set MTU: %v", err)
		}
		if err := netlink.NetworkLinkUp(iface); err != nil {
			return fmt.Errorf("Unable to set up networking: %v", err)
		}

		// loopback
		iface, err = net.InterfaceByName("lo")
		if err != nil {
			return fmt.Errorf("Unable to set up networking: %v", err)
		}
		if err := netlink.NetworkLinkUp(iface); err != nil {
			return fmt.Errorf("Unable to set up networking: %v", err)
		}
	}
	if args.Gateway != "" {
		gw := net.ParseIP(args.Gateway)
		if gw == nil {
			return fmt.Errorf("Unable to set up networking, %s is not a valid gateway IP", args.Gateway)
		}

		if err := netlink.AddDefaultGw(gw); err != nil {
			return fmt.Errorf("Unable to set up networking: %v", err)
		}
	}

	return nil
}

// Setup working directory
func setupWorkingDirectory(args *execdriver.InitArgs) error {
	if args.WorkDir == "" {
		return nil
	}
	if err := syscall.Chdir(args.WorkDir); err != nil {
		return fmt.Errorf("Unable to change dir to %v: %v", args.WorkDir, err)
	}
	return nil
}

// Takes care of dropping privileges to the desired user
func changeUser(args *execdriver.InitArgs) error {
	uid, gid, suppGids, err := user.GetUserGroupSupplementary(
		args.User,
		syscall.Getuid(), syscall.Getgid(),
	)
	if err != nil {
		return err
	}

	if err := syscall.Setgroups(suppGids); err != nil {
		return fmt.Errorf("Setgroups failed: %v", err)
	}
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("Setgid failed: %v", err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("Setuid failed: %v", err)
	}

	return nil
}

func setupCapabilities(args *execdriver.InitArgs) error {
	if args.Privileged {
		return nil
	}

	drop := []capability.Cap{
		capability.CAP_SETPCAP,
		capability.CAP_SYS_MODULE,
		capability.CAP_SYS_RAWIO,
		capability.CAP_SYS_PACCT,
		capability.CAP_SYS_ADMIN,
		capability.CAP_SYS_NICE,
		capability.CAP_SYS_RESOURCE,
		capability.CAP_SYS_TIME,
		capability.CAP_SYS_TTY_CONFIG,
		capability.CAP_MKNOD,
		capability.CAP_AUDIT_WRITE,
		capability.CAP_AUDIT_CONTROL,
		capability.CAP_MAC_OVERRIDE,
		capability.CAP_MAC_ADMIN,
		capability.CAP_NET_ADMIN,
	}

	c, err := capability.NewPid(os.Getpid())
	if err != nil {
		return err
	}

	c.Unset(capability.CAPS|capability.BOUNDS, drop...)

	if err := c.Apply(capability.CAPS | capability.BOUNDS); err != nil {
		return err
	}
	return nil
}

func getEnv(args *execdriver.InitArgs, key string) string {
	for _, kv := range args.Env {
		parts := strings.SplitN(kv, "=", 2)
		if parts[0] == key && len(parts) == 2 {
			return parts[1]
		}
	}
	return ""
}
