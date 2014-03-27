package server

import (
	"encoding/json"
	"fmt"
	"github.com/dotcloud/docker/archive"
	"github.com/dotcloud/docker/daemonconfig"
	"github.com/dotcloud/docker/dockerversion"
	"github.com/dotcloud/docker/engine"
	"github.com/dotcloud/docker/graph"
	"github.com/dotcloud/docker/image"
	"github.com/dotcloud/docker/pkg/graphdb"
	"github.com/dotcloud/docker/pkg/signal"
	"github.com/dotcloud/docker/registry"
	"github.com/dotcloud/docker/runconfig"
	"github.com/dotcloud/docker/runtime"
	"github.com/dotcloud/docker/utils"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	gosignal "os/signal"
	"path"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// jobInitApi runs the remote api server `srv` as a daemon,
// Only one api server can run at the same time - this is enforced by a pidfile.
// The signals SIGINT, SIGQUIT and SIGTERM are intercepted for cleanup.
func InitServer(job *engine.Job) engine.Status {
	job.Logf("Creating server")
	srv, err := NewServer(job.Eng, daemonconfig.ConfigFromJob(job))
	if err != nil {
		return job.Error(err)
	}
	if srv.runtime.Config().Pidfile != "" {
		job.Logf("Creating pidfile")
		if err := utils.CreatePidFile(srv.runtime.Config().Pidfile); err != nil {
			// FIXME: do we need fatal here instead of returning a job error?
			log.Fatal(err)
		}
	}
	job.Logf("Setting up signal traps")
	c := make(chan os.Signal, 1)
	gosignal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		sig := <-c
		log.Printf("Received signal '%v', exiting\n", sig)
		utils.RemovePidFile(srv.runtime.Config().Pidfile)
		srv.Close()
		os.Exit(0)
	}()
	job.Eng.Hack_SetGlobalVar("httpapi.server", srv)
	job.Eng.Hack_SetGlobalVar("httpapi.runtime", srv.runtime)

	for name, handler := range map[string]engine.Handler{
		"export":           srv.ContainerExport,
		"create":           srv.ContainerCreate,
		"stop":             srv.ContainerStop,
		"restart":          srv.ContainerRestart,
		"start":            srv.ContainerStart,
		"kill":             srv.ContainerKill,
		"wait":             srv.ContainerWait,
		"tag":              srv.ImageTag,
		"resize":           srv.ContainerResize,
		"commit":           srv.ContainerCommit,
		"info":             srv.DockerInfo,
		"container_delete": srv.ContainerDestroy,
		"image_export":     srv.ImageExport,
		"images":           srv.Images,
		"history":          srv.ImageHistory,
		"viz":              srv.ImagesViz,
		"container_copy":   srv.ContainerCopy,
		"insert":           srv.ImageInsert,
		"attach":           srv.ContainerAttach,
		"search":           srv.ImagesSearch,
		"changes":          srv.ContainerChanges,
		"top":              srv.ContainerTop,
		"version":          srv.DockerVersion,
		"load":             srv.ImageLoad,
		"build":            srv.Build,
		"pull":             srv.ImagePull,
		"import":           srv.ImageImport,
		"image_delete":     srv.ImageDelete,
		"inspect":          srv.JobInspect,
		"events":           srv.Events,
		"push":             srv.ImagePush,
		"containers":       srv.Containers,
		"auth":             srv.Auth,
	} {
		if err := job.Eng.Register(name, handler); err != nil {
			return job.Error(err)
		}
	}
	return engine.StatusOK
}

// simpleVersionInfo is a simple implementation of
// the interface VersionInfo, which is used
// to provide version information for some product,
// component, etc. It stores the product name and the version
// in string and returns them on calls to Name() and Version().
type simpleVersionInfo struct {
	name    string
	version string
}

func (v *simpleVersionInfo) Name() string {
	return v.name
}

func (v *simpleVersionInfo) Version() string {
	return v.version
}

// ContainerKill send signal to the container
// If no signal is given (sig 0), then Kill with SIGKILL and wait
// for the container to exit.
// If a signal is given, then just send it to the container and return.
func (srv *Server) ContainerKill(job *engine.Job) engine.Status {
	if n := len(job.Args); n < 1 || n > 2 {
		return job.Errorf("Usage: %s CONTAINER [SIGNAL]", job.Name)
	}
	var (
		name = job.Args[0]
		sig  uint64
		err  error
	)

	// If we have a signal, look at it. Otherwise, do nothing
	if len(job.Args) == 2 && job.Args[1] != "" {
		// Check if we passed the signal as a number:
		// The largest legal signal is 31, so let's parse on 5 bits
		sig, err = strconv.ParseUint(job.Args[1], 10, 5)
		if err != nil {
			// The signal is not a number, treat it as a string
			sig = uint64(signal.SignalMap[job.Args[1]])
			if sig == 0 {
				return job.Errorf("Invalid signal: %s", job.Args[1])
			}

		}
	}

	if container := srv.runtime.Get(name); container != nil {
		// If no signal is passed, or SIGKILL, perform regular Kill (SIGKILL + wait())
		if sig == 0 || syscall.Signal(sig) == syscall.SIGKILL {
			if err := container.Kill(); err != nil {
				return job.Errorf("Cannot kill container %s: %s", name, err)
			}
			srv.LogEvent("kill", container.ID, srv.runtime.Repositories().ImageName(container.Image))
		} else {
			// Otherwise, just send the requested signal
			if err := container.KillSig(int(sig)); err != nil {
				return job.Errorf("Cannot kill container %s: %s", name, err)
			}
			// FIXME: Add event for signals
		}
	} else {
		return job.Errorf("No such container: %s", name)
	}
	return engine.StatusOK
}

func (srv *Server) Auth(job *engine.Job) engine.Status {
	var (
		err        error
		authConfig = &registry.AuthConfig{}
	)

	job.GetenvJson("authConfig", authConfig)
	// TODO: this is only done here because auth and registry need to be merged into one pkg
	if addr := authConfig.ServerAddress; addr != "" && addr != registry.IndexServerAddress() {
		addr, err = registry.ExpandAndVerifyRegistryUrl(addr)
		if err != nil {
			return job.Error(err)
		}
		authConfig.ServerAddress = addr
	}
	status, err := registry.Login(authConfig, srv.HTTPRequestFactory(nil))
	if err != nil {
		return job.Error(err)
	}
	job.Printf("%s\n", status)
	return engine.StatusOK
}

func (srv *Server) Events(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("Usage: %s FROM", job.Name)
	}

	var (
		from  = job.Args[0]
		since = job.GetenvInt64("since")
	)
	sendEvent := func(event *utils.JSONMessage) error {
		b, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("JSON error")
		}
		_, err = job.Stdout.Write(b)
		if err != nil {
			// On error, evict the listener
			utils.Errorf("%s", err)
			srv.Lock()
			delete(srv.listeners, from)
			srv.Unlock()
			return err
		}
		return nil
	}

	listener := make(chan utils.JSONMessage)
	srv.Lock()
	if old, ok := srv.listeners[from]; ok {
		delete(srv.listeners, from)
		close(old)
	}
	srv.listeners[from] = listener
	srv.Unlock()
	job.Stdout.Write(nil) // flush
	if since != 0 {
		// If since, send previous events that happened after the timestamp
		for _, event := range srv.GetEvents() {
			if event.Time >= since {
				err := sendEvent(&event)
				if err != nil && err.Error() == "JSON error" {
					continue
				}
				if err != nil {
					job.Error(err)
					return engine.StatusErr
				}
			}
		}
	}
	for event := range listener {
		err := sendEvent(&event)
		if err != nil && err.Error() == "JSON error" {
			continue
		}
		if err != nil {
			return job.Error(err)
		}
	}
	return engine.StatusOK
}

func (srv *Server) ContainerExport(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("Usage: %s container_id", job.Name)
	}
	name := job.Args[0]
	if container := srv.runtime.Get(name); container != nil {
		data, err := container.Export()
		if err != nil {
			return job.Errorf("%s: %s", name, err)
		}
		defer data.Close()

		// Stream the entire contents of the container (basically a volatile snapshot)
		if _, err := io.Copy(job.Stdout, data); err != nil {
			return job.Errorf("%s: %s", name, err)
		}
		// FIXME: factor job-specific LogEvent to engine.Job.Run()
		srv.LogEvent("export", container.ID, srv.runtime.Repositories().ImageName(container.Image))
		return engine.StatusOK
	}
	return job.Errorf("No such container: %s", name)
}

