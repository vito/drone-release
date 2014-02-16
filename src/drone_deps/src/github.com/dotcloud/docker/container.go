package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dotcloud/docker/archive"
	"github.com/dotcloud/docker/engine"
	"github.com/dotcloud/docker/execdriver"
	"github.com/dotcloud/docker/graphdriver"
	"github.com/dotcloud/docker/nat"
	"github.com/dotcloud/docker/pkg/mount"
	"github.com/dotcloud/docker/pkg/term"
	"github.com/dotcloud/docker/runconfig"
	"github.com/dotcloud/docker/utils"
	"github.com/kr/pty"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNotATTY = errors.New("The PTY is not a file")
	ErrNoTTY   = errors.New("No PTY found")
)

type Container struct {
	sync.Mutex
	root   string // Path to the "home" of the container, including metadata.
	basefs string // Path to the graphdriver mountpoint

	ID string

	Created time.Time

	Path string
	Args []string

	Config *runconfig.Config
	State  State
	Image  string

	NetworkSettings *NetworkSettings

	ResolvConfPath string
	HostnamePath   string
	HostsPath      string
	Name           string
	Driver         string

	command   *execdriver.Command
	stdout    *utils.WriteBroadcaster
	stderr    *utils.WriteBroadcaster
	stdin     io.ReadCloser
	stdinPipe io.WriteCloser
	ptyMaster io.Closer

	runtime *Runtime

	waitLock chan struct{}
	Volumes  map[string]string
	// Store rw/ro in a separate structure to preserve reverse-compatibility on-disk.
	// Easier than migrating older container configs :)
	VolumesRW  map[string]bool
	hostConfig *runconfig.HostConfig

	activeLinks map[string]*Link
}

type BindMap struct {
	SrcPath string
	DstPath string
	Mode    string
}

var (
	ErrContainerStart        = errors.New("The container failed to start. Unknown error")
	ErrContainerStartTimeout = errors.New("The container failed to start due to timed out.")
)

// FIXME: move deprecated port stuff to nat to clean up the core.
type PortMapping map[string]string // Deprecated

type NetworkSettings struct {
	IPAddress   string
	IPPrefixLen int
	Gateway     string
	Bridge      string
	PortMapping map[string]PortMapping // Deprecated
	Ports       nat.PortMap
}

func (settings *NetworkSettings) PortMappingAPI() *engine.Table {
	var outs = engine.NewTable("", 0)
	for port, bindings := range settings.Ports {
		p, _ := nat.ParsePort(port.Port())
		if len(bindings) == 0 {
			out := &engine.Env{}
			out.SetInt("PublicPort", p)
			out.Set("Type", port.Proto())
			outs.Add(out)
			continue
		}
		for _, binding := range bindings {
			out := &engine.Env{}
			h, _ := nat.ParsePort(binding.HostPort)
			out.SetInt("PrivatePort", p)
			out.SetInt("PublicPort", h)
			out.Set("Type", port.Proto())
			out.Set("IP", binding.HostIp)
			outs.Add(out)
		}
	}
	return outs
}

// Inject the io.Reader at the given path. Note: do not close the reader
func (container *Container) Inject(file io.Reader, pth string) error {
	if err := container.Mount(); err != nil {
		return fmt.Errorf("inject: error mounting container %s: %s", container.ID, err)
	}
	defer container.Unmount()

	// Return error if path exists
	destPath := path.Join(container.basefs, pth)
	if _, err := os.Stat(destPath); err == nil {
		// Since err is nil, the path could be stat'd and it exists
		return fmt.Errorf("%s exists", pth)
	} else if !os.IsNotExist(err) {
		// Expect err might be that the file doesn't exist, so
		// if it's some other error, return that.

		return err
	}

	// Make sure the directory exists
	if err := os.MkdirAll(path.Join(container.basefs, path.Dir(pth)), 0755); err != nil {
		return err
	}

	dest, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer dest.Close()

	if _, err := io.Copy(dest, file); err != nil {
		return err
	}
	return nil
}

func (container *Container) When() time.Time {
	return container.Created
}

func (container *Container) FromDisk() error {
	data, err := ioutil.ReadFile(container.jsonPath())
	if err != nil {
		return err
	}
	// Load container settings
	// udp broke compat of docker.PortMapping, but it's not used when loading a container, we can skip it
	if err := json.Unmarshal(data, container); err != nil && !strings.Contains(err.Error(), "docker.PortMapping") {
		return err
	}
	return container.readHostConfig()
}

func (container *Container) ToDisk() (err error) {
	data, err := json.Marshal(container)
	if err != nil {
		return
	}
	err = ioutil.WriteFile(container.jsonPath(), data, 0666)
	if err != nil {
		return
	}
	return container.writeHostConfig()
}

func (container *Container) readHostConfig() error {
	container.hostConfig = &runconfig.HostConfig{}
	// If the hostconfig file does not exist, do not read it.
	// (We still have to initialize container.hostConfig,
	// but that's OK, since we just did that above.)
	_, err := os.Stat(container.hostConfigPath())
	if os.IsNotExist(err) {
		return nil
	}
	data, err := ioutil.ReadFile(container.hostConfigPath())
	if err != nil {
		return err
	}
	return json.Unmarshal(data, container.hostConfig)
}