// ImageExport exports all images with the given tag. All versions
// containing the same tag are exported. The resulting output is an
// uncompressed tar ball.
// name is the set of tags to export.
// out is the writer where the images are written to.
func (srv *Server) ImageExport(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("Usage: %s CONTAINER\n", job.Name)
	}
	name := job.Args[0]
	// get image json
	tempdir, err := ioutil.TempDir("", "docker-export-")
	if err != nil {
		return job.Error(err)
	}
	defer os.RemoveAll(tempdir)

	utils.Debugf("Serializing %s", name)

	rootRepo, err := srv.runtime.Repositories().Get(name)
	if err != nil {
		return job.Error(err)
	}
	if rootRepo != nil {
		for _, id := range rootRepo {
			image, err := srv.ImageInspect(id)
			if err != nil {
				return job.Error(err)
			}

			if err := srv.exportImage(image, tempdir); err != nil {
				return job.Error(err)
			}
		}

		// write repositories
		rootRepoMap := map[string]graph.Repository{}
		rootRepoMap[name] = rootRepo
		rootRepoJson, _ := json.Marshal(rootRepoMap)

		if err := ioutil.WriteFile(path.Join(tempdir, "repositories"), rootRepoJson, os.ModeAppend); err != nil {
			return job.Error(err)
		}
	} else {
		image, err := srv.ImageInspect(name)
		if err != nil {
			return job.Error(err)
		}
		if err := srv.exportImage(image, tempdir); err != nil {
			return job.Error(err)
		}
	}

	fs, err := archive.Tar(tempdir, archive.Uncompressed)
	if err != nil {
		return job.Error(err)
	}
	defer fs.Close()

	if _, err := io.Copy(job.Stdout, fs); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (srv *Server) exportImage(img *image.Image, tempdir string) error {
	for i := img; i != nil; {
		// temporary directory
		tmpImageDir := path.Join(tempdir, i.ID)
		if err := os.Mkdir(tmpImageDir, os.ModeDir); err != nil {
			if os.IsExist(err) {
				return nil
			}
			return err
		}

		var version = "1.0"
		var versionBuf = []byte(version)

		if err := ioutil.WriteFile(path.Join(tmpImageDir, "VERSION"), versionBuf, os.ModeAppend); err != nil {
			return err
		}

		// serialize json
		b, err := json.Marshal(i)
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(path.Join(tmpImageDir, "json"), b, os.ModeAppend); err != nil {
			return err
		}

		// serialize filesystem
		fs, err := i.TarLayer()
		if err != nil {
			return err
		}
		defer fs.Close()

		fsTar, err := os.Create(path.Join(tmpImageDir, "layer.tar"))
		if err != nil {
			return err
		}
		if written, err := io.Copy(fsTar, fs); err != nil {
			return err
		} else {
			utils.Debugf("rendered layer for %s of [%d] size", i.ID, written)
		}

		if err = fsTar.Close(); err != nil {
			return err
		}

		// find parent
		if i.Parent != "" {
			i, err = srv.ImageInspect(i.Parent)
			if err != nil {
				return err
			}
		} else {
			i = nil
		}
	}
	return nil
}

func (srv *Server) Build(job *engine.Job) engine.Status {
	if len(job.Args) != 0 {
		return job.Errorf("Usage: %s\n", job.Name)
	}
	var (
		remoteURL      = job.Getenv("remote")
		repoName       = job.Getenv("t")
		suppressOutput = job.GetenvBool("q")
		noCache        = job.GetenvBool("nocache")
		rm             = job.GetenvBool("rm")
		authConfig     = &registry.AuthConfig{}
		configFile     = &registry.ConfigFile{}
		tag            string
		context        io.ReadCloser
	)
	job.GetenvJson("authConfig", authConfig)
	job.GetenvJson("configFile", configFile)
	repoName, tag = utils.ParseRepositoryTag(repoName)

	if remoteURL == "" {
		context = ioutil.NopCloser(job.Stdin)
	} else if utils.IsGIT(remoteURL) {
		if !strings.HasPrefix(remoteURL, "git://") {
			remoteURL = "https://" + remoteURL
		}
		root, err := ioutil.TempDir("", "docker-build-git")
		if err != nil {
			return job.Error(err)
		}
		defer os.RemoveAll(root)

		if output, err := exec.Command("git", "clone", "--recursive", remoteURL, root).CombinedOutput(); err != nil {
			return job.Errorf("Error trying to use git: %s (%s)", err, output)
		}

		c, err := archive.Tar(root, archive.Uncompressed)
		if err != nil {
			return job.Error(err)
		}
		context = c
	} else if utils.IsURL(remoteURL) {
		f, err := utils.Download(remoteURL)
		if err != nil {
			return job.Error(err)
		}
		defer f.Body.Close()
		dockerFile, err := ioutil.ReadAll(f.Body)
		if err != nil {
			return job.Error(err)
		}
		c, err := archive.Generate("Dockerfile", string(dockerFile))
		if err != nil {
			return job.Error(err)
		}
		context = c
	}
	defer context.Close()

	sf := utils.NewStreamFormatter(job.GetenvBool("json"))
	b := NewBuildFile(srv,
		&utils.StdoutFormater{
			Writer:          job.Stdout,
			StreamFormatter: sf,
		},
		&utils.StderrFormater{
			Writer:          job.Stdout,
			StreamFormatter: sf,
		},
		!suppressOutput, !noCache, rm, job.Stdout, sf, authConfig, configFile)
	id, err := b.Build(context)
	if err != nil {
		return job.Error(err)
	}
	if repoName != "" {
		srv.runtime.Repositories().Set(repoName, tag, id, false)
	}
	return engine.StatusOK
}

// Loads a set of images into the repository. This is the complementary of ImageExport.
// The input stream is an uncompressed tar ball containing images and metadata.
func (srv *Server) ImageLoad(job *engine.Job) engine.Status {
	tmpImageDir, err := ioutil.TempDir("", "docker-import-")
	if err != nil {
		return job.Error(err)
	}
	defer os.RemoveAll(tmpImageDir)

	var (
		repoTarFile = path.Join(tmpImageDir, "repo.tar")
		repoDir     = path.Join(tmpImageDir, "repo")
	)

	tarFile, err := os.Create(repoTarFile)
	if err != nil {
		return job.Error(err)
	}
	if _, err := io.Copy(tarFile, job.Stdin); err != nil {
		return job.Error(err)
	}
	tarFile.Close()

	repoFile, err := os.Open(repoTarFile)
	if err != nil {
		return job.Error(err)
	}
	if err := os.Mkdir(repoDir, os.ModeDir); err != nil {
		return job.Error(err)
	}
	if err := archive.Untar(repoFile, repoDir, nil); err != nil {
		return job.Error(err)
	}

	dirs, err := ioutil.ReadDir(repoDir)
	if err != nil {
		return job.Error(err)
	}

	for _, d := range dirs {
		if d.IsDir() {
			if err := srv.recursiveLoad(d.Name(), tmpImageDir); err != nil {
				return job.Error(err)
			}
		}
	}

	repositoriesJson, err := ioutil.ReadFile(path.Join(tmpImageDir, "repo", "repositories"))
	if err == nil {
		repositories := map[string]graph.Repository{}
		if err := json.Unmarshal(repositoriesJson, &repositories); err != nil {
			return job.Error(err)
		}

		for imageName, tagMap := range repositories {
			for tag, address := range tagMap {
				if err := srv.runtime.Repositories().Set(imageName, tag, address, true); err != nil {
					return job.Error(err)
				}
			}
		}
	} else if !os.IsNotExist(err) {
		return job.Error(err)
	}

	return engine.StatusOK
}

func (srv *Server) recursiveLoad(address, tmpImageDir string) error {
	if _, err := srv.ImageInspect(address); err != nil {
		utils.Debugf("Loading %s", address)

		imageJson, err := ioutil.ReadFile(path.Join(tmpImageDir, "repo", address, "json"))
		if err != nil {
			utils.Debugf("Error reading json", err)
			return err
		}

		layer, err := os.Open(path.Join(tmpImageDir, "repo", address, "layer.tar"))
		if err != nil {
			utils.Debugf("Error reading embedded tar", err)
			return err
		}
		img, err := image.NewImgJSON(imageJson)
		if err != nil {
			utils.Debugf("Error unmarshalling json", err)
			return err
		}
		if img.Parent != "" {
			if !srv.runtime.Graph().Exists(img.Parent) {
				if err := srv.recursiveLoad(img.Parent, tmpImageDir); err != nil {
					return err
				}
			}
		}
		if err := srv.runtime.Graph().Register(imageJson, layer, img); err != nil {
			return err
		}
	}
	utils.Debugf("Completed processing %s", address)

	return nil
}

func (srv *Server) ImagesSearch(job *engine.Job) engine.Status {
	if n := len(job.Args); n != 1 {
		return job.Errorf("Usage: %s TERM", job.Name)
	}
	var (
		term        = job.Args[0]
		metaHeaders = map[string][]string{}
		authConfig  = &registry.AuthConfig{}
	)
	job.GetenvJson("authConfig", authConfig)
	job.GetenvJson("metaHeaders", metaHeaders)

	r, err := registry.NewRegistry(authConfig, srv.HTTPRequestFactory(metaHeaders), registry.IndexServerAddress())
	if err != nil {
		return job.Error(err)
	}
	results, err := r.SearchRepositories(term)
	if err != nil {
		return job.Error(err)
	}
	outs := engine.NewTable("star_count", 0)
	for _, result := range results.Results {
		out := &engine.Env{}
		out.Import(result)
		outs.Add(out)
	}
	outs.ReverseSort()
	if _, err := outs.WriteListTo(job.Stdout); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (srv *Server) ImageInsert(job *engine.Job) engine.Status {
	if len(job.Args) != 3 {
		return job.Errorf("Usage: %s IMAGE URL PATH\n", job.Name)
	}

	var (
		name = job.Args[0]
		url  = job.Args[1]
		path = job.Args[2]
	)

	sf := utils.NewStreamFormatter(job.GetenvBool("json"))

	out := utils.NewWriteFlusher(job.Stdout)
	img, err := srv.runtime.Repositories().LookupImage(name)
	if err != nil {
		return job.Error(err)
	}

	file, err := utils.Download(url)
	if err != nil {
		return job.Error(err)
	}
	defer file.Body.Close()

	config, _, _, err := runconfig.Parse([]string{img.ID, "echo", "insert", url, path}, srv.runtime.SystemConfig())
	if err != nil {
		return job.Error(err)
	}

	c, _, err := srv.runtime.Create(config, "")
	if err != nil {
		return job.Error(err)
	}

	if err := c.Inject(utils.ProgressReader(file.Body, int(file.ContentLength), out, sf, false, utils.TruncateID(img.ID), "Downloading"), path); err != nil {
		return job.Error(err)
	}
	// FIXME: Handle custom repo, tag comment, author
	img, err = srv.runtime.Commit(c, "", "", img.Comment, img.Author, nil)
	if err != nil {
		out.Write(sf.FormatError(err))
		return engine.StatusErr
	}
	out.Write(sf.FormatStatus("", img.ID))
	return engine.StatusOK
}

func (srv *Server) ImagesViz(job *engine.Job) engine.Status {
	images, _ := srv.runtime.Graph().Map()
	if images == nil {
		return engine.StatusOK
	}
	job.Stdout.Write([]byte("digraph docker {\n"))

	var (
		parentImage *image.Image
		err         error
	)
	for _, image := range images {
		parentImage, err = image.GetParent()
		if err != nil {
			return job.Errorf("Error while getting parent image: %v", err)
		}
		if parentImage != nil {
			job.Stdout.Write([]byte(" \"" + parentImage.ID + "\" -> \"" + image.ID + "\"\n"))
		} else {
			job.Stdout.Write([]byte(" base -> \"" + image.ID + "\" [style=invis]\n"))
		}
	}

	reporefs := make(map[string][]string)

	for name, repository := range srv.runtime.Repositories().Repositories {
		for tag, id := range repository {
			reporefs[utils.TruncateID(id)] = append(reporefs[utils.TruncateID(id)], fmt.Sprintf("%s:%s", name, tag))
		}
	}

	for id, repos := range reporefs {
		job.Stdout.Write([]byte(" \"" + id + "\" [label=\"" + id + "\\n" + strings.Join(repos, "\\n") + "\",shape=box,fillcolor=\"paleturquoise\",style=\"filled,rounded\"];\n"))
	}
	job.Stdout.Write([]byte(" base [style=invisible]\n}\n"))
	return engine.StatusOK
}

func (srv *Server) Images(job *engine.Job) engine.Status {
	var (
		allImages map[string]*image.Image
		err       error
	)
	if job.GetenvBool("all") {
		allImages, err = srv.runtime.Graph().Map()
	} else {
		allImages, err = srv.runtime.Graph().Heads()
	}
	if err != nil {
		return job.Error(err)
	}
	lookup := make(map[string]*engine.Env)
	for name, repository := range srv.runtime.Repositories().Repositories {
		if job.Getenv("filter") != "" {
			if match, _ := path.Match(job.Getenv("filter"), name); !match {
				continue
			}
		}
		for tag, id := range repository {
			image, err := srv.runtime.Graph().Get(id)
			if err != nil {
				log.Printf("Warning: couldn't load %s from %s/%s: %s", id, name, tag, err)
				continue
			}

			if out, exists := lookup[id]; exists {
				out.SetList("RepoTags", append(out.GetList("RepoTags"), fmt.Sprintf("%s:%s", name, tag)))
			} else {
				out := &engine.Env{}
				delete(allImages, id)
				out.Set("ParentId", image.Parent)
				out.SetList("RepoTags", []string{fmt.Sprintf("%s:%s", name, tag)})
				out.Set("Id", image.ID)
				out.SetInt64("Created", image.Created.Unix())
				out.SetInt64("Size", image.Size)
				out.SetInt64("VirtualSize", image.GetParentsSize(0)+image.Size)
				lookup[id] = out
			}

		}
	}

	outs := engine.NewTable("Created", len(lookup))
	for _, value := range lookup {
		outs.Add(value)
	}

	// Display images which aren't part of a repository/tag
	if job.Getenv("filter") == "" {
		for _, image := range allImages {
			out := &engine.Env{}
			out.Set("ParentId", image.Parent)
			out.SetList("RepoTags", []string{"<none>:<none>"})
			out.Set("Id", image.ID)
			out.SetInt64("Created", image.Created.Unix())
			out.SetInt64("Size", image.Size)
			out.SetInt64("VirtualSize", image.GetParentsSize(0)+image.Size)
			outs.Add(out)
		}
	}

	outs.ReverseSort()
	if _, err := outs.WriteListTo(job.Stdout); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (srv *Server) DockerInfo(job *engine.Job) engine.Status {
	images, _ := srv.runtime.Graph().Map()
	var imgcount int
	if images == nil {
		imgcount = 0
	} else {
		imgcount = len(images)
	}
	kernelVersion := "<unknown>"
	if kv, err := utils.GetKernelVersion(); err == nil {
		kernelVersion = kv.String()
	}

	// if we still have the original dockerinit binary from before we copied it locally, let's return the path to that, since that's more intuitive (the copied path is trivial to derive by hand given VERSION)
	initPath := utils.DockerInitPath("")
	if initPath == "" {
		// if that fails, we'll just return the path from the runtime
		initPath = srv.runtime.SystemInitPath()
	}

	v := &engine.Env{}
	v.SetInt("Containers", len(srv.runtime.List()))
	v.SetInt("Images", imgcount)
	v.Set("Driver", srv.runtime.GraphDriver().String())
	v.SetJson("DriverStatus", srv.runtime.GraphDriver().Status())
	v.SetBool("MemoryLimit", srv.runtime.SystemConfig().MemoryLimit)
	v.SetBool("SwapLimit", srv.runtime.SystemConfig().SwapLimit)
	v.SetBool("IPv4Forwarding", !srv.runtime.SystemConfig().IPv4ForwardingDisabled)
	v.SetBool("Debug", os.Getenv("DEBUG") != "")
	v.SetInt("NFd", utils.GetTotalUsedFds())
	v.SetInt("NGoroutines", goruntime.NumGoroutine())
	v.Set("ExecutionDriver", srv.runtime.ExecutionDriver().Name())
	v.SetInt("NEventsListener", len(srv.listeners))
	v.Set("KernelVersion", kernelVersion)
	v.Set("IndexServerAddress", registry.IndexServerAddress())
	v.Set("InitSha1", dockerversion.INITSHA1)
	v.Set("InitPath", initPath)
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (srv *Server) DockerVersion(job *engine.Job) engine.Status {
	v := &engine.Env{}
	v.Set("Version", dockerversion.VERSION)
	v.Set("GitCommit", dockerversion.GITCOMMIT)
	v.Set("GoVersion", goruntime.Version())
	v.Set("Os", goruntime.GOOS)
	v.Set("Arch", goruntime.GOARCH)
	if kernelVersion, err := utils.GetKernelVersion(); err == nil {
		v.Set("KernelVersion", kernelVersion.String())
	}
	if _, err := v.WriteTo(job.Stdout); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (srv *Server) ImageHistory(job *engine.Job) engine.Status {
	if n := len(job.Args); n != 1 {
		return job.Errorf("Usage: %s IMAGE", job.Name)
	}
	name := job.Args[0]
	foundImage, err := srv.runtime.Repositories().LookupImage(name)
	if err != nil {
		return job.Error(err)
	}

	lookupMap := make(map[string][]string)
	for name, repository := range srv.runtime.Repositories().Repositories {
		for tag, id := range repository {
			// If the ID already has a reverse lookup, do not update it unless for "latest"
			if _, exists := lookupMap[id]; !exists {
				lookupMap[id] = []string{}
			}
			lookupMap[id] = append(lookupMap[id], name+":"+tag)
		}
	}

	outs := engine.NewTable("Created", 0)
	err = foundImage.WalkHistory(func(img *image.Image) error {
		out := &engine.Env{}
		out.Set("Id", img.ID)
		out.SetInt64("Created", img.Created.Unix())
		out.Set("CreatedBy", strings.Join(img.ContainerConfig.Cmd, " "))
		out.SetList("Tags", lookupMap[img.ID])
		out.SetInt64("Size", img.Size)
		outs.Add(out)
		return nil
	})
	outs.ReverseSort()
	if _, err := outs.WriteListTo(job.Stdout); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (srv *Server) ContainerTop(job *engine.Job) engine.Status {
	if len(job.Args) != 1 && len(job.Args) != 2 {
		return job.Errorf("Not enough arguments. Usage: %s CONTAINER [PS_ARGS]\n", job.Name)
	}
	var (
		name   = job.Args[0]
		psArgs = "-ef"
	)

	if len(job.Args) == 2 && job.Args[1] != "" {
		psArgs = job.Args[1]
	}

	if container := srv.runtime.Get(name); container != nil {
		if !container.State.IsRunning() {
			return job.Errorf("Container %s is not running", name)
		}
		pids, err := srv.runtime.ExecutionDriver().GetPidsForContainer(container.ID)
		if err != nil {
			return job.Error(err)
		}
		output, err := exec.Command("ps", psArgs).Output()
		if err != nil {
			return job.Errorf("Error running ps: %s", err)
		}

		lines := strings.Split(string(output), "\n")
		header := strings.Fields(lines[0])
		out := &engine.Env{}
		out.SetList("Titles", header)

		pidIndex := -1
		for i, name := range header {
			if name == "PID" {
				pidIndex = i
			}
		}
		if pidIndex == -1 {
			return job.Errorf("Couldn't find PID field in ps output")
		}

		processes := [][]string{}
		for _, line := range lines[1:] {
			if len(line) == 0 {
				continue
			}
			fields := strings.Fields(line)
			p, err := strconv.Atoi(fields[pidIndex])
			if err != nil {
				return job.Errorf("Unexpected pid '%s': %s", fields[pidIndex], err)
			}

			for _, pid := range pids {
				if pid == p {
					// Make sure number of fields equals number of header titles
					// merging "overhanging" fields
					process := fields[:len(header)-1]
					process = append(process, strings.Join(fields[len(header)-1:], " "))
					processes = append(processes, process)
				}
			}
		}
		out.SetJson("Processes", processes)
		out.WriteTo(job.Stdout)
		return engine.StatusOK

	}
	return job.Errorf("No such container: %s", name)
}

func (srv *Server) ContainerChanges(job *engine.Job) engine.Status {
	if n := len(job.Args); n != 1 {
		return job.Errorf("Usage: %s CONTAINER", job.Name)
	}
	name := job.Args[0]
	if container := srv.runtime.Get(name); container != nil {
		outs := engine.NewTable("", 0)
		changes, err := container.Changes()
		if err != nil {
			return job.Error(err)
		}
		for _, change := range changes {
			out := &engine.Env{}
			if err := out.Import(change); err != nil {
				return job.Error(err)
			}
			outs.Add(out)
		}
		if _, err := outs.WriteListTo(job.Stdout); err != nil {
			return job.Error(err)
		}
	} else {
		return job.Errorf("No such container: %s", name)
	}
	return engine.StatusOK
}

func (srv *Server) Containers(job *engine.Job) engine.Status {
	var (
		foundBefore bool
		displayed   int
		all         = job.GetenvBool("all")
		since       = job.Getenv("since")
		before      = job.Getenv("before")
		n           = job.GetenvInt("limit")
		size        = job.GetenvBool("size")
	)
	outs := engine.NewTable("Created", 0)

	names := map[string][]string{}
	srv.runtime.ContainerGraph().Walk("/", func(p string, e *graphdb.Entity) error {
		names[e.ID()] = append(names[e.ID()], p)
		return nil
	}, -1)

	var beforeCont, sinceCont *runtime.Container
	if before != "" {
		beforeCont = srv.runtime.Get(before)
		if beforeCont == nil {
			return job.Error(fmt.Errorf("Could not find container with name or id %s", before))
		}
	}

	if since != "" {
		sinceCont = srv.runtime.Get(since)
		if sinceCont == nil {
			return job.Error(fmt.Errorf("Could not find container with name or id %s", since))
		}
	}

	for _, container := range srv.runtime.List() {
		if !container.State.IsRunning() && !all && n <= 0 && since == "" && before == "" {
			continue
		}
		if before != "" && !foundBefore {
			if container.ID == beforeCont.ID {
				foundBefore = true
			}
			continue
		}
		if n > 0 && displayed == n {
			break
		}
		if since != "" {
			if container.ID == sinceCont.ID {
				break
			}
		}
		displayed++
		out := &engine.Env{}
		out.Set("Id", container.ID)
		out.SetList("Names", names[container.ID])
		out.Set("Image", srv.runtime.Repositories().ImageName(container.Image))
		if len(container.Args) > 0 {
			out.Set("Command", fmt.Sprintf("\"%s %s\"", container.Path, container.ArgsAsString()))
		} else {
			out.Set("Command", fmt.Sprintf("\"%s\"", container.Path))
		}
		out.SetInt64("Created", container.Created.Unix())
		out.Set("Status", container.State.String())
		str, err := container.NetworkSettings.PortMappingAPI().ToListString()
		if err != nil {
			return job.Error(err)
		}
		out.Set("Ports", str)
		if size {
			sizeRw, sizeRootFs := container.GetSize()
			out.SetInt64("SizeRw", sizeRw)
			out.SetInt64("SizeRootFs", sizeRootFs)
		}
		outs.Add(out)
	}
	outs.ReverseSort()
	if _, err := outs.WriteListTo(job.Stdout); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (srv *Server) ContainerCommit(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("Not enough arguments. Usage: %s CONTAINER\n", job.Name)
	}
	name := job.Args[0]

	container := srv.runtime.Get(name)
	if container == nil {
		return job.Errorf("No such container: %s", name)
	}
	var config = container.Config
	var newConfig runconfig.Config
	if err := job.GetenvJson("config", &newConfig); err != nil {
		return job.Error(err)
	}

	if err := runconfig.Merge(&newConfig, config); err != nil {
		return job.Error(err)
	}

	img, err := srv.runtime.Commit(container, job.Getenv("repo"), job.Getenv("tag"), job.Getenv("comment"), job.Getenv("author"), &newConfig)
	if err != nil {
		return job.Error(err)
	}
	job.Printf("%s\n", img.ID)
	return engine.StatusOK
}

func (srv *Server) ImageTag(job *engine.Job) engine.Status {
	if len(job.Args) != 2 && len(job.Args) != 3 {
		return job.Errorf("Usage: %s IMAGE REPOSITORY [TAG]\n", job.Name)
	}
	var tag string
	if len(job.Args) == 3 {
		tag = job.Args[2]
	}
	if err := srv.runtime.Repositories().Set(job.Args[1], tag, job.Args[0], job.GetenvBool("force")); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (srv *Server) pullImage(r *registry.Registry, out io.Writer, imgID, endpoint string, token []string, sf *utils.StreamFormatter) error {
	history, err := r.GetRemoteHistory(imgID, endpoint, token)
	if err != nil {
		return err
	}
	out.Write(sf.FormatProgress(utils.TruncateID(imgID), "Pulling dependent layers", nil))
	// FIXME: Try to stream the images?
	// FIXME: Launch the getRemoteImage() in goroutines

	for i := len(history) - 1; i >= 0; i-- {
		id := history[i]

		// ensure no two downloads of the same layer happen at the same time
		if c, err := srv.poolAdd("pull", "layer:"+id); err != nil {
			utils.Errorf("Image (id: %s) pull is already running, skipping: %v", id, err)
			<-c
		}
		defer srv.poolRemove("pull", "layer:"+id)

		if !srv.runtime.Graph().Exists(id) {
			out.Write(sf.FormatProgress(utils.TruncateID(id), "Pulling metadata", nil))
			var (
				imgJSON []byte
				imgSize int
				err     error
				img     *image.Image
			)
			retries := 5
			for j := 1; j <= retries; j++ {
				imgJSON, imgSize, err = r.GetRemoteImageJSON(id, endpoint, token)
				if err != nil && j == retries {
					out.Write(sf.FormatProgress(utils.TruncateID(id), "Error pulling dependent layers", nil))
					return err
				} else if err != nil {
					time.Sleep(time.Duration(j) * 500 * time.Millisecond)
					continue
				}
				img, err = image.NewImgJSON(imgJSON)
				if err != nil && j == retries {
					out.Write(sf.FormatProgress(utils.TruncateID(id), "Error pulling dependent layers", nil))
					return fmt.Errorf("Failed to parse json: %s", err)
				} else if err != nil {
					time.Sleep(time.Duration(j) * 500 * time.Millisecond)
					continue
				} else {
					break
				}
			}

			// Get the layer
			out.Write(sf.FormatProgress(utils.TruncateID(id), "Pulling fs layer", nil))
			layer, err := r.GetRemoteImageLayer(img.ID, endpoint, token)
			if err != nil {
				out.Write(sf.FormatProgress(utils.TruncateID(id), "Error pulling dependent layers", nil))
				return err
			}
			defer layer.Close()
			if err := srv.runtime.Graph().Register(imgJSON, utils.ProgressReader(layer, imgSize, out, sf, false, utils.TruncateID(id), "Downloading"), img); err != nil {
				out.Write(sf.FormatProgress(utils.TruncateID(id), "Error downloading dependent layers", nil))
				return err
			}
		}
		out.Write(sf.FormatProgress(utils.TruncateID(id), "Download complete", nil))

	}
	return nil
}

func (srv *Server) pullRepository(r *registry.Registry, out io.Writer, localName, remoteName, askedTag string, sf *utils.StreamFormatter, parallel bool) error {
	out.Write(sf.FormatStatus("", "Pulling repository %s", localName))

	repoData, err := r.GetRepositoryData(remoteName)
	if err != nil {
		return err
	}

	utils.Debugf("Retrieving the tag list")
	tagsList, err := r.GetRemoteTags(repoData.Endpoints, remoteName, repoData.Tokens)
	if err != nil {
		utils.Errorf("%v", err)
		return err
	}

	for tag, id := range tagsList {
		repoData.ImgList[id] = &registry.ImgData{
			ID:       id,
			Tag:      tag,
			Checksum: "",
		}
	}

	utils.Debugf("Registering tags")
	// If no tag has been specified, pull them all
	if askedTag == "" {
		for tag, id := range tagsList {
			repoData.ImgList[id].Tag = tag
		}
	} else {
		// Otherwise, check that the tag exists and use only that one
		id, exists := tagsList[askedTag]
		if !exists {
			return fmt.Errorf("Tag %s not found in repository %s", askedTag, localName)
		}
		repoData.ImgList[id].Tag = askedTag
	}

	errors := make(chan error)
	for _, image := range repoData.ImgList {
		downloadImage := func(img *registry.ImgData) {
			if askedTag != "" && img.Tag != askedTag {
				utils.Debugf("(%s) does not match %s (id: %s), skipping", img.Tag, askedTag, img.ID)
				if parallel {
					errors <- nil
				}
				return
			}

			if img.Tag == "" {
				utils.Debugf("Image (id: %s) present in this repository but untagged, skipping", img.ID)
				if parallel {
					errors <- nil
				}
				return
			}

			// ensure no two downloads of the same image happen at the same time
			if c, err := srv.poolAdd("pull", "img:"+img.ID); err != nil {
				if c != nil {
					out.Write(sf.FormatProgress(utils.TruncateID(img.ID), "Layer already being pulled by another client. Waiting.", nil))
					<-c
					out.Write(sf.FormatProgress(utils.TruncateID(img.ID), "Download complete", nil))
				} else {
					utils.Errorf("Image (id: %s) pull is already running, skipping: %v", img.ID, err)
				}
				if parallel {
					errors <- nil
				}
				return
			}
			defer srv.poolRemove("pull", "img:"+img.ID)

			out.Write(sf.FormatProgress(utils.TruncateID(img.ID), fmt.Sprintf("Pulling image (%s) from %s", img.Tag, localName), nil))
			success := false
			var lastErr error
			for _, ep := range repoData.Endpoints {
				out.Write(sf.FormatProgress(utils.TruncateID(img.ID), fmt.Sprintf("Pulling image (%s) from %s, endpoint: %s", img.Tag, localName, ep), nil))
				if err := srv.pullImage(r, out, img.ID, ep, repoData.Tokens, sf); err != nil {
					// Its not ideal that only the last error  is returned, it would be better to concatenate the errors.
					// As the error is also given to the output stream the user will see the error.
					lastErr = err
					out.Write(sf.FormatProgress(utils.TruncateID(img.ID), fmt.Sprintf("Error pulling image (%s) from %s, endpoint: %s, %s", img.Tag, localName, ep, err), nil))
					continue
				}
				success = true
				break
			}
			if !success {
				out.Write(sf.FormatProgress(utils.TruncateID(img.ID), fmt.Sprintf("Error pulling image (%s) from %s, %s", img.Tag, localName, lastErr), nil))
				if parallel {
					errors <- fmt.Errorf("Could not find repository on any of the indexed registries.")
					return
				}
			}
			out.Write(sf.FormatProgress(utils.TruncateID(img.ID), "Download complete", nil))

			if parallel {
				errors <- nil
			}
		}

		if parallel {
			go downloadImage(image)
		} else {
			downloadImage(image)
		}
	}
	if parallel {
		var lastError error
		for i := 0; i < len(repoData.ImgList); i++ {
			if err := <-errors; err != nil {
				lastError = err
			}
		}
		if lastError != nil {
			return lastError
		}

	}
	for tag, id := range tagsList {
		if askedTag != "" && tag != askedTag {
			continue
		}
		if err := srv.runtime.Repositories().Set(localName, tag, id, true); err != nil {
			return err
		}
	}
	if err := srv.runtime.Repositories().Save(); err != nil {
		return err
	}

	return nil
}

func (srv *Server) poolAdd(kind, key string) (chan struct{}, error) {
	srv.Lock()
	defer srv.Unlock()

	if c, exists := srv.pullingPool[key]; exists {
		return c, fmt.Errorf("pull %s is already in progress", key)
	}
	if c, exists := srv.pushingPool[key]; exists {
		return c, fmt.Errorf("push %s is already in progress", key)
	}

	c := make(chan struct{})
	switch kind {
	case "pull":
		srv.pullingPool[key] = c
	case "push":
		srv.pushingPool[key] = c
	default:
		return nil, fmt.Errorf("Unknown pool type")
	}
	return c, nil
}

func (srv *Server) poolRemove(kind, key string) error {
	srv.Lock()
	defer srv.Unlock()
	switch kind {
	case "pull":
		if c, exists := srv.pullingPool[key]; exists {
			close(c)
			delete(srv.pullingPool, key)
		}
	case "push":
		if c, exists := srv.pushingPool[key]; exists {
			close(c)
			delete(srv.pushingPool, key)
		}
	default:
		return fmt.Errorf("Unknown pool type")
	}
	return nil
}

func (srv *Server) ImagePull(job *engine.Job) engine.Status {
	if n := len(job.Args); n != 1 && n != 2 {
		return job.Errorf("Usage: %s IMAGE [TAG]", job.Name)
	}
	var (
		localName   = job.Args[0]
		tag         string
		sf          = utils.NewStreamFormatter(job.GetenvBool("json"))
		authConfig  = &registry.AuthConfig{}
		metaHeaders map[string][]string
	)
	if len(job.Args) > 1 {
		tag = job.Args[1]
	}

	job.GetenvJson("authConfig", authConfig)
	job.GetenvJson("metaHeaders", metaHeaders)

	c, err := srv.poolAdd("pull", localName+":"+tag)
	if err != nil {
		if c != nil {
			// Another pull of the same repository is already taking place; just wait for it to finish
			job.Stdout.Write(sf.FormatStatus("", "Repository %s already being pulled by another client. Waiting.", localName))
			<-c
			return engine.StatusOK
		}
		return job.Error(err)
	}
	defer srv.poolRemove("pull", localName+":"+tag)

	// Resolve the Repository name from fqn to endpoint + name
	hostname, remoteName, err := registry.ResolveRepositoryName(localName)
	if err != nil {
		return job.Error(err)
	}

	endpoint, err := registry.ExpandAndVerifyRegistryUrl(hostname)
	if err != nil {
		return job.Error(err)
	}

	r, err := registry.NewRegistry(authConfig, srv.HTTPRequestFactory(metaHeaders), endpoint)
	if err != nil {
		return job.Error(err)
	}

	if endpoint == registry.IndexServerAddress() {
		// If pull "index.docker.io/foo/bar", it's stored locally under "foo/bar"
		localName = remoteName
	}

	if err = srv.pullRepository(r, job.Stdout, localName, remoteName, tag, sf, job.GetenvBool("parallel")); err != nil {
		return job.Error(err)
	}

	return engine.StatusOK
}

// Retrieve the all the images to be uploaded in the correct order
func (srv *Server) getImageList(localRepo map[string]string) ([]string, map[string][]string, error) {
	var (
		imageList   []string
		imagesSeen  map[string]bool     = make(map[string]bool)
		tagsByImage map[string][]string = make(map[string][]string)
	)

	for tag, id := range localRepo {
		var imageListForThisTag []string

		tagsByImage[id] = append(tagsByImage[id], tag)

		for img, err := srv.runtime.Graph().Get(id); img != nil; img, err = img.GetParent() {
			if err != nil {
				return nil, nil, err
			}

			if imagesSeen[img.ID] {
				// This image is already on the list, we can ignore it and all its parents
				break
			}

			imagesSeen[img.ID] = true
			imageListForThisTag = append(imageListForThisTag, img.ID)
		}

		// reverse the image list for this tag (so the "most"-parent image is first)
		for i, j := 0, len(imageListForThisTag)-1; i < j; i, j = i+1, j-1 {
			imageListForThisTag[i], imageListForThisTag[j] = imageListForThisTag[j], imageListForThisTag[i]
		}

		// append to main image list
		imageList = append(imageList, imageListForThisTag...)
	}

	utils.Debugf("Image list: %v", imageList)
	utils.Debugf("Tags by image: %v", tagsByImage)

	return imageList, tagsByImage, nil
}

func (srv *Server) pushRepository(r *registry.Registry, out io.Writer, localName, remoteName string, localRepo map[string]string, sf *utils.StreamFormatter) error {
	out = utils.NewWriteFlusher(out)
	utils.Debugf("Local repo: %s", localRepo)
	imgList, tagsByImage, err := srv.getImageList(localRepo)
	if err != nil {
		return err
	}

	out.Write(sf.FormatStatus("", "Sending image list"))

	var repoData *registry.RepositoryData
	var imageIndex []*registry.ImgData

	for _, imgId := range imgList {
		if tags, exists := tagsByImage[imgId]; exists {
			// If an image has tags you must add an entry in the image index
			// for each tag
			for _, tag := range tags {
				imageIndex = append(imageIndex, &registry.ImgData{
					ID:  imgId,
					Tag: tag,
				})
			}
		} else {
			// If the image does not have a tag it still needs to be sent to the
			// registry with an empty tag so that it is accociated with the repository
			imageIndex = append(imageIndex, &registry.ImgData{
				ID:  imgId,
				Tag: "",
			})

		}
	}

	utils.Debugf("Preparing to push %s with the following images and tags\n", localRepo)
	for _, data := range imageIndex {
		utils.Debugf("Pushing ID: %s with Tag: %s\n", data.ID, data.Tag)
	}

	// Register all the images in a repository with the registry
	// If an image is not in this list it will not be associated with the repository
	repoData, err = r.PushImageJSONIndex(remoteName, imageIndex, false, nil)
	if err != nil {
		return err
	}

	for _, ep := range repoData.Endpoints {
		out.Write(sf.FormatStatus("", "Pushing repository %s (%d tags)", localName, len(localRepo)))

		for _, imgId := range imgList {
			if r.LookupRemoteImage(imgId, ep, repoData.Tokens) {
				out.Write(sf.FormatStatus("", "Image %s already pushed, skipping", utils.TruncateID(imgId)))
			} else {
				if _, err := srv.pushImage(r, out, remoteName, imgId, ep, repoData.Tokens, sf); err != nil {
					// FIXME: Continue on error?
					return err
				}
			}

			for _, tag := range tagsByImage[imgId] {
				out.Write(sf.FormatStatus("", "Pushing tag for rev [%s] on {%s}", utils.TruncateID(imgId), ep+"repositories/"+remoteName+"/tags/"+tag))

				if err := r.PushRegistryTag(remoteName, imgId, tag, ep, repoData.Tokens); err != nil {
					return err
				}
			}
		}
	}

	if _, err := r.PushImageJSONIndex(remoteName, imageIndex, true, repoData.Endpoints); err != nil {
		return err
	}

	return nil
}

func (srv *Server) pushImage(r *registry.Registry, out io.Writer, remote, imgID, ep string, token []string, sf *utils.StreamFormatter) (checksum string, err error) {
	out = utils.NewWriteFlusher(out)
	jsonRaw, err := ioutil.ReadFile(path.Join(srv.runtime.Graph().Root, imgID, "json"))
	if err != nil {
		return "", fmt.Errorf("Cannot retrieve the path for {%s}: %s", imgID, err)
	}
	out.Write(sf.FormatProgress(utils.TruncateID(imgID), "Pushing", nil))

	imgData := &registry.ImgData{
		ID: imgID,
	}

	// Send the json
	if err := r.PushImageJSONRegistry(imgData, jsonRaw, ep, token); err != nil {
		if err == registry.ErrAlreadyExists {
			out.Write(sf.FormatProgress(utils.TruncateID(imgData.ID), "Image already pushed, skipping", nil))
			return "", nil
		}
		return "", err
	}

	layerData, err := srv.runtime.Graph().TempLayerArchive(imgID, archive.Uncompressed, sf, out)
	if err != nil {
		return "", fmt.Errorf("Failed to generate layer archive: %s", err)
	}
	defer os.RemoveAll(layerData.Name())

	// Send the layer
	utils.Debugf("rendered layer for %s of [%d] size", imgData.ID, layerData.Size)

	checksum, checksumPayload, err := r.PushImageLayerRegistry(imgData.ID, utils.ProgressReader(layerData, int(layerData.Size), out, sf, false, utils.TruncateID(imgData.ID), "Pushing"), ep, token, jsonRaw)
	if err != nil {
		return "", err
	}
	imgData.Checksum = checksum
	imgData.ChecksumPayload = checksumPayload
	// Send the checksum
	if err := r.PushImageChecksumRegistry(imgData, ep, token); err != nil {
		return "", err
	}

	out.Write(sf.FormatProgress(utils.TruncateID(imgData.ID), "Image successfully pushed", nil))
	return imgData.Checksum, nil
}

// FIXME: Allow to interrupt current push when new push of same image is done.
func (srv *Server) ImagePush(job *engine.Job) engine.Status {
	if n := len(job.Args); n != 1 {
		return job.Errorf("Usage: %s IMAGE", job.Name)
	}
	var (
		localName   = job.Args[0]
		sf          = utils.NewStreamFormatter(job.GetenvBool("json"))
		authConfig  = &registry.AuthConfig{}
		metaHeaders map[string][]string
	)

	job.GetenvJson("authConfig", authConfig)
	job.GetenvJson("metaHeaders", metaHeaders)
	if _, err := srv.poolAdd("push", localName); err != nil {
		return job.Error(err)
	}
	defer srv.poolRemove("push", localName)

	// Resolve the Repository name from fqn to endpoint + name
	hostname, remoteName, err := registry.ResolveRepositoryName(localName)
	if err != nil {
		return job.Error(err)
	}

	endpoint, err := registry.ExpandAndVerifyRegistryUrl(hostname)
	if err != nil {
		return job.Error(err)
	}

	img, err := srv.runtime.Graph().Get(localName)
	r, err2 := registry.NewRegistry(authConfig, srv.HTTPRequestFactory(metaHeaders), endpoint)
	if err2 != nil {
		return job.Error(err2)
	}

	if err != nil {
		reposLen := len(srv.runtime.Repositories().Repositories[localName])
		job.Stdout.Write(sf.FormatStatus("", "The push refers to a repository [%s] (len: %d)", localName, reposLen))
		// If it fails, try to get the repository
		if localRepo, exists := srv.runtime.Repositories().Repositories[localName]; exists {
			if err := srv.pushRepository(r, job.Stdout, localName, remoteName, localRepo, sf); err != nil {
				return job.Error(err)
			}
			return engine.StatusOK
		}
		return job.Error(err)
	}

	var token []string
	job.Stdout.Write(sf.FormatStatus("", "The push refers to an image: [%s]", localName))
	if _, err := srv.pushImage(r, job.Stdout, remoteName, img.ID, endpoint, token, sf); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (srv *Server) ImageImport(job *engine.Job) engine.Status {
	if n := len(job.Args); n != 2 && n != 3 {
		return job.Errorf("Usage: %s SRC REPO [TAG]", job.Name)
	}
	var (
		src     = job.Args[0]
		repo    = job.Args[1]
		tag     string
		sf      = utils.NewStreamFormatter(job.GetenvBool("json"))
		archive archive.ArchiveReader
		resp    *http.Response
	)
	if len(job.Args) > 2 {
		tag = job.Args[2]
	}

	if src == "-" {
		archive = job.Stdin
	} else {
		u, err := url.Parse(src)
		if err != nil {
			return job.Error(err)
		}
		if u.Scheme == "" {
			u.Scheme = "http"
			u.Host = src
			u.Path = ""
		}
		job.Stdout.Write(sf.FormatStatus("", "Downloading from %s", u))
		// Download with curl (pretty progress bar)
		// If curl is not available, fallback to http.Get()
		resp, err = utils.Download(u.String())
		if err != nil {
			return job.Error(err)
		}
		progressReader := utils.ProgressReader(resp.Body, int(resp.ContentLength), job.Stdout, sf, true, "", "Importing")
		defer progressReader.Close()
		archive = progressReader
	}
	img, err := srv.runtime.Graph().Create(archive, "", "", "Imported from "+src, "", nil, nil)
	if err != nil {
		return job.Error(err)
	}
	// Optionally register the image at REPO/TAG
	if repo != "" {
		if err := srv.runtime.Repositories().Set(repo, tag, img.ID, true); err != nil {
			return job.Error(err)
		}
	}
	job.Stdout.Write(sf.FormatStatus("", img.ID))
	return engine.StatusOK
}

func (srv *Server) ContainerCreate(job *engine.Job) engine.Status {
	var name string
	if len(job.Args) == 1 {
		name = job.Args[0]
	} else if len(job.Args) > 1 {
		return job.Errorf("Usage: %s", job.Name)
	}
	config := runconfig.ContainerConfigFromJob(job)
	if config.Memory != 0 && config.Memory < 524288 {
		return job.Errorf("Minimum memory limit allowed is 512k")
	}
	if config.Memory > 0 && !srv.runtime.SystemConfig().MemoryLimit {
		job.Errorf("Your kernel does not support memory limit capabilities. Limitation discarded.\n")
		config.Memory = 0
	}
	if config.Memory > 0 && !srv.runtime.SystemConfig().SwapLimit {
		job.Errorf("Your kernel does not support swap limit capabilities. Limitation discarded.\n")
		config.MemorySwap = -1
	}
	resolvConf, err := utils.GetResolvConf()
	if err != nil {
		return job.Error(err)
	}
	if !config.NetworkDisabled && len(config.Dns) == 0 && len(srv.runtime.Config().Dns) == 0 && utils.CheckLocalDns(resolvConf) {
		job.Errorf("Local (127.0.0.1) DNS resolver found in resolv.conf and containers can't use it. Using default external servers : %v\n", runtime.DefaultDns)
		config.Dns = runtime.DefaultDns
	}

	container, buildWarnings, err := srv.runtime.Create(config, name)
	if err != nil {
		if srv.runtime.Graph().IsNotExist(err) {
			_, tag := utils.ParseRepositoryTag(config.Image)
			if tag == "" {
				tag = graph.DEFAULTTAG
			}
			return job.Errorf("No such image: %s (tag: %s)", config.Image, tag)
		}
		return job.Error(err)
	}
	if !container.Config.NetworkDisabled && srv.runtime.SystemConfig().IPv4ForwardingDisabled {
		job.Errorf("IPv4 forwarding is disabled.\n")
	}
	srv.LogEvent("create", container.ID, srv.runtime.Repositories().ImageName(container.Image))
	// FIXME: this is necessary because runtime.Create might return a nil container
	// with a non-nil error. This should not happen! Once it's fixed we
	// can remove this workaround.
	if container != nil {
		job.Printf("%s\n", container.ID)
	}
	for _, warning := range buildWarnings {
		job.Errorf("%s\n", warning)
	}
	return engine.StatusOK
}

func (srv *Server) ContainerRestart(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("Usage: %s CONTAINER\n", job.Name)
	}
	var (
		name = job.Args[0]
		t    = 10
	)
	if job.EnvExists("t") {
		t = job.GetenvInt("t")
	}
	if container := srv.runtime.Get(name); container != nil {
		if err := container.Restart(int(t)); err != nil {
			return job.Errorf("Cannot restart container %s: %s\n", name, err)
		}
		srv.LogEvent("restart", container.ID, srv.runtime.Repositories().ImageName(container.Image))
	} else {
		return job.Errorf("No such container: %s\n", name)
	}
	return engine.StatusOK
}

func (srv *Server) ContainerDestroy(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("Not enough arguments. Usage: %s CONTAINER\n", job.Name)
	}
	name := job.Args[0]
	removeVolume := job.GetenvBool("removeVolume")
	removeLink := job.GetenvBool("removeLink")
	forceRemove := job.GetenvBool("forceRemove")

	container := srv.runtime.Get(name)

	if removeLink {
		if container == nil {
			return job.Errorf("No such link: %s", name)
		}
		name, err := runtime.GetFullContainerName(name)
		if err != nil {
			job.Error(err)
		}
		parent, n := path.Split(name)
		if parent == "/" {
			return job.Errorf("Conflict, cannot remove the default name of the container")
		}
		pe := srv.runtime.ContainerGraph().Get(parent)
		if pe == nil {
			return job.Errorf("Cannot get parent %s for name %s", parent, name)
		}
		parentContainer := srv.runtime.Get(pe.ID())

		if parentContainer != nil {
			parentContainer.DisableLink(n)
		}

		if err := srv.runtime.ContainerGraph().Delete(name); err != nil {
			return job.Error(err)
		}
		return engine.StatusOK
	}

	if container != nil {
		if container.State.IsRunning() {
			if forceRemove {
				if err := container.Stop(5); err != nil {
					return job.Errorf("Could not stop running container, cannot remove - %v", err)
				}
			} else {
				return job.Errorf("Impossible to remove a running container, please stop it first or use -f")
			}
		}
		if err := srv.runtime.Destroy(container); err != nil {
			return job.Errorf("Cannot destroy container %s: %s", name, err)
		}
		srv.LogEvent("destroy", container.ID, srv.runtime.Repositories().ImageName(container.Image))

		if removeVolume {
			var (
				volumes     = make(map[string]struct{})
				binds       = make(map[string]struct{})
				usedVolumes = make(map[string]*runtime.Container)
			)

			// the volume id is always the base of the path
			getVolumeId := func(p string) string {
				return filepath.Base(strings.TrimSuffix(p, "/layer"))
			}

			// populate bind map so that they can be skipped and not removed
			for _, bind := range container.HostConfig().Binds {
				source := strings.Split(bind, ":")[0]
				// TODO: refactor all volume stuff, all of it
				// this is very important that we eval the link
				// or comparing the keys to container.Volumes will not work
				p, err := filepath.EvalSymlinks(source)
				if err != nil {
					return job.Error(err)
				}
				source = p
				binds[source] = struct{}{}
			}

			// Store all the deleted containers volumes
			for _, volumeId := range container.Volumes {
				// Skip the volumes mounted from external
				// bind mounts here will will be evaluated for a symlink
				if _, exists := binds[volumeId]; exists {
					continue
				}

				volumeId = getVolumeId(volumeId)
				volumes[volumeId] = struct{}{}
			}

			// Retrieve all volumes from all remaining containers
			for _, container := range srv.runtime.List() {
				for _, containerVolumeId := range container.Volumes {
					containerVolumeId = getVolumeId(containerVolumeId)
					usedVolumes[containerVolumeId] = container
				}
			}

			for volumeId := range volumes {
				// If the requested volu
				if c, exists := usedVolumes[volumeId]; exists {
					log.Printf("The volume %s is used by the container %s. Impossible to remove it. Skipping.\n", volumeId, c.ID)
					continue
				}
				if err := srv.runtime.Volumes().Delete(volumeId); err != nil {
					return job.Errorf("Error calling volumes.Delete(%q): %v", volumeId, err)
				}
			}
		}
	} else {
		return job.Errorf("No such container: %s", name)
	}
	return engine.StatusOK
}

func (srv *Server) DeleteImage(name string, imgs *engine.Table, first, force, noprune bool) error {
	var (
		repoName, tag string
		tags          = []string{}
	)

	repoName, tag = utils.ParseRepositoryTag(name)
	if tag == "" {
		tag = graph.DEFAULTTAG
	}

	img, err := srv.runtime.Repositories().LookupImage(name)
	if err != nil {
		if r, _ := srv.runtime.Repositories().Get(repoName); r != nil {
			return fmt.Errorf("No such image: %s:%s", repoName, tag)
		}
		return fmt.Errorf("No such image: %s", name)
	}

	if strings.Contains(img.ID, name) {
		repoName = ""
		tag = ""
	}

	byParents, err := srv.runtime.Graph().ByParent()
	if err != nil {
		return err
	}

	//If delete by id, see if the id belong only to one repository
	if repoName == "" {
		for _, repoAndTag := range srv.runtime.Repositories().ByID()[img.ID] {
			parsedRepo, parsedTag := utils.ParseRepositoryTag(repoAndTag)
			if repoName == "" || repoName == parsedRepo {
				repoName = parsedRepo
				if parsedTag != "" {
					tags = append(tags, parsedTag)
				}
			} else if repoName != parsedRepo && !force {
				// the id belongs to multiple repos, like base:latest and user:test,
				// in that case return conflict
				return fmt.Errorf("Conflict, cannot delete image %s because it is tagged in multiple repositories, use -f to force", name)
			}
		}
	} else {
		tags = append(tags, tag)
	}

	if !first && len(tags) > 0 {
		return nil
	}

	//Untag the current image
	for _, tag := range tags {
		tagDeleted, err := srv.runtime.Repositories().Delete(repoName, tag)
		if err != nil {
			return err
		}
		if tagDeleted {
			out := &engine.Env{}
			out.Set("Untagged", repoName+":"+tag)
			imgs.Add(out)
			srv.LogEvent("untag", img.ID, "")
		}
	}
	tags = srv.runtime.Repositories().ByID()[img.ID]
	if (len(tags) <= 1 && repoName == "") || len(tags) == 0 {
		if len(byParents[img.ID]) == 0 {
			if err := srv.canDeleteImage(img.ID); err != nil {
				return err
			}
			if err := srv.runtime.Repositories().DeleteAll(img.ID); err != nil {
				return err
			}
			if err := srv.runtime.Graph().Delete(img.ID); err != nil {
				return err
			}
			out := &engine.Env{}
			out.Set("Deleted", img.ID)
			imgs.Add(out)
			srv.LogEvent("delete", img.ID, "")
			if img.Parent != "" && !noprune {
				err := srv.DeleteImage(img.Parent, imgs, false, force, noprune)
				if first {
					return err
				}

			}

		}
	}
	return nil
}

func (srv *Server) ImageDelete(job *engine.Job) engine.Status {
	if n := len(job.Args); n != 1 {
		return job.Errorf("Usage: %s IMAGE", job.Name)
	}
	imgs := engine.NewTable("", 0)
	if err := srv.DeleteImage(job.Args[0], imgs, true, job.GetenvBool("force"), job.GetenvBool("noprune")); err != nil {
		return job.Error(err)
	}
	if len(imgs.Data) == 0 {
		return job.Errorf("Conflict, %s wasn't deleted", job.Args[0])
	}
	if _, err := imgs.WriteListTo(job.Stdout); err != nil {
		return job.Error(err)
	}
	return engine.StatusOK
}

func (srv *Server) canDeleteImage(imgID string) error {
	for _, container := range srv.runtime.List() {
		parent, err := srv.runtime.Repositories().LookupImage(container.Image)
		if err != nil {
			return err
		}

		if err := parent.WalkHistory(func(p *image.Image) error {
			if imgID == p.ID {
				return fmt.Errorf("Conflict, cannot delete %s because the container %s is using it", utils.TruncateID(imgID), utils.TruncateID(container.ID))
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (srv *Server) ImageGetCached(imgID string, config *runconfig.Config) (*image.Image, error) {
	// Retrieve all images
	images, err := srv.runtime.Graph().Map()
	if err != nil {
		return nil, err
	}

	// Store the tree in a map of map (map[parentId][childId])
	imageMap := make(map[string]map[string]struct{})
	for _, img := range images {
		if _, exists := imageMap[img.Parent]; !exists {
			imageMap[img.Parent] = make(map[string]struct{})
		}
		imageMap[img.Parent][img.ID] = struct{}{}
	}

	// Loop on the children of the given image and check the config
	var match *image.Image
	for elem := range imageMap[imgID] {
		img, err := srv.runtime.Graph().Get(elem)
		if err != nil {
			return nil, err
		}
		if runconfig.Compare(&img.ContainerConfig, config) {
			if match == nil || match.Created.Before(img.Created) {
				match = img
			}
		}
	}
	return match, nil
}

func (srv *Server) RegisterLinks(container *runtime.Container, hostConfig *runconfig.HostConfig) error {
	runtime := srv.runtime

	if hostConfig != nil && hostConfig.Links != nil {
		for _, l := range hostConfig.Links {
			parts, err := utils.PartParser("name:alias", l)
			if err != nil {
				return err
			}
			child, err := srv.runtime.GetByName(parts["name"])
			if err != nil {
				return err
			}
			if child == nil {
				return fmt.Errorf("Could not get container for %s", parts["name"])
			}
			if err := runtime.RegisterLink(container, child, parts["alias"]); err != nil {
				return err
			}
		}

		// After we load all the links into the runtime
		// set them to nil on the hostconfig
		hostConfig.Links = nil
		if err := container.WriteHostConfig(); err != nil {
			return err
		}
	}
	return nil
}

func (srv *Server) ContainerStart(job *engine.Job) engine.Status {
	if len(job.Args) < 1 {
		return job.Errorf("Usage: %s container_id", job.Name)
	}
	name := job.Args[0]
	runtime := srv.runtime
	container := runtime.Get(name)

	if container == nil {
		return job.Errorf("No such container: %s", name)
	}
	// If no environment was set, then no hostconfig was passed.
	if len(job.Environ()) > 0 {
		hostConfig := runconfig.ContainerHostConfigFromJob(job)
		// Validate the HostConfig binds. Make sure that:
		// 1) the source of a bind mount isn't /
		//         The bind mount "/:/foo" isn't allowed.
		// 2) Check that the source exists
		//        The source to be bind mounted must exist.
		for _, bind := range hostConfig.Binds {
			splitBind := strings.Split(bind, ":")
			source := splitBind[0]

			// refuse to bind mount "/" to the container
			if source == "/" {
				return job.Errorf("Invalid bind mount '%s' : source can't be '/'", bind)
			}

			// ensure the source exists on the host
			_, err := os.Stat(source)
			if err != nil && os.IsNotExist(err) {
				err = os.MkdirAll(source, 0755)
				if err != nil {
					return job.Errorf("Could not create local directory '%s' for bind mount: %s!", source, err.Error())
				}
			}
		}
		// Register any links from the host config before starting the container
		if err := srv.RegisterLinks(container, hostConfig); err != nil {
			return job.Error(err)
		}
		container.SetHostConfig(hostConfig)
		container.ToDisk()
	}
	if err := container.Start(); err != nil {
		return job.Errorf("Cannot start container %s: %s", name, err)
	}
	srv.LogEvent("start", container.ID, runtime.Repositories().ImageName(container.Image))

	return engine.StatusOK
}

func (srv *Server) ContainerStop(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("Usage: %s CONTAINER\n", job.Name)
	}
	var (
		name = job.Args[0]
		t    = 10
	)
	if job.EnvExists("t") {
		t = job.GetenvInt("t")
	}
	if container := srv.runtime.Get(name); container != nil {
		if err := container.Stop(int(t)); err != nil {
			return job.Errorf("Cannot stop container %s: %s\n", name, err)
		}
		srv.LogEvent("stop", container.ID, srv.runtime.Repositories().ImageName(container.Image))
	} else {
		return job.Errorf("No such container: %s\n", name)
	}
	return engine.StatusOK
}

func (srv *Server) ContainerWait(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("Usage: %s", job.Name)
	}
	name := job.Args[0]
	if container := srv.runtime.Get(name); container != nil {
		status := container.Wait()
		job.Printf("%d\n", status)
		return engine.StatusOK
	}
	return job.Errorf("%s: no such container: %s", job.Name, name)
}

func (srv *Server) ContainerResize(job *engine.Job) engine.Status {
	if len(job.Args) != 3 {
		return job.Errorf("Not enough arguments. Usage: %s CONTAINER HEIGHT WIDTH\n", job.Name)
	}
	name := job.Args[0]
	height, err := strconv.Atoi(job.Args[1])
	if err != nil {
		return job.Error(err)
	}
	width, err := strconv.Atoi(job.Args[2])
	if err != nil {
		return job.Error(err)
	}
	if container := srv.runtime.Get(name); container != nil {
		if err := container.Resize(height, width); err != nil {
			return job.Error(err)
		}
		return engine.StatusOK
	}
	return job.Errorf("No such container: %s", name)
}

func (srv *Server) ContainerAttach(job *engine.Job) engine.Status {
	if len(job.Args) != 1 {
		return job.Errorf("Usage: %s CONTAINER\n", job.Name)
	}

	var (
		name   = job.Args[0]
		logs   = job.GetenvBool("logs")
		stream = job.GetenvBool("stream")
		stdin  = job.GetenvBool("stdin")
		stdout = job.GetenvBool("stdout")
		stderr = job.GetenvBool("stderr")
	)

	container := srv.runtime.Get(name)
	if container == nil {
		return job.Errorf("No such container: %s", name)
	}

	//logs
	if logs {
		cLog, err := container.ReadLog("json")
		if err != nil && os.IsNotExist(err) {
			// Legacy logs
			utils.Debugf("Old logs format")
			if stdout {
				cLog, err := container.ReadLog("stdout")
				if err != nil {
					utils.Errorf("Error reading logs (stdout): %s", err)
				} else if _, err := io.Copy(job.Stdout, cLog); err != nil {
					utils.Errorf("Error streaming logs (stdout): %s", err)
				}
			}
			if stderr {
				cLog, err := container.ReadLog("stderr")
				if err != nil {
					utils.Errorf("Error reading logs (stderr): %s", err)
				} else if _, err := io.Copy(job.Stderr, cLog); err != nil {
					utils.Errorf("Error streaming logs (stderr): %s", err)
				}
			}
		} else if err != nil {
			utils.Errorf("Error reading logs (json): %s", err)
		} else {
			dec := json.NewDecoder(cLog)
			for {
				l := &utils.JSONLog{}

				if err := dec.Decode(l); err == io.EOF {
					break
				} else if err != nil {
					utils.Errorf("Error streaming logs: %s", err)
					break
				}
				if l.Stream == "stdout" && stdout {
					fmt.Fprintf(job.Stdout, "%s", l.Log)
				}
				if l.Stream == "stderr" && stderr {
					fmt.Fprintf(job.Stderr, "%s", l.Log)
				}
			}
		}
	}

	//stream
	if stream {
		if container.State.IsGhost() {
			return job.Errorf("Impossible to attach to a ghost container")
		}

		var (
			cStdin           io.ReadCloser
			cStdout, cStderr io.Writer
			cStdinCloser     io.Closer
		)

		if stdin {
			r, w := io.Pipe()
			go func() {
				defer w.Close()
				defer utils.Debugf("Closing buffered stdin pipe")
				io.Copy(w, job.Stdin)
			}()
			cStdin = r
			cStdinCloser = job.Stdin
		}
		if stdout {
			cStdout = job.Stdout
		}
		if stderr {
			cStderr = job.Stderr
		}

		<-container.Attach(cStdin, cStdinCloser, cStdout, cStderr)

		// If we are in stdinonce mode, wait for the process to end
		// otherwise, simply return
		if container.Config.StdinOnce && !container.Config.Tty {
			container.Wait()
		}
	}
	return engine.StatusOK
}

func (srv *Server) ContainerInspect(name string) (*runtime.Container, error) {
	if container := srv.runtime.Get(name); container != nil {
		return container, nil
	}
	return nil, fmt.Errorf("No such container: %s", name)
}

func (srv *Server) ImageInspect(name string) (*image.Image, error) {
	if image, err := srv.runtime.Repositories().LookupImage(name); err == nil && image != nil {
		return image, nil
	}
	return nil, fmt.Errorf("No such image: %s", name)
}

func (srv *Server) JobInspect(job *engine.Job) engine.Status {
	// TODO: deprecate KIND/conflict
	if n := len(job.Args); n != 2 {
		return job.Errorf("Usage: %s CONTAINER|IMAGE KIND", job.Name)
	}
	var (
		name                    = job.Args[0]
		kind                    = job.Args[1]
		object                  interface{}
		conflict                = job.GetenvBool("conflict") //should the job detect conflict between containers and images
		image, errImage         = srv.ImageInspect(name)
		container, errContainer = srv.ContainerInspect(name)
	)

	if conflict && image != nil && container != nil {
		return job.Errorf("Conflict between containers and images")
	}

	switch kind {
	case "image":
		if errImage != nil {
			return job.Error(errImage)
		}
		object = image
	case "container":
		if errContainer != nil {
			return job.Error(errContainer)
		}
		object = &struct {
			*runtime.Container
			HostConfig *runconfig.HostConfig
		}{container, container.HostConfig()}
	default:
		return job.Errorf("Unknown kind: %s", kind)
	}

	b, err := json.Marshal(object)
	if err != nil {
		return job.Error(err)
	}
	job.Stdout.Write(b)
	return engine.StatusOK
}

func (srv *Server) ContainerCopy(job *engine.Job) engine.Status {
	if len(job.Args) != 2 {
		return job.Errorf("Usage: %s CONTAINER RESOURCE\n", job.Name)
	}

	var (
		name     = job.Args[0]
		resource = job.Args[1]
	)

	if container := srv.runtime.Get(name); container != nil {

		data, err := container.Copy(resource)
		if err != nil {
			return job.Error(err)
		}
		defer data.Close()

		if _, err := io.Copy(job.Stdout, data); err != nil {
			return job.Error(err)
		}
		return engine.StatusOK
	}
	return job.Errorf("No such container: %s", name)
}

func NewServer(eng *engine.Engine, config *daemonconfig.Config) (*Server, error) {
	runtime, err := runtime.NewRuntime(config, eng)
	if err != nil {
		return nil, err
	}
	srv := &Server{
		Eng:         eng,
		runtime:     runtime,
		pullingPool: make(map[string]chan struct{}),
		pushingPool: make(map[string]chan struct{}),
		events:      make([]utils.JSONMessage, 0, 64), //only keeps the 64 last events
		listeners:   make(map[string]chan utils.JSONMessage),
		running:     true,
	}
	runtime.SetServer(srv)
	return srv, nil
}

func (srv *Server) HTTPRequestFactory(metaHeaders map[string][]string) *utils.HTTPRequestFactory {
	httpVersion := make([]utils.VersionInfo, 0, 4)
	httpVersion = append(httpVersion, &simpleVersionInfo{"docker", dockerversion.VERSION})
	httpVersion = append(httpVersion, &simpleVersionInfo{"go", goruntime.Version()})
	httpVersion = append(httpVersion, &simpleVersionInfo{"git-commit", dockerversion.GITCOMMIT})
	if kernelVersion, err := utils.GetKernelVersion(); err == nil {
		httpVersion = append(httpVersion, &simpleVersionInfo{"kernel", kernelVersion.String()})
	}
	httpVersion = append(httpVersion, &simpleVersionInfo{"os", goruntime.GOOS})
	httpVersion = append(httpVersion, &simpleVersionInfo{"arch", goruntime.GOARCH})
	ud := utils.NewHTTPUserAgentDecorator(httpVersion...)
	md := &utils.HTTPMetaHeadersDecorator{
		Headers: metaHeaders,
	}
	factory := utils.NewHTTPRequestFactory(ud, md)
	return factory
}

func (srv *Server) LogEvent(action, id, from string) *utils.JSONMessage {
	now := time.Now().UTC().Unix()
	jm := utils.JSONMessage{Status: action, ID: id, From: from, Time: now}
	srv.AddEvent(jm)
	for _, c := range srv.listeners {
		select { // non blocking channel
		case c <- jm:
		default:
		}
	}
	return &jm
}

func (srv *Server) AddEvent(jm utils.JSONMessage) {
	srv.Lock()
	defer srv.Unlock()
	srv.events = append(srv.events, jm)
}

func (srv *Server) GetEvents() []utils.JSONMessage {
	srv.RLock()
	defer srv.RUnlock()
	return srv.events
}

func (srv *Server) SetRunning(status bool) {
	srv.Lock()
	defer srv.Unlock()

	srv.running = status
}

func (srv *Server) IsRunning() bool {
	srv.RLock()
	defer srv.RUnlock()
	return srv.running
}

func (srv *Server) Close() error {
	if srv == nil {
		return nil
	}
	srv.SetRunning(false)
	if srv.runtime == nil {
		return nil
	}
	return srv.runtime.Close()
}

type Server struct {
	sync.RWMutex
	runtime     *runtime.Runtime
	pullingPool map[string]chan struct{}
	pushingPool map[string]chan struct{}
	events      []utils.JSONMessage
	listeners   map[string]chan utils.JSONMessage
	Eng         *engine.Engine
	running     bool
}