func (container *Container) writeHostConfig() (err error) {
	data, err := json.Marshal(container.hostConfig)
	if err != nil {
		return
	}
	return ioutil.WriteFile(container.hostConfigPath(), data, 0666)
}

func (container *Container) generateEnvConfig(env []string) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	p, err := container.EnvConfigPath()
	if err != nil {
		return err
	}
	ioutil.WriteFile(p, data, 0600)
	return nil
}

func (container *Container) setupPty() error {
	ptyMaster, ptySlave, err := pty.Open()
	if err != nil {
		return err
	}
	container.ptyMaster = ptyMaster
	container.command.Stdout = ptySlave
	container.command.Stderr = ptySlave
	container.command.Console = ptySlave.Name()

	// Copy the PTYs to our broadcasters
	go func() {
		defer container.stdout.CloseWriters()
		utils.Debugf("startPty: begin of stdout pipe")
		io.Copy(container.stdout, ptyMaster)
		utils.Debugf("startPty: end of stdout pipe")
	}()

	// stdin
	if container.Config.OpenStdin {
		container.command.Stdin = ptySlave
		container.command.SysProcAttr.Setctty = true
		go func() {
			defer container.stdin.Close()
			utils.Debugf("startPty: begin of stdin pipe")
			io.Copy(ptyMaster, container.stdin)
			utils.Debugf("startPty: end of stdin pipe")
		}()
	}
	return nil
}

func (container *Container) setupStd() error {
	container.command.Stdout = container.stdout
	container.command.Stderr = container.stderr
	if container.Config.OpenStdin {
		stdin, err := container.command.StdinPipe()
		if err != nil {
			return err
		}
		go func() {
			defer stdin.Close()
			utils.Debugf("start: begin of stdin pipe")
			io.Copy(stdin, container.stdin)
			utils.Debugf("start: end of stdin pipe")
		}()
	}
	return nil
}

func (container *Container) Attach(stdin io.ReadCloser, stdinCloser io.Closer, stdout io.Writer, stderr io.Writer) chan error {
	var cStdout, cStderr io.ReadCloser

	var nJobs int
	errors := make(chan error, 3)
	if stdin != nil && container.Config.OpenStdin {
		nJobs += 1
		if cStdin, err := container.StdinPipe(); err != nil {
			errors <- err
		} else {
			go func() {
				utils.Debugf("attach: stdin: begin")
				defer utils.Debugf("attach: stdin: end")
				// No matter what, when stdin is closed (io.Copy unblock), close stdout and stderr
				if container.Config.StdinOnce && !container.Config.Tty {
					defer cStdin.Close()
				} else {
					defer func() {
						if cStdout != nil {
							cStdout.Close()
						}
						if cStderr != nil {
							cStderr.Close()
						}
					}()
				}
				if container.Config.Tty {
					_, err = utils.CopyEscapable(cStdin, stdin)
				} else {
					_, err = io.Copy(cStdin, stdin)
				}
				if err == io.ErrClosedPipe {
					err = nil
				}
				if err != nil {
					utils.Errorf("attach: stdin: %s", err)
				}
				errors <- err
			}()
		}
	}
	if stdout != nil {
		nJobs += 1
		if p, err := container.StdoutPipe(); err != nil {
			errors <- err
		} else {
			cStdout = p
			go func() {
				utils.Debugf("attach: stdout: begin")
				defer utils.Debugf("attach: stdout: end")
				// If we are in StdinOnce mode, then close stdin
				if container.Config.StdinOnce && stdin != nil {
					defer stdin.Close()
				}
				if stdinCloser != nil {
					defer stdinCloser.Close()
				}
				_, err := io.Copy(stdout, cStdout)
				if err == io.ErrClosedPipe {
					err = nil
				}
				if err != nil {
					utils.Errorf("attach: stdout: %s", err)
				}
				errors <- err
			}()
		}
	} else {
		go func() {
			if stdinCloser != nil {
				defer stdinCloser.Close()
			}
			if cStdout, err := container.StdoutPipe(); err != nil {
				utils.Errorf("attach: stdout pipe: %s", err)
			} else {
				io.Copy(&utils.NopWriter{}, cStdout)
			}
		}()
	}
	if stderr != nil {
		nJobs += 1
		if p, err := container.StderrPipe(); err != nil {
			errors <- err
		} else {
			cStderr = p
			go func() {
				utils.Debugf("attach: stderr: begin")
				defer utils.Debugf("attach: stderr: end")
				// If we are in StdinOnce mode, then close stdin
				if container.Config.StdinOnce && stdin != nil {
					defer stdin.Close()
				}
				if stdinCloser != nil {
					defer stdinCloser.Close()
				}
				_, err := io.Copy(stderr, cStderr)
				if err == io.ErrClosedPipe {
					err = nil
				}
				if err != nil {
					utils.Errorf("attach: stderr: %s", err)
				}
				errors <- err
			}()
		}
	} else {
		go func() {
			if stdinCloser != nil {
				defer stdinCloser.Close()
			}

			if cStderr, err := container.StderrPipe(); err != nil {
				utils.Errorf("attach: stdout pipe: %s", err)
			} else {
				io.Copy(&utils.NopWriter{}, cStderr)
			}
		}()
	}

	return utils.Go(func() error {
		defer func() {
			if cStdout != nil {
				cStdout.Close()
			}
			if cStderr != nil {
				cStderr.Close()
			}
		}()

		// FIXME: how to clean up the stdin goroutine without the unwanted side effect
		// of closing the passed stdin? Add an intermediary io.Pipe?
		for i := 0; i < nJobs; i += 1 {
			utils.Debugf("attach: waiting for job %d/%d", i+1, nJobs)
			if err := <-errors; err != nil {
				utils.Errorf("attach: job %d returned error %s, aborting all jobs", i+1, err)
				return err
			}
			utils.Debugf("attach: job %d completed successfully", i+1)
		}
		utils.Debugf("attach: all jobs completed successfully")
		return nil
	})
}

func populateCommand(c *Container) {
	var (
		en           *execdriver.Network
		driverConfig []string
	)

	if !c.Config.NetworkDisabled {
		network := c.NetworkSettings
		en = &execdriver.Network{
			Gateway:     network.Gateway,
			Bridge:      network.Bridge,
			IPAddress:   network.IPAddress,
			IPPrefixLen: network.IPPrefixLen,
			Mtu:         c.runtime.config.Mtu,
		}
	}

	if lxcConf := c.hostConfig.LxcConf; lxcConf != nil {
		for _, pair := range lxcConf {
			driverConfig = append(driverConfig, fmt.Sprintf("%s = %s", pair.Key, pair.Value))
		}
	}
	resources := &execdriver.Resources{
		Memory:     c.Config.Memory,
		MemorySwap: c.Config.MemorySwap,
		CpuShares:  c.Config.CpuShares,
	}
	c.command = &execdriver.Command{
		ID:         c.ID,
		Privileged: c.hostConfig.Privileged,
		Rootfs:     c.RootfsPath(),
		InitPath:   "/.dockerinit",
		Entrypoint: c.Path,
		Arguments:  c.Args,
		WorkingDir: c.Config.WorkingDir,
		Network:    en,
		Tty:        c.Config.Tty,
		User:       c.Config.User,
		Config:     driverConfig,
		Resources:  resources,
	}
	c.command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func (container *Container) Start() (err error) {
	container.Lock()
	defer container.Unlock()

	if container.State.IsRunning() {
		return fmt.Errorf("The container %s is already running.", container.ID)
	}

	defer func() {
		if err != nil {
			container.cleanup()
		}
	}()

	if err := container.Mount(); err != nil {
		return err
	}

	if container.runtime.config.DisableNetwork {
		container.Config.NetworkDisabled = true
		container.buildHostnameAndHostsFiles("127.0.1.1")
	} else {
		if err := container.allocateNetwork(); err != nil {
			return err
		}
		container.buildHostnameAndHostsFiles(container.NetworkSettings.IPAddress)
	}

	// Make sure the config is compatible with the current kernel
	if container.Config.Memory > 0 && !container.runtime.sysInfo.MemoryLimit {
		log.Printf("WARNING: Your kernel does not support memory limit capabilities. Limitation discarded.\n")
		container.Config.Memory = 0
	}
	if container.Config.Memory > 0 && !container.runtime.sysInfo.SwapLimit {
		log.Printf("WARNING: Your kernel does not support swap limit capabilities. Limitation discarded.\n")
		container.Config.MemorySwap = -1
	}

	if container.runtime.sysInfo.IPv4ForwardingDisabled {
		log.Printf("WARNING: IPv4 forwarding is disabled. Networking will not work")
	}

	if container.Volumes == nil || len(container.Volumes) == 0 {
		container.Volumes = make(map[string]string)
		container.VolumesRW = make(map[string]bool)
	}

	// Apply volumes from another container if requested
	if err := container.applyExternalVolumes(); err != nil {
		return err
	}

	if err := container.createVolumes(); err != nil {
		return err
	}

	// Setup environment
	env := []string{
		"HOME=/",
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOSTNAME=" + container.Config.Hostname,
	}

	if container.Config.Tty {
		env = append(env, "TERM=xterm")
	}

	// Init any links between the parent and children
	runtime := container.runtime

	children, err := runtime.Children(container.Name)
	if err != nil {
		return err
	}

	if len(children) > 0 {
		container.activeLinks = make(map[string]*Link, len(children))

		// If we encounter an error make sure that we rollback any network
		// config and ip table changes
		rollback := func() {
			for _, link := range container.activeLinks {
				link.Disable()
			}
			container.activeLinks = nil
		}

		for p, child := range children {
			link, err := NewLink(container, child, p, runtime.eng)
			if err != nil {
				rollback()
				return err
			}

			container.activeLinks[link.Alias()] = link
			if err := link.Enable(); err != nil {
				rollback()
				return err
			}

			for _, envVar := range link.ToEnv() {
				env = append(env, envVar)
			}
		}
	}

	for _, elem := range container.Config.Env {
		env = append(env, elem)
	}

	if err := container.generateEnvConfig(env); err != nil {
		return err
	}

	if container.Config.WorkingDir != "" {
		container.Config.WorkingDir = path.Clean(container.Config.WorkingDir)
		if err := os.MkdirAll(path.Join(container.basefs, container.Config.WorkingDir), 0755); err != nil {
			return nil
		}
	}

	envPath, err := container.EnvConfigPath()
	if err != nil {
		return err
	}

	// Setup the root fs as a bind mount of the base fs
	root := container.RootfsPath()
	if err := os.MkdirAll(root, 0755); err != nil && !os.IsExist(err) {
		return nil
	}

	// Create a bind mount of the base fs as a place where we can add mounts
	// without affecting the ability to access the base fs
	if err := mount.Mount(container.basefs, root, "none", "bind,rw"); err != nil {
		return err
	}

	// Make sure the root fs is private so the mounts here don't propagate to basefs
	if err := mount.ForceMount(root, root, "none", "private"); err != nil {
		return err
	}

	// Mount docker specific files into the containers root fs
	if err := mount.Mount(runtime.sysInitPath, path.Join(root, "/.dockerinit"), "none", "bind,ro"); err != nil {
		return err
	}
	if err := mount.Mount(envPath, path.Join(root, "/.dockerenv"), "none", "bind,ro"); err != nil {
		return err
	}
	if err := mount.Mount(container.ResolvConfPath, path.Join(root, "/etc/resolv.conf"), "none", "bind,ro"); err != nil {
		return err
	}

	if container.HostnamePath != "" && container.HostsPath != "" {
		if err := mount.Mount(container.HostnamePath, path.Join(root, "/etc/hostname"), "none", "bind,ro"); err != nil {
			return err
		}
		if err := mount.Mount(container.HostsPath, path.Join(root, "/etc/hosts"), "none", "bind,ro"); err != nil {
			return err
		}
	}

	// Mount user specified volumes
	for r, v := range container.Volumes {
		mountAs := "ro"
		if container.VolumesRW[r] {
			mountAs = "rw"
		}

		r = path.Join(root, r)
		if p, err := utils.FollowSymlinkInScope(r, root); err != nil {
			return err
		} else {
			r = p
		}

		if err := mount.Mount(v, r, "none", fmt.Sprintf("bind,%s", mountAs)); err != nil {
			return err
		}
	}

	populateCommand(container)

	// Setup logging of stdout and stderr to disk
	if err := container.runtime.LogToDisk(container.stdout, container.logPath("json"), "stdout"); err != nil {
		return err
	}
	if err := container.runtime.LogToDisk(container.stderr, container.logPath("json"), "stderr"); err != nil {
		return err
	}
	container.waitLock = make(chan struct{})

	// Setuping pipes and/or Pty
	var setup func() error
	if container.Config.Tty {
		setup = container.setupPty
	} else {
		setup = container.setupStd
	}
	if err := setup(); err != nil {
		return err
	}

	callbackLock := make(chan struct{})
	callback := func(command *execdriver.Command) {
		container.State.SetRunning(command.Pid())
		if command.Tty {
			// The callback is called after the process Start()
			// so we are in the parent process. In TTY mode, stdin/out/err is the PtySlace
			// which we close here.
			if c, ok := command.Stdout.(io.Closer); ok {
				c.Close()
			}
		}
		if err := container.ToDisk(); err != nil {
			utils.Debugf("%s", err)
		}
		close(callbackLock)
	}

	// We use a callback here instead of a goroutine and an chan for
	// syncronization purposes
	cErr := utils.Go(func() error { return container.monitor(callback) })

	// Start should not return until the process is actually running
	select {
	case <-callbackLock:
	case err := <-cErr:
		return err
	}
	return nil
}

func (container *Container) getBindMap() (map[string]BindMap, error) {
	// Create the requested bind mounts
	binds := make(map[string]BindMap)
	// Define illegal container destinations
	illegalDsts := []string{"/", "."}

	for _, bind := range container.hostConfig.Binds {
		// FIXME: factorize bind parsing in parseBind
		var src, dst, mode string
		arr := strings.Split(bind, ":")
		if len(arr) == 2 {
			src = arr[0]
			dst = arr[1]
			mode = "rw"
		} else if len(arr) == 3 {
			src = arr[0]
			dst = arr[1]
			mode = arr[2]
		} else {
			return nil, fmt.Errorf("Invalid bind specification: %s", bind)
		}

		// Bail if trying to mount to an illegal destination
		for _, illegal := range illegalDsts {
			if dst == illegal {
				return nil, fmt.Errorf("Illegal bind destination: %s", dst)
			}
		}

		bindMap := BindMap{
			SrcPath: src,
			DstPath: dst,
			Mode:    mode,
		}
		binds[path.Clean(dst)] = bindMap
	}
	return binds, nil
}

func (container *Container) createVolumes() error {
	binds, err := container.getBindMap()
	if err != nil {
		return err
	}
	volumesDriver := container.runtime.volumes.driver
	// Create the requested volumes if they don't exist
	for volPath := range container.Config.Volumes {
		volPath = path.Clean(volPath)
		volIsDir := true
		// Skip existing volumes
		if _, exists := container.Volumes[volPath]; exists {
			continue
		}
		var srcPath string
		var isBindMount bool
		srcRW := false
		// If an external bind is defined for this volume, use that as a source
		if bindMap, exists := binds[volPath]; exists {
			isBindMount = true
			srcPath = bindMap.SrcPath
			if strings.ToLower(bindMap.Mode) == "rw" {
				srcRW = true
			}
			if stat, err := os.Stat(bindMap.SrcPath); err != nil {
				return err
			} else {
				volIsDir = stat.IsDir()
			}
			// Otherwise create an directory in $ROOT/volumes/ and use that
		} else {

			// Do not pass a container as the parameter for the volume creation.
			// The graph driver using the container's information ( Image ) to
			// create the parent.
			c, err := container.runtime.volumes.Create(nil, nil, "", "", nil)
			if err != nil {
				return err
			}
			srcPath, err = volumesDriver.Get(c.ID)
			if err != nil {
				return fmt.Errorf("Driver %s failed to get volume rootfs %s: %s", volumesDriver, c.ID, err)
			}
			srcRW = true // RW by default
		}

		if p, err := filepath.EvalSymlinks(srcPath); err != nil {
			return err
		} else {
			srcPath = p
		}

		container.Volumes[volPath] = srcPath
		container.VolumesRW[volPath] = srcRW

		// Create the mountpoint
		volPath = path.Join(container.basefs, volPath)
		rootVolPath, err := utils.FollowSymlinkInScope(volPath, container.basefs)
		if err != nil {
			return err
		}

		if _, err := os.Stat(rootVolPath); err != nil {
			if os.IsNotExist(err) {
				if volIsDir {
					if err := os.MkdirAll(rootVolPath, 0755); err != nil {
						return err
					}
				} else {
					if err := os.MkdirAll(path.Dir(rootVolPath), 0755); err != nil {
						return err
					}
					if f, err := os.OpenFile(rootVolPath, os.O_CREATE, 0755); err != nil {
						return err
					} else {
						f.Close()
					}
				}
			}
		}

		// Do not copy or change permissions if we are mounting from the host
		if srcRW && !isBindMount {
			volList, err := ioutil.ReadDir(rootVolPath)
			if err != nil {
				return err
			}
			if len(volList) > 0 {
				srcList, err := ioutil.ReadDir(srcPath)
				if err != nil {
					return err
				}
				if len(srcList) == 0 {
					// If the source volume is empty copy files from the root into the volume
					if err := archive.CopyWithTar(rootVolPath, srcPath); err != nil {
						return err
					}

					var stat syscall.Stat_t
					if err := syscall.Stat(rootVolPath, &stat); err != nil {
						return err
					}
					var srcStat syscall.Stat_t
					if err := syscall.Stat(srcPath, &srcStat); err != nil {
						return err
					}
					// Change the source volume's ownership if it differs from the root
					// files that were just copied
					if stat.Uid != srcStat.Uid || stat.Gid != srcStat.Gid {
						if err := os.Chown(srcPath, int(stat.Uid), int(stat.Gid)); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

func (container *Container) applyExternalVolumes() error {
	if container.Config.VolumesFrom != "" {
		containerSpecs := strings.Split(container.Config.VolumesFrom, ",")
		for _, containerSpec := range containerSpecs {
			mountRW := true
			specParts := strings.SplitN(containerSpec, ":", 2)
			switch len(specParts) {
			case 0:
				return fmt.Errorf("Malformed volumes-from specification: %s", container.Config.VolumesFrom)
			case 2:
				switch specParts[1] {
				case "ro":
					mountRW = false
				case "rw": // mountRW is already true
				default:
					return fmt.Errorf("Malformed volumes-from specification: %s", containerSpec)
				}
			}
			c := container.runtime.Get(specParts[0])
			if c == nil {
				return fmt.Errorf("Container %s not found. Impossible to mount its volumes", container.ID)
			}
			for volPath, id := range c.Volumes {
				if _, exists := container.Volumes[volPath]; exists {
					continue
				}
				if err := os.MkdirAll(path.Join(container.basefs, volPath), 0755); err != nil {
					return err
				}
				container.Volumes[volPath] = id
				if isRW, exists := c.VolumesRW[volPath]; exists {
					container.VolumesRW[volPath] = isRW && mountRW
				}
			}

		}
	}
	return nil
}

func (container *Container) Run() error {
	if err := container.Start(); err != nil {
		return err
	}
	container.Wait()
	return nil
}

func (container *Container) Output() (output []byte, err error) {
	pipe, err := container.StdoutPipe()
	if err != nil {
		return nil, err
	}
	defer pipe.Close()
	if err := container.Start(); err != nil {
		return nil, err
	}
	output, err = ioutil.ReadAll(pipe)
	container.Wait()
	return output, err
}

// Container.StdinPipe returns a WriteCloser which can be used to feed data
// to the standard input of the container's active process.
// Container.StdoutPipe and Container.StderrPipe each return a ReadCloser
// which can be used to retrieve the standard output (and error) generated
// by the container's active process. The output (and error) are actually
// copied and delivered to all StdoutPipe and StderrPipe consumers, using
// a kind of "broadcaster".

func (container *Container) StdinPipe() (io.WriteCloser, error) {
	return container.stdinPipe, nil
}

func (container *Container) StdoutPipe() (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	container.stdout.AddWriter(writer, "")
	return utils.NewBufReader(reader), nil
}

func (container *Container) StderrPipe() (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	container.stderr.AddWriter(writer, "")
	return utils.NewBufReader(reader), nil
}

func (container *Container) buildHostnameAndHostsFiles(IP string) {
	container.HostnamePath = path.Join(container.root, "hostname")
	ioutil.WriteFile(container.HostnamePath, []byte(container.Config.Hostname+"\n"), 0644)

	hostsContent := []byte(`
127.0.0.1	localhost
::1		localhost ip6-localhost ip6-loopback
fe00::0		ip6-localnet
ff00::0		ip6-mcastprefix
ff02::1		ip6-allnodes
ff02::2		ip6-allrouters
`)

	container.HostsPath = path.Join(container.root, "hosts")

	if container.Config.Domainname != "" {
		hostsContent = append([]byte(fmt.Sprintf("%s\t%s.%s %s\n", IP, container.Config.Hostname, container.Config.Domainname, container.Config.Hostname)), hostsContent...)
	} else if !container.Config.NetworkDisabled {
		hostsContent = append([]byte(fmt.Sprintf("%s\t%s\n", IP, container.Config.Hostname)), hostsContent...)
	}

	ioutil.WriteFile(container.HostsPath, hostsContent, 0644)
}

func (container *Container) allocateNetwork() error {
	if container.Config.NetworkDisabled {
		return nil
	}

	var (
		env *engine.Env
		err error
		eng = container.runtime.eng
	)

	if container.State.IsGhost() {
		if container.runtime.config.DisableNetwork {
			env = &engine.Env{}
		} else {
			currentIP := container.NetworkSettings.IPAddress

			job := eng.Job("allocate_interface", container.ID)
			if currentIP != "" {
				job.Setenv("RequestIP", currentIP)
			}

			env, err = job.Stdout.AddEnv()
			if err != nil {
				return err
			}

			if err := job.Run(); err != nil {
				return err
			}
		}
	} else {
		job := eng.Job("allocate_interface", container.ID)
		env, err = job.Stdout.AddEnv()
		if err != nil {
			return err
		}
		if err := job.Run(); err != nil {
			return err
		}
	}

	if container.Config.PortSpecs != nil {
		utils.Debugf("Migrating port mappings for container: %s", strings.Join(container.Config.PortSpecs, ", "))
		if err := migratePortMappings(container.Config, container.hostConfig); err != nil {
			return err
		}
		container.Config.PortSpecs = nil
		if err := container.writeHostConfig(); err != nil {
			return err
		}
	}

	var (
		portSpecs = make(nat.PortSet)
		bindings  = make(nat.PortMap)
	)

	if !container.State.IsGhost() {
		if container.Config.ExposedPorts != nil {
			portSpecs = container.Config.ExposedPorts
		}
		if container.hostConfig.PortBindings != nil {
			bindings = container.hostConfig.PortBindings
		}
	} else {
		if container.NetworkSettings.Ports != nil {
			for port, binding := range container.NetworkSettings.Ports {
				portSpecs[port] = struct{}{}
				bindings[port] = binding
			}
		}
	}

	container.NetworkSettings.PortMapping = nil

	for port := range portSpecs {
		binding := bindings[port]
		if container.hostConfig.PublishAllPorts && len(binding) == 0 {
			binding = append(binding, nat.PortBinding{})
		}

		for i := 0; i < len(binding); i++ {
			b := binding[i]

			portJob := eng.Job("allocate_port", container.ID)
			portJob.Setenv("HostIP", b.HostIp)
			portJob.Setenv("HostPort", b.HostPort)
			portJob.Setenv("Proto", port.Proto())
			portJob.Setenv("ContainerPort", port.Port())

			portEnv, err := portJob.Stdout.AddEnv()
			if err != nil {
				return err
			}
			if err := portJob.Run(); err != nil {
				eng.Job("release_interface", container.ID).Run()
				return err
			}
			b.HostIp = portEnv.Get("HostIP")
			b.HostPort = portEnv.Get("HostPort")

			binding[i] = b
		}
		bindings[port] = binding
	}
	container.writeHostConfig()

	container.NetworkSettings.Ports = bindings

	container.NetworkSettings.Bridge = env.Get("Bridge")
	container.NetworkSettings.IPAddress = env.Get("IP")
	container.NetworkSettings.IPPrefixLen = env.GetInt("IPPrefixLen")
	container.NetworkSettings.Gateway = env.Get("Gateway")

	return nil
}

func (container *Container) releaseNetwork() {
	if container.Config.NetworkDisabled {
		return
	}
	eng := container.runtime.eng

	eng.Job("release_interface", container.ID).Run()
	container.NetworkSettings = &NetworkSettings{}
}

func (container *Container) monitor(callback execdriver.StartCallback) error {
	var (
		err      error
		exitCode int
	)

	if container.command == nil {
		// This happends when you have a GHOST container with lxc
		populateCommand(container)
		err = container.runtime.RestoreCommand(container)
	} else {
		exitCode, err = container.runtime.Run(container, callback)
	}

	if err != nil {
		utils.Errorf("Error running container: %s", err)
	}

	// Cleanup
	container.cleanup()

	// Re-create a brand new stdin pipe once the container exited
	if container.Config.OpenStdin {
		container.stdin, container.stdinPipe = io.Pipe()
	}

	container.State.SetStopped(exitCode)

	if container.runtime != nil && container.runtime.srv != nil {
		container.runtime.srv.LogEvent("die", container.ID, container.runtime.repositories.ImageName(container.Image))
	}

	close(container.waitLock)

	// FIXME: there is a race condition here which causes this to fail during the unit tests.
	// If another goroutine was waiting for Wait() to return before removing the container's root
	// from the filesystem... At this point it may already have done so.
	// This is because State.setStopped() has already been called, and has caused Wait()
	// to return.
	// FIXME: why are we serializing running state to disk in the first place?
	//log.Printf("%s: Failed to dump configuration to the disk: %s", container.ID, err)
	container.ToDisk()

	return err
}

func (container *Container) cleanup() {
	container.releaseNetwork()

	// Disable all active links
	if container.activeLinks != nil {
		for _, link := range container.activeLinks {
			link.Disable()
		}
	}

	if container.Config.OpenStdin {
		if err := container.stdin.Close(); err != nil {
			utils.Errorf("%s: Error close stdin: %s", container.ID, err)
		}
	}
	if err := container.stdout.CloseWriters(); err != nil {
		utils.Errorf("%s: Error close stdout: %s", container.ID, err)
	}
	if err := container.stderr.CloseWriters(); err != nil {
		utils.Errorf("%s: Error close stderr: %s", container.ID, err)
	}

	if container.ptyMaster != nil {
		if err := container.ptyMaster.Close(); err != nil {
			utils.Errorf("%s: Error closing Pty master: %s", container.ID, err)
		}
	}

	var (
		root   = container.RootfsPath()
		mounts = []string{
			root,
			path.Join(root, "/.dockerinit"),
			path.Join(root, "/.dockerenv"),
			path.Join(root, "/etc/resolv.conf"),
		}
	)

	if container.HostnamePath != "" && container.HostsPath != "" {
		mounts = append(mounts, path.Join(root, "/etc/hostname"), path.Join(root, "/etc/hosts"))
	}

	for r := range container.Volumes {
		mounts = append(mounts, path.Join(root, r))
	}

	for i := len(mounts) - 1; i >= 0; i-- {
		if lastError := mount.Unmount(mounts[i]); lastError != nil {
			log.Printf("Failed to umount %v: %v", mounts[i], lastError)
		}
	}

	if err := container.Unmount(); err != nil {
		log.Printf("%v: Failed to umount filesystem: %v", container.ID, err)
	}
}

func (container *Container) kill(sig int) error {
	container.Lock()
	defer container.Unlock()

	if !container.State.IsRunning() {
		return nil
	}
	return container.runtime.Kill(container, sig)
}

func (container *Container) Kill() error {
	if !container.State.IsRunning() {
		return nil
	}

	// 1. Send SIGKILL
	if err := container.kill(9); err != nil {
		return err
	}

	// 2. Wait for the process to die, in last resort, try to kill the process directly
	if err := container.WaitTimeout(10 * time.Second); err != nil {
		if container.command == nil {
			return fmt.Errorf("lxc-kill failed, impossible to kill the container %s", utils.TruncateID(container.ID))
		}
		log.Printf("Container %s failed to exit within 10 seconds of lxc-kill %s - trying direct SIGKILL", "SIGKILL", utils.TruncateID(container.ID))
		if err := container.runtime.Kill(container, 9); err != nil {
			return err
		}
	}

	container.Wait()
	return nil
}

func (container *Container) Stop(seconds int) error {
	if !container.State.IsRunning() {
		return nil
	}

	// 1. Send a SIGTERM
	if err := container.kill(15); err != nil {
		utils.Debugf("Error sending kill SIGTERM: %s", err)
		log.Print("Failed to send SIGTERM to the process, force killing")
		if err := container.kill(9); err != nil {
			return err
		}
	}

	// 2. Wait for the process to exit on its own
	if err := container.WaitTimeout(time.Duration(seconds) * time.Second); err != nil {
		log.Printf("Container %v failed to exit within %d seconds of SIGTERM - using the force", container.ID, seconds)
		// 3. If it doesn't, then send SIGKILL
		if err := container.Kill(); err != nil {
			return err
		}
	}
	return nil
}

func (container *Container) Restart(seconds int) error {
	if err := container.Stop(seconds); err != nil {
		return err
	}
	return container.Start()
}

// Wait blocks until the container stops running, then returns its exit code.
func (container *Container) Wait() int {
	<-container.waitLock
	return container.State.GetExitCode()
}

func (container *Container) Resize(h, w int) error {
	pty, ok := container.ptyMaster.(*os.File)
	if !ok {
		return fmt.Errorf("ptyMaster does not have Fd() method")
	}
	return term.SetWinsize(pty.Fd(), &term.Winsize{Height: uint16(h), Width: uint16(w)})
}

func (container *Container) ExportRw() (archive.Archive, error) {
	if err := container.Mount(); err != nil {
		return nil, err
	}
	if container.runtime == nil {
		return nil, fmt.Errorf("Can't load storage driver for unregistered container %s", container.ID)
	}
	archive, err := container.runtime.Diff(container)
	if err != nil {
		container.Unmount()
		return nil, err
	}
	return utils.NewReadCloserWrapper(archive, func() error {
		err := archive.Close()
		container.Unmount()
		return err
	}), nil
}

func (container *Container) Export() (archive.Archive, error) {
	if err := container.Mount(); err != nil {
		return nil, err
	}

	archive, err := archive.Tar(container.basefs, archive.Uncompressed)
	if err != nil {
		container.Unmount()
		return nil, err
	}
	return utils.NewReadCloserWrapper(archive, func() error {
		err := archive.Close()
		container.Unmount()
		return err
	}), nil
}

func (container *Container) WaitTimeout(timeout time.Duration) error {
	done := make(chan bool)
	go func() {
		container.Wait()
		done <- true
	}()

	select {
	case <-time.After(timeout):
		return fmt.Errorf("Timed Out")
	case <-done:
		return nil
	}
}

func (container *Container) Mount() error {
	return container.runtime.Mount(container)
}

func (container *Container) Changes() ([]archive.Change, error) {
	return container.runtime.Changes(container)
}

func (container *Container) GetImage() (*Image, error) {
	if container.runtime == nil {
		return nil, fmt.Errorf("Can't get image of unregistered container")
	}
	return container.runtime.graph.Get(container.Image)
}

func (container *Container) Unmount() error {
	return container.runtime.Unmount(container)
}

func (container *Container) logPath(name string) string {
	return path.Join(container.root, fmt.Sprintf("%s-%s.log", container.ID, name))
}

func (container *Container) ReadLog(name string) (io.Reader, error) {
	return os.Open(container.logPath(name))
}

func (container *Container) hostConfigPath() string {
	return path.Join(container.root, "hostconfig.json")
}

func (container *Container) jsonPath() string {
	return path.Join(container.root, "config.json")
}

func (container *Container) EnvConfigPath() (string, error) {
	p := path.Join(container.root, "config.env")
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			f, err := os.Create(p)
			if err != nil {
				return "", err
			}
			f.Close()
		} else {
			return "", err
		}
	}
	return p, nil
}

// This method must be exported to be used from the lxc template
// This directory is only usable when the container is running
func (container *Container) RootfsPath() string {
	return path.Join(container.root, "root")
}

// This is the stand-alone version of the root fs, without any additional mounts.
// This directory is usable whenever the container is mounted (and not unmounted)
func (container *Container) BasefsPath() string {
	return container.basefs
}

func validateID(id string) error {
	if id == "" {
		return fmt.Errorf("Invalid empty id")
	}
	return nil
}

// GetSize, return real size, virtual size
func (container *Container) GetSize() (int64, int64) {
	var (
		sizeRw, sizeRootfs int64
		err                error
		driver             = container.runtime.driver
	)

	if err := container.Mount(); err != nil {
		utils.Errorf("Warning: failed to compute size of container rootfs %s: %s", container.ID, err)
		return sizeRw, sizeRootfs
	}
	defer container.Unmount()

	if differ, ok := container.runtime.driver.(graphdriver.Differ); ok {
		sizeRw, err = differ.DiffSize(container.ID)
		if err != nil {
			utils.Errorf("Warning: driver %s couldn't return diff size of container %s: %s", driver, container.ID, err)
			// FIXME: GetSize should return an error. Not changing it now in case
			// there is a side-effect.
			sizeRw = -1
		}
	} else {
		changes, _ := container.Changes()
		if changes != nil {
			sizeRw = archive.ChangesSize(container.basefs, changes)
		} else {
			sizeRw = -1
		}
	}

	if _, err = os.Stat(container.basefs); err != nil {
		if sizeRootfs, err = utils.TreeSize(container.basefs); err != nil {
			sizeRootfs = -1
		}
	}
	return sizeRw, sizeRootfs
}

func (container *Container) Copy(resource string) (io.ReadCloser, error) {
	if err := container.Mount(); err != nil {
		return nil, err
	}
	var filter []string
	basePath := path.Join(container.basefs, resource)
	stat, err := os.Stat(basePath)
	if err != nil {
		container.Unmount()
		return nil, err
	}
	if !stat.IsDir() {
		d, f := path.Split(basePath)
		basePath = d
		filter = []string{f}
	} else {
		filter = []string{path.Base(basePath)}
		basePath = path.Dir(basePath)
	}

	archive, err := archive.TarFilter(basePath, &archive.TarOptions{
		Compression: archive.Uncompressed,
		Includes:    filter,
	})
	if err != nil {
		return nil, err
	}
	return utils.NewReadCloserWrapper(archive, func() error {
		err := archive.Close()
		container.Unmount()
		return err
	}), nil
}

// Returns true if the container exposes a certain port
func (container *Container) Exposes(p nat.Port) bool {
	_, exists := container.Config.ExposedPorts[p]
	return exists
}

func (container *Container) GetPtyMaster() (*os.File, error) {
	if container.ptyMaster == nil {
		return nil, ErrNoTTY
	}
	if pty, ok := container.ptyMaster.(*os.File); ok {
		return pty, nil
	}
	return nil, ErrNotATTY
}
