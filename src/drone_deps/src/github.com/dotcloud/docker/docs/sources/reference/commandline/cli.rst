:title: Command Line Interface
:description: Docker's CLI command description and usage
:keywords: Docker, Docker documentation, CLI, command line

.. _cli:

Command Line Help
-----------------

To list available commands, either run ``docker`` with no parameters or execute
``docker help``::

  $ sudo docker
    Usage: docker [OPTIONS] COMMAND [arg...]
      -H=[unix:///var/run/docker.sock]: tcp://[host]:port to bind/connect to or unix://[/path/to/socket] to use. When host=[127.0.0.1] is omitted for tcp or path=[/var/run/docker.sock] is omitted for unix sockets, default values are used.

    A self-sufficient runtime for linux containers.

    ...

.. _cli_options:

Options
-------

Single character commandline options can be combined, so rather than typing
``docker run -t -i --name test busybox sh``, you can write
``docker run -ti --name test busybox sh``.

Boolean
~~~~~~~

Boolean options look like ``-d=false``. The value you see is the
default value which gets set if you do **not** use the boolean
flag. If you do call ``run -d``, that sets the opposite boolean value,
so in this case, ``true``, and so ``docker run -d`` **will** run in
"detached" mode, in the background. Other boolean options are similar
-- specifying them will set the value to the opposite of the default
value.

Multi
~~~~~

Options like ``-a=[]`` indicate they can be specified multiple times::

  docker run -a stdin -a stdout -a stderr -i -t ubuntu /bin/bash

Sometimes this can use a more complex value string, as for ``-v``::

  docker run -v /host:/container example/mysql

Strings and Integers
~~~~~~~~~~~~~~~~~~~~

Options like ``--name=""`` expect a string, and they can only be
specified once. Options like ``-c=0`` expect an integer, and they can
only be specified once.

----

Commands
--------

.. _cli_daemon:

``daemon``
----------

::

    Usage of docker:
      -D, --debug=false: Enable debug mode
      -H, --host=[]: Multiple tcp://host:port or unix://path/to/socket to bind in daemon mode, single connection otherwise. systemd socket activation can be used with fd://[socketfd].
      -G, --group="docker": Group to assign the unix socket specified by -H when running in daemon mode; use '' (the empty string) to disable setting of a group
      --api-enable-cors=false: Enable CORS headers in the remote API
      -b, --bridge="": Attach containers to a pre-existing network bridge; use 'none' to disable container networking
      --bip="": Use this CIDR notation address for the network bridge's IP, not compatible with -b
      -d, --daemon=false: Enable daemon mode
      --dns=[]: Force docker to use specific DNS servers
      --dns-search=[]: Force Docker to use specific DNS search domains
      -g, --graph="/var/lib/docker": Path to use as the root of the docker runtime
      --icc=true: Enable inter-container communication
      --ip="0.0.0.0": Default IP address to use when binding container ports
      --ip-forward=true: Enable net.ipv4.ip_forward
      --iptables=true: Enable Docker's addition of iptables rules
      -p, --pidfile="/var/run/docker.pid": Path to use for daemon PID file
      -r, --restart=true: Restart previously running containers
      -s, --storage-driver="": Force the docker runtime to use a specific storage driver
      -e, --exec-driver="native": Force the docker runtime to use a specific exec driver
      -v, --version=false: Print version information and quit
      --tls=false: Use TLS; implied by tls-verify flags
      --tlscacert="~/.docker/ca.pem": Trust only remotes providing a certificate signed by the CA given here
      --tlscert="~/.docker/cert.pem": Path to TLS certificate file
      --tlskey="~/.docker/key.pem": Path to TLS key file
      --tlsverify=false: Use TLS and verify the remote (daemon: verify client, client: verify daemon)
      --mtu=0: Set the containers network MTU; if no value is provided: default to the default route MTU or 1500 if no default route is available

The Docker daemon is the persistent process that manages containers.  Docker uses the same binary for both the
daemon and client.  To run the daemon you provide the ``-d`` flag.

To force Docker to use devicemapper as the storage driver, use ``docker -d -s devicemapper``.

To set the DNS server for all Docker containers, use ``docker -d --dns 8.8.8.8``.

To set the DNS search domain for all Docker containers, use ``docker -d --dns-search example.com``.

To run the daemon with debug output, use ``docker -d -D``.

To use lxc as the execution driver, use ``docker -d -e lxc``.

The docker client will also honor the ``DOCKER_HOST`` environment variable to set
the ``-H`` flag for the client.

::

        docker -H tcp://0.0.0.0:4243 ps
        # or
        export DOCKER_HOST="tcp://0.0.0.0:4243"
        docker ps
        # both are equal

To run the daemon with `systemd socket activation <http://0pointer.de/blog/projects/socket-activation.html>`_, use ``docker -d -H fd://``.
Using ``fd://`` will work perfectly for most setups but you can also specify individual sockets too ``docker -d -H fd://3``.
If the specified socket activated files aren't found then docker will exit.
You can find examples of using systemd socket activation with docker and systemd in the `docker source tree <https://github.com/dotcloud/docker/blob/master/contrib/init/systemd/socket-activation/>`_.

Docker supports softlinks for the Docker data directory (``/var/lib/docker``) and for ``/tmp``.
TMPDIR and the data directory can be set like this:

::

    TMPDIR=/mnt/disk2/tmp /usr/local/bin/docker -d -D -g /var/lib/docker -H unix:// > /var/lib/boot2docker/docker.log 2>&1
    # or
    export TMPDIR=/mnt/disk2/tmp
    /usr/local/bin/docker -d -D -g /var/lib/docker -H unix:// > /var/lib/boot2docker/docker.log 2>&1

.. _cli_attach:

``attach``
----------

::

    Usage: docker attach CONTAINER

    Attach to a running container.

      --no-stdin=false: Do not attach stdin
      --sig-proxy=true: Proxify all received signal to the process (even in non-tty mode)

You can detach from the container again (and leave it running) with
``CTRL-c`` (for a quiet exit) or ``CTRL-\`` to get a stacktrace of
the Docker client when it quits.  When you detach from the container's
process the exit code will be returned to the client.

To stop a container, use ``docker stop``.

To kill the container, use ``docker kill``.

.. _cli_attach_examples:

Examples:
~~~~~~~~~

.. code-block:: bash

     $ ID=$(sudo docker run -d ubuntu /usr/bin/top -b)
     $ sudo docker attach $ID
     top - 02:05:52 up  3:05,  0 users,  load average: 0.01, 0.02, 0.05
     Tasks:   1 total,   1 running,   0 sleeping,   0 stopped,   0 zombie
     Cpu(s):  0.1%us,  0.2%sy,  0.0%ni, 99.7%id,  0.0%wa,  0.0%hi,  0.0%si,  0.0%st
     Mem:    373572k total,   355560k used,    18012k free,    27872k buffers
     Swap:   786428k total,        0k used,   786428k free,   221740k cached

     PID USER      PR  NI  VIRT  RES  SHR S %CPU %MEM    TIME+  COMMAND
      1 root      20   0 17200 1116  912 R    0  0.3   0:00.03 top

      top - 02:05:55 up  3:05,  0 users,  load average: 0.01, 0.02, 0.05
      Tasks:   1 total,   1 running,   0 sleeping,   0 stopped,   0 zombie
      Cpu(s):  0.0%us,  0.2%sy,  0.0%ni, 99.8%id,  0.0%wa,  0.0%hi,  0.0%si,  0.0%st
      Mem:    373572k total,   355244k used,    18328k free,    27872k buffers
      Swap:   786428k total,        0k used,   786428k free,   221776k cached

        PID USER      PR  NI  VIRT  RES  SHR S %CPU %MEM    TIME+  COMMAND
	    1 root      20   0 17208 1144  932 R    0  0.3   0:00.03 top


      top - 02:05:58 up  3:06,  0 users,  load average: 0.01, 0.02, 0.05
      Tasks:   1 total,   1 running,   0 sleeping,   0 stopped,   0 zombie
      Cpu(s):  0.2%us,  0.3%sy,  0.0%ni, 99.5%id,  0.0%wa,  0.0%hi,  0.0%si,  0.0%st
      Mem:    373572k total,   355780k used,    17792k free,    27880k buffers
      Swap:   786428k total,        0k used,   786428k free,   221776k cached

      PID USER      PR  NI  VIRT  RES  SHR S %CPU %MEM    TIME+  COMMAND
           1 root      20   0 17208 1144  932 R    0  0.3   0:00.03 top
     ^C$
     $ sudo docker stop $ID

.. _cli_build:

``build``
---------

::

    Usage: docker build [OPTIONS] PATH | URL | -
    Build a new container image from the source code at PATH
      -t, --tag="": Repository name (and optionally a tag) to be applied
             to the resulting image in case of success.
      -q, --quiet=false: Suppress the verbose output generated by the containers.
      --no-cache: Do not use the cache when building the image.
      --rm=true: Remove intermediate containers after a successful build

The files at ``PATH`` or ``URL`` are called the "context" of the build.
The build process may refer to any of the files in the context, for example when
using an :ref:`ADD <dockerfile_add>` instruction.
When a single ``Dockerfile`` is given as ``URL``, then no context is set.

When a Git repository is set as ``URL``, then the repository is used as the context. 
The Git repository is cloned with its submodules (`git clone --recursive`).
A fresh git clone occurs in a temporary directory on your local host, and then this 
is sent to the Docker daemon as the context. 
This way, your local user credentials and vpn's etc can be used to access private repositories

.. _cli_build_examples:

.. seealso:: :ref:`dockerbuilder`.

Examples:
~~~~~~~~~

.. code-block:: bash

    $ sudo docker build .
    Uploading context 10240 bytes
    Step 1 : FROM busybox
    Pulling repository busybox
     ---> e9aa60c60128MB/2.284 MB (100%) endpoint: https://cdn-registry-1.docker.io/v1/
    Step 2 : RUN ls -lh /
     ---> Running in 9c9e81692ae9
    total 24
    drwxr-xr-x    2 root     root        4.0K Mar 12  2013 bin
    drwxr-xr-x    5 root     root        4.0K Oct 19 00:19 dev
    drwxr-xr-x    2 root     root        4.0K Oct 19 00:19 etc
    drwxr-xr-x    2 root     root        4.0K Nov 15 23:34 lib
    lrwxrwxrwx    1 root     root           3 Mar 12  2013 lib64 -> lib
    dr-xr-xr-x  116 root     root           0 Nov 15 23:34 proc
    lrwxrwxrwx    1 root     root           3 Mar 12  2013 sbin -> bin
    dr-xr-xr-x   13 root     root           0 Nov 15 23:34 sys
    drwxr-xr-x    2 root     root        4.0K Mar 12  2013 tmp
    drwxr-xr-x    2 root     root        4.0K Nov 15 23:34 usr
     ---> b35f4035db3f
    Step 3 : CMD echo Hello World
     ---> Running in 02071fceb21b
     ---> f52f38b7823e
    Successfully built f52f38b7823e
    Removing intermediate container 9c9e81692ae9
    Removing intermediate container 02071fceb21b


This example specifies that the ``PATH`` is ``.``, and so all the files in
the local directory get tar'd and sent to the Docker daemon.  The ``PATH``
specifies where to find the files for the "context" of the build on
the Docker daemon. Remember that the daemon could be running on a
remote machine and that no parsing of the ``Dockerfile`` happens at the
client side (where you're running ``docker build``). That means that
*all* the files at ``PATH`` get sent, not just the ones listed to
:ref:`ADD <dockerfile_add>` in the ``Dockerfile``.

The transfer of context from the local machine to the Docker daemon is
what the ``docker`` client means when you see the "Uploading context"
message.

If you wish to keep the intermediate containers after the build is complete,
you must use ``--rm=false``. This does not affect the build cache.


.. code-block:: bash

   $ sudo docker build -t vieux/apache:2.0 .

This will build like the previous example, but it will then tag the
resulting image. The repository name will be ``vieux/apache`` and the
tag will be ``2.0``


.. code-block:: bash

    $ sudo docker build - < Dockerfile

This will read a ``Dockerfile`` from *stdin* without context. Due to
the lack of a context, no contents of any local directory will be sent
to the ``docker`` daemon.  Since there is no context, a ``Dockerfile``
``ADD`` only works if it refers to a remote URL.

.. code-block:: bash

    $ sudo docker build github.com/creack/docker-firefox

This will clone the GitHub repository and use the cloned repository as
context. The ``Dockerfile`` at the root of the repository is used as
``Dockerfile``.  Note that you can specify an arbitrary Git repository
by using the ``git://`` schema.


.. _cli_commit:

``commit``
----------

::

    Usage: docker commit [OPTIONS] CONTAINER [REPOSITORY[:TAG]]

    Create a new image from a container's changes

      -m, --message="": Commit message
      -a, --author="": Author (eg. "John Hannibal Smith <hannibal@a-team.com>"
      --run="": Configuration changes to be applied when the image is launched with `docker run`.
               (ex: --run='{"Cmd": ["cat", "/world"], "PortSpecs": ["22"]}')

.. _cli_commit_examples:

Commit an existing container
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: bash

	$ sudo docker ps
	ID                  IMAGE               COMMAND             CREATED             STATUS              PORTS
	c3f279d17e0a        ubuntu:12.04        /bin/bash           7 days ago          Up 25 hours
	197387f1b436        ubuntu:12.04        /bin/bash           7 days ago          Up 25 hours
	$ docker commit c3f279d17e0a  SvenDowideit/testimage:version3
	f5283438590d
	$ docker images | head
	REPOSITORY                        TAG                 ID                  CREATED             VIRTUAL SIZE
	SvenDowideit/testimage            version3            f5283438590d        16 seconds ago      335.7 MB

Change the command that a container runs
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Sometimes you have an application container running just a service and you need
to make a quick change and then change it back.

In this example, we run a container with ``ls`` and then change the image to
run ``ls /etc``.

.. code-block:: bash

        $ docker run -t --name test ubuntu ls
        bin  boot  dev  etc  home  lib  lib64  media  mnt  opt  proc  root  run  sbin  selinux  srv  sys  tmp  usr  var
        $ docker commit --run='{"Cmd": ["ls","/etc"]}' test test2
        933d16de9e70005304c1717b5c6f2f39d6fd50752834c6f34a155c70790011eb
        $ docker run -t test2
        adduser.conf            gshadow          login.defs           rc0.d
        alternatives            gshadow-         logrotate.d          rc1.d
        apt                     host.conf        lsb-base             rc2.d
        ...

Merged configs example
......................

Say you have a Dockerfile like so:

.. code-block:: bash

        ENV MYVAR foobar
        RUN apt-get install openssh
        EXPOSE 22
        CMD ["/usr/sbin/sshd -D"]
        ...

If you run that, make some changes, and then commit, Docker will merge the environment variable and exposed port configuration settings with any that you specify in the --run= option. This is a change from Docker 0.8.0 and prior where no attempt was made to preserve any existing configuration on commit.

.. code-block:: bash

        $ docker build -t me/foo .
        $ docker run -t -i me/foo /bin/bash
        foo-container$ [make changes in the container]
        foo-container$ exit
        $ docker commit --run='{"Cmd": ["ls"]}' [container-id] me/bar
        ...

The me/bar image will now have port 22 exposed, MYVAR env var set to 'foobar', and its default command will be ["ls"].

Note that this is currently a shallow merge. So, for example, if you had specified a new port spec in the --run= config above, that would have clobbered the 'EXPOSE 22' setting from the parent container.

Full --run example
..................

The ``--run`` JSON hash changes the ``Config`` section when running ``docker inspect CONTAINERID``
or ``config`` when running ``docker inspect IMAGEID``. Existing configuration key-values that are
not overridden in the JSON hash will be merged in.

(Multiline is okay within a single quote ``'``)

.. code-block:: bash

  $ sudo docker commit --run='
  {
      "Entrypoint" : null,
      "Privileged" : false,
      "User" : "",
      "VolumesFrom" : "",
      "Cmd" : ["cat", "-e", "/etc/resolv.conf"],
      "Dns" : ["8.8.8.8", "8.8.4.4"],
      "DnsSearch" : ["example.com"],
      "MemorySwap" : 0,
      "AttachStdin" : false,
      "AttachStderr" : false,
      "CpuShares" : 0,
      "OpenStdin" : false,
      "Volumes" : null,
      "Hostname" : "122612f45831",
      "PortSpecs" : ["22", "80", "443"],
      "Image" : "b750fe79269d2ec9a3c593ef05b4332b1d1a02a62b4accb2c21d589ff2f5f2dc",
      "Tty" : false,
      "Env" : [
         "HOME=/",
         "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
      ],
      "StdinOnce" : false,
      "Domainname" : "",
      "WorkingDir" : "/",
      "NetworkDisabled" : false,
      "Memory" : 0,
      "AttachStdout" : false
  }' $CONTAINER_ID

.. _cli_cp:

``cp``
------

::

    Usage: docker cp CONTAINER:PATH HOSTPATH

    Copy files/folders from the containers filesystem to the host
    path.  Paths are relative to the root of the filesystem.

.. code-block:: bash

    $ sudo docker cp 7bb0e258aefe:/etc/debian_version .
    $ sudo docker cp blue_frog:/etc/hosts .

.. _cli_diff:

``diff``
--------

::

    Usage: docker diff CONTAINER

    List the changed files and directories in a container's filesystem

There are 3 events that are listed in the 'diff':

1. ```A``` - Add
2. ```D``` - Delete
3. ```C``` - Change

For example:

.. code-block:: bash

	$ sudo docker diff 7bb0e258aefe

	C /dev
	A /dev/kmsg
	C /etc
	A /etc/mtab
	A /go
	A /go/src
	A /go/src/github.com
	A /go/src/github.com/dotcloud
	A /go/src/github.com/dotcloud/docker
	A /go/src/github.com/dotcloud/docker/.git
	....

.. _cli_events:

``events``
----------

::

    Usage: docker events

    Get real time events from the server

    --since="": Show previously created events and then stream.
               (either seconds since epoch, or date string as below)

.. _cli_events_example:

Examples
~~~~~~~~

You'll need two shells for this example.

Shell 1: Listening for events
.............................

.. code-block:: bash

    $ sudo docker events

Shell 2: Start and Stop a Container
...................................

.. code-block:: bash

    $ sudo docker start 4386fb97867d
    $ sudo docker stop 4386fb97867d

Shell 1: (Again .. now showing events)
......................................

.. code-block:: bash

    [2013-09-03 15:49:26 +0200 CEST] 4386fb97867d: (from 12de384bfb10) start
    [2013-09-03 15:49:29 +0200 CEST] 4386fb97867d: (from 12de384bfb10) die
    [2013-09-03 15:49:29 +0200 CEST] 4386fb97867d: (from 12de384bfb10) stop

Show events in the past from a specified time
.............................................

.. code-block:: bash

    $ sudo docker events --since 1378216169
    [2013-09-03 15:49:29 +0200 CEST] 4386fb97867d: (from 12de384bfb10) die
    [2013-09-03 15:49:29 +0200 CEST] 4386fb97867d: (from 12de384bfb10) stop

    $ sudo docker events --since '2013-09-03'
    [2013-09-03 15:49:26 +0200 CEST] 4386fb97867d: (from 12de384bfb10) start
    [2013-09-03 15:49:29 +0200 CEST] 4386fb97867d: (from 12de384bfb10) die
    [2013-09-03 15:49:29 +0200 CEST] 4386fb97867d: (from 12de384bfb10) stop

    $ sudo docker events --since '2013-09-03 15:49:29 +0200 CEST'
    [2013-09-03 15:49:29 +0200 CEST] 4386fb97867d: (from 12de384bfb10) die
    [2013-09-03 15:49:29 +0200 CEST] 4386fb97867d: (from 12de384bfb10) stop

.. _cli_export:

``export``
----------

::

    Usage: docker export CONTAINER

    Export the contents of a filesystem as a tar archive to STDOUT

For example:

.. code-block:: bash

    $ sudo docker export red_panda > latest.tar

.. _cli_history:

``history``
-----------

::

    Usage: docker history [OPTIONS] IMAGE

    Show the history of an image

      --no-trunc=false: Don't truncate output
      -q, --quiet=false: Only show numeric IDs

To see how the ``docker:latest`` image was built:

.. code-block:: bash

	$ docker history docker
        IMAGE                                                              CREATED             CREATED BY                                                                                                                                                 SIZE
        3e23a5875458790b7a806f95f7ec0d0b2a5c1659bfc899c89f939f6d5b8f7094   8 days ago          /bin/sh -c #(nop) ENV LC_ALL=C.UTF-8                                                                                                                       0 B
        8578938dd17054dce7993d21de79e96a037400e8d28e15e7290fea4f65128a36   8 days ago          /bin/sh -c dpkg-reconfigure locales &&    locale-gen C.UTF-8 &&    /usr/sbin/update-locale LANG=C.UTF-8                                                    1.245 MB
        be51b77efb42f67a5e96437b3e102f81e0a1399038f77bf28cea0ed23a65cf60   8 days ago          /bin/sh -c apt-get update && apt-get install -y    git    libxml2-dev    python    build-essential    make    gcc    python-dev    locales    python-pip   338.3 MB
        4b137612be55ca69776c7f30c2d2dd0aa2e7d72059820abf3e25b629f887a084   6 weeks ago         /bin/sh -c #(nop) ADD jessie.tar.xz in /                                                                                                                   121 MB
        750d58736b4b6cc0f9a9abe8f258cef269e3e9dceced1146503522be9f985ada   6 weeks ago         /bin/sh -c #(nop) MAINTAINER Tianon Gravi <admwiggin@gmail.com> - mkimage-debootstrap.sh -t jessie.tar.xz jessie http://http.debian.net/debian             0 B
        511136ea3c5a64f264b78b5433614aec563103b4d4702f3ba7d4d2698e22c158   9 months ago                                                                                                                                                                   0 B
	
.. _cli_images:

``images``
----------

::

    Usage: docker images [OPTIONS] [NAME]

    List images

      -a, --all=false: Show all images (by default filter out the intermediate images used to build)
      --no-trunc=false: Don't truncate output
      -q, --quiet=false: Only show numeric IDs
      -t, --tree=false: Output graph in tree format
      -v, --viz=false: Output graph in graphviz format

Listing the most recently created images
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: bash

	$ sudo docker images | head
	REPOSITORY                    TAG                 IMAGE ID            CREATED             VIRTUAL SIZE
	<none>                        <none>              77af4d6b9913        19 hours ago        1.089 GB
	committest                    latest              b6fa739cedf5        19 hours ago        1.089 GB
	<none>                        <none>              78a85c484f71        19 hours ago        1.089 GB
	docker                        latest              30557a29d5ab        20 hours ago        1.089 GB
	<none>                        <none>              0124422dd9f9        20 hours ago        1.089 GB
	<none>                        <none>              18ad6fad3402        22 hours ago        1.082 GB
	<none>                        <none>              f9f1e26352f0        23 hours ago        1.089 GB
	tryout                        latest              2629d1fa0b81        23 hours ago        131.5 MB
	<none>                        <none>              5ed6274db6ce        24 hours ago        1.089 GB

Listing the full length image IDs
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: bash

	$ sudo docker images --no-trunc | head
	REPOSITORY                    TAG                 IMAGE ID                                                           CREATED             VIRTUAL SIZE
	<none>                        <none>              77af4d6b9913e693e8d0b4b294fa62ade6054e6b2f1ffb617ac955dd63fb0182   19 hours ago        1.089 GB
	committest                    latest              b6fa739cedf5ea12a620a439402b6004d057da800f91c7524b5086a5e4749c9f   19 hours ago        1.089 GB
	<none>                        <none>              78a85c484f71509adeaace20e72e941f6bdd2b25b4c75da8693efd9f61a37921   19 hours ago        1.089 GB
	docker                        latest              30557a29d5abc51e5f1d5b472e79b7e296f595abcf19fe6b9199dbbc809c6ff4   20 hours ago        1.089 GB
	<none>                        <none>              0124422dd9f9cf7ef15c0617cda3931ee68346455441d66ab8bdc5b05e9fdce5   20 hours ago        1.089 GB
	<none>                        <none>              18ad6fad340262ac2a636efd98a6d1f0ea775ae3d45240d3418466495a19a81b   22 hours ago        1.082 GB
	<none>                        <none>              f9f1e26352f0a3ba6a0ff68167559f64f3e21ff7ada60366e2d44a04befd1d3a   23 hours ago        1.089 GB
	tryout                        latest              2629d1fa0b81b222fca63371ca16cbf6a0772d07759ff80e8d1369b926940074   23 hours ago        131.5 MB
	<none>                        <none>              5ed6274db6ceb2397844896966ea239290555e74ef307030ebb01ff91b1914df   24 hours ago        1.089 GB

Displaying images visually
~~~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: bash

    $ sudo docker images --viz | dot -Tpng -o docker.png

.. image:: docker_images.gif
   :alt: Example inheritance graph of Docker images.


Displaying image hierarchy
~~~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: bash

    $ sudo docker images --tree

    ├─8dbd9e392a96 Size: 131.5 MB (virtual 131.5 MB) Tags: ubuntu:12.04,ubuntu:latest,ubuntu:precise
    └─27cf78414709 Size: 180.1 MB (virtual 180.1 MB)
      └─b750fe79269d Size: 24.65 kB (virtual 180.1 MB) Tags: ubuntu:12.10,ubuntu:quantal
        ├─f98de3b610d5 Size: 12.29 kB (virtual 180.1 MB)
        │ └─7da80deb7dbf Size: 16.38 kB (virtual 180.1 MB)
        │   └─65ed2fee0a34 Size: 20.66 kB (virtual 180.2 MB)
        │     └─a2b9ea53dddc Size: 819.7 MB (virtual 999.8 MB)
        │       └─a29b932eaba8 Size: 28.67 kB (virtual 999.9 MB)
        │         └─e270a44f124d Size: 12.29 kB (virtual 999.9 MB) Tags: progrium/buildstep:latest
        └─17e74ac162d8 Size: 53.93 kB (virtual 180.2 MB)
          └─339a3f56b760 Size: 24.65 kB (virtual 180.2 MB)
            └─904fcc40e34d Size: 96.7 MB (virtual 276.9 MB)
              └─b1b0235328dd Size: 363.3 MB (virtual 640.2 MB)
                └─7cb05d1acb3b Size: 20.48 kB (virtual 640.2 MB)
                  └─47bf6f34832d Size: 20.48 kB (virtual 640.2 MB)
                    └─f165104e82ed Size: 12.29 kB (virtual 640.2 MB)
                      └─d9cf85a47b7e Size: 1.911 MB (virtual 642.2 MB)
                        └─3ee562df86ca Size: 17.07 kB (virtual 642.2 MB)
                          └─b05fc2d00e4a Size: 24.96 kB (virtual 642.2 MB)
                            └─c96a99614930 Size: 12.29 kB (virtual 642.2 MB)
                              └─a6a357a48c49 Size: 12.29 kB (virtual 642.2 MB) Tags: ndj/mongodb:latest

.. _cli_import:

``import``
----------

::

    Usage: docker import URL|- [REPOSITORY[:TAG]]

    Create an empty filesystem image and import the contents of the tarball
    (.tar, .tar.gz, .tgz, .bzip, .tar.xz, .txz) into it, then optionally tag it.

At this time, the URL must start with ``http`` and point to a single
file archive (.tar, .tar.gz, .tgz, .bzip, .tar.xz, or .txz) containing a
root filesystem. If you would like to import from a local directory or
archive, you can use the ``-`` parameter to take the data from *stdin*.

Examples
~~~~~~~~

Import from a remote location
.............................

This will create a new untagged image.

.. code-block:: bash

    $ sudo docker import http://example.com/exampleimage.tgz

Import from a local file
........................

Import to docker via pipe and *stdin*.

.. code-block:: bash

    $ cat exampleimage.tgz | sudo docker import - exampleimagelocal:new

Import from a local directory
.............................

.. code-block:: bash

    $ sudo tar -c . | docker import - exampleimagedir

Note the ``sudo`` in this example -- you must preserve the ownership of the
files (especially root ownership) during the archiving with tar. If you are not
root (or the sudo command) when you tar, then the ownerships might not get
preserved.

.. _cli_info:

``info``
--------

::

    Usage: docker info

    Display system-wide information.

.. code-block:: bash

	$ sudo docker info
	Containers: 292
	Images: 194
	Debug mode (server): false
	Debug mode (client): false
	Fds: 22
	Goroutines: 67
	LXC Version: 0.9.0
	EventsListeners: 115
	Kernel Version: 3.8.0-33-generic
	WARNING: No swap limit support


.. _cli_insert:

``insert``
----------

::

    Usage: docker insert IMAGE URL PATH

    Insert a file from URL in the IMAGE at PATH

Use the specified ``IMAGE`` as the parent for a new image which adds a
:ref:`layer <layer_def>` containing the new file. The ``insert`` command does
not modify the original image, and the new image has the contents of the parent
image, plus the new file.


Examples
~~~~~~~~

Insert file from GitHub
.......................

.. code-block:: bash

    $ sudo docker insert 8283e18b24bc https://raw.github.com/metalivedev/django/master/postinstall /tmp/postinstall.sh
    06fd35556d7b

.. _cli_inspect:

``inspect``
-----------

::

    Usage: docker inspect CONTAINER|IMAGE [CONTAINER|IMAGE...]

    Return low-level information on a container/image

      -f, --format="": Format the output using the given go template.

By default, this will render all results in a JSON array.  If a format
is specified, the given template will be executed for each result.

Go's `text/template <http://golang.org/pkg/text/template/>`_ package
describes all the details of the format.

Examples
~~~~~~~~

Get an instance's IP Address
............................

For the most part, you can pick out any field from the JSON in a
fairly straightforward manner.

.. code-block:: bash

    $ sudo docker inspect --format='{{.NetworkSettings.IPAddress}}' $INSTANCE_ID

List All Port Bindings
......................

One can loop over arrays and maps in the results to produce simple
text output:

.. code-block:: bash

    $ sudo docker inspect --format='{{range $p, $conf := .NetworkSettings.Ports}} {{$p}} -> {{(index $conf 0).HostPort}} {{end}}' $INSTANCE_ID

Find a Specific Port Mapping
............................

The ``.Field`` syntax doesn't work when the field name begins with a
number, but the template language's ``index`` function does.  The
``.NetworkSettings.Ports`` section contains a map of the internal port
mappings to a list of external address/port objects, so to grab just
the numeric public port, you use ``index`` to find the specific port
map, and then ``index`` 0 contains first object inside of that.  Then
we ask for the ``HostPort`` field to get the public address.

.. code-block:: bash

    $ sudo docker inspect --format='{{(index (index .NetworkSettings.Ports "8787/tcp") 0).HostPort}}' $INSTANCE_ID

Get config
..........

The ``.Field`` syntax doesn't work when the field contains JSON data,
but the template language's custom ``json`` function does. The ``.config``
section contains complex json object, so to grab it as JSON, you use ``json``
to convert config object into JSON

.. code-block:: bash

    $ sudo docker inspect --format='{{json .config}}' $INSTANCE_ID


.. _cli_kill:

``kill``
--------

::

    Usage: docker kill [OPTIONS] CONTAINER [CONTAINER...]

    Kill a running container (send SIGKILL, or specified signal)

      -s, --signal="KILL": Signal to send to the container

The main process inside the container will be sent SIGKILL, or any signal specified with option ``--signal``.

Known Issues (kill)
~~~~~~~~~~~~~~~~~~~

* :issue:`197` indicates that ``docker kill`` may leave directories
  behind and make it difficult to remove the container.
* :issue:`3844` lxc 1.0.0 beta3 removed ``lcx-kill`` which is used by Docker versions before 0.8.0;
  see the issue for a workaround.

.. _cli_load:

``load``
--------

::

    Usage: docker load 

    Load an image from a tar archive on STDIN

      -i, --input="": Read from a tar archive file, instead of STDIN

Loads a tarred repository from a file or the standard input stream.
Restores both images and tags.

.. code-block:: bash

   $ sudo docker images
   REPOSITORY          TAG                 IMAGE ID            CREATED             VIRTUAL SIZE
   $ sudo docker load < busybox.tar
   $ sudo docker images
   REPOSITORY          TAG                 IMAGE ID            CREATED             VIRTUAL SIZE
   busybox             latest              769b9341d937        7 weeks ago         2.489 MB
   $ sudo docker load --input fedora.tar
   $ sudo docker images
   REPOSITORY          TAG                 IMAGE ID            CREATED             VIRTUAL SIZE
   busybox             latest              769b9341d937        7 weeks ago         2.489 MB
   fedora              rawhide             0d20aec6529d        7 weeks ago         387 MB
   fedora              20                  58394af37342        7 weeks ago         385.5 MB
   fedora              heisenbug           58394af37342        7 weeks ago         385.5 MB
   fedora              latest              58394af37342        7 weeks ago         385.5 MB


.. _cli_login:

``login``
---------

::

    Usage: docker login [OPTIONS] [SERVER]

    Register or Login to the docker registry server

    -e, --email="": Email
    -p, --password="": Password
    -u, --username="": Username

    If you want to login to a private registry you can
    specify this by adding the server name.

    example:
    docker login localhost:8080


.. _cli_logs:

``logs``
--------

::

    Usage: docker logs [OPTIONS] CONTAINER

    Fetch the logs of a container

    -f, --follow=false: Follow log output

The ``docker logs`` command is a convenience which batch-retrieves whatever
logs are present at the time of execution. This does not guarantee execution
order when combined with a ``docker run`` (i.e. your run may not have generated
any logs at the time you execute ``docker logs``).

The ``docker logs --follow`` command combines ``docker logs`` and ``docker attach``:
it will first return all logs from the beginning and then continue streaming
new output from the container's stdout and stderr.


.. _cli_port:

``port``
--------

::

    Usage: docker port [OPTIONS] CONTAINER PRIVATE_PORT

    Lookup the public-facing port which is NAT-ed to PRIVATE_PORT


.. _cli_ps:

``ps``
------

::

    Usage: docker ps [OPTIONS]

    List containers

      -a, --all=false: Show all containers. Only running containers are shown by default.
      --before="": Show only container created before Id or Name, include non-running ones.
      -l, --latest=false: Show only the latest created container, include non-running ones.
      -n=-1: Show n last created containers, include non-running ones.
      --no-trunc=false: Don't truncate output
      -q, --quiet=false: Only display numeric IDs
      -s, --size=false: Display sizes, not to be used with -q
      --since="": Show only containers created since Id or Name, include non-running ones.


Running ``docker ps`` showing 2 linked containers.

.. code-block:: bash

    $ docker ps
    CONTAINER ID        IMAGE                        COMMAND                CREATED              STATUS              PORTS               NAMES
    4c01db0b339c        ubuntu:12.04                 bash                   17 seconds ago       Up 16 seconds                           webapp
    d7886598dbe2        crosbymichael/redis:latest   /redis-server --dir    33 minutes ago       Up 33 minutes       6379/tcp            redis,webapp/db
    fd2645e2e2b5        busybox:latest               top                    10 days ago          Ghost                                   insane_ptolemy

The last container is marked as a ``Ghost`` container. It is a container that was running when the docker daemon was restarted (upgraded, or ``-H`` settings changed). The container is still running, but as this docker daemon process is not able to manage it, you can't attach to it. To bring them out of ``Ghost`` Status, you need to use ``docker kill`` or ``docker restart``.

``docker ps`` will show only running containers by default.  To see all containers: ``docker ps -a``

.. _cli_pull:

``pull``
--------

::

    Usage: docker pull NAME

    Pull an image or a repository from the registry

      -t, --tag="": Download tagged image in repository


.. _cli_push:

``push``
--------

::

    Usage: docker push NAME

    Push an image or a repository to the registry


.. _cli_restart:

``restart``
-----------

::

    Usage: docker restart [OPTIONS] NAME

    Restart a running container

       -t, --time=10: Number of seconds to try to stop for before killing the container. Once killed it will then be restarted. Default=10

.. _cli_rm:

``rm``
------

::

    Usage: docker rm [OPTIONS] CONTAINER

    Remove one or more containers
        -l, --link="": Remove the link instead of the actual container
        -f, --force=false: Force removal of running container
        -v, --volumes=false: Remove the volumes associated to the container

Known Issues (rm)
~~~~~~~~~~~~~~~~~

* :issue:`197` indicates that ``docker kill`` may leave directories
  behind and make it difficult to remove the container.


Examples:
~~~~~~~~~

.. code-block:: bash

    $ sudo docker rm /redis
    /redis


This will remove the container referenced under the link ``/redis``.


.. code-block:: bash

    $ sudo docker rm --link /webapp/redis
    /webapp/redis


This will remove the underlying link between ``/webapp`` and the ``/redis`` containers removing all
network communication.

.. code-block:: bash

    $ sudo docker rm `docker ps -a -q`


This command will delete all stopped containers. The command ``docker ps -a -q`` will return all
existing container IDs and pass them to the ``rm`` command which will delete them. Any running
containers will not be deleted.

.. _cli_rmi:

``rmi``
-------

::

    Usage: docker rmi IMAGE [IMAGE...]

    Remove one or more images

      -f, --force=false: Force
      --no-prune=false: Do not delete untagged parents

Removing tagged images
~~~~~~~~~~~~~~~~~~~~~~

Images can be removed either by their short or long ID's, or their image names.
If an image has more than one name, each of them needs to be removed before the
image is removed.

.. code-block:: bash

    $ sudo docker images
    REPOSITORY                TAG                 IMAGE ID            CREATED             SIZE
    test1                     latest              fd484f19954f        23 seconds ago      7 B (virtual 4.964 MB)
    test                      latest              fd484f19954f        23 seconds ago      7 B (virtual 4.964 MB)
    test2                     latest              fd484f19954f        23 seconds ago      7 B (virtual 4.964 MB)

    $ sudo docker rmi fd484f19954f
    Error: Conflict, cannot delete image fd484f19954f because it is tagged in multiple repositories
    2013/12/11 05:47:16 Error: failed to remove one or more images

    $ sudo docker rmi test1
    Untagged: fd484f19954f4920da7ff372b5067f5b7ddb2fd3830cecd17b96ea9e286ba5b8
    $ sudo docker rmi test2
    Untagged: fd484f19954f4920da7ff372b5067f5b7ddb2fd3830cecd17b96ea9e286ba5b8

    $ sudo docker images
    REPOSITORY                TAG                 IMAGE ID            CREATED             SIZE
    test1                     latest              fd484f19954f        23 seconds ago      7 B (virtual 4.964 MB)
    $ sudo docker rmi test
    Untagged: fd484f19954f4920da7ff372b5067f5b7ddb2fd3830cecd17b96ea9e286ba5b8
    Deleted: fd484f19954f4920da7ff372b5067f5b7ddb2fd3830cecd17b96ea9e286ba5b8


.. _cli_run:

``run``
-------

::

    Usage: docker run [OPTIONS] IMAGE[:TAG] [COMMAND] [ARG...]

    Run a command in a new container

      -a, --attach=map[]: Attach to stdin, stdout or stderr
      -c, --cpu-shares=0: CPU shares (relative weight)
      --cidfile="": Write the container ID to the file
      -d, --detach=false: Detached mode: Run container in the background, print new container id
      -e, --env=[]: Set environment variables
      -h, --hostname="": Container host name
      -i, --interactive=false: Keep stdin open even if not attached
      --privileged=false: Give extended privileges to this container
      -m, --memory="": Memory limit (format: <number><optional unit>, where unit = b, k, m or g)
      -n, --networking=true: Enable networking for this container
      -p, --publish=[]: Map a network port to the container
      --rm=false: Automatically remove the container when it exits (incompatible with -d)
      -t, --tty=false: Allocate a pseudo-tty
      -u, --user="": Username or UID
      --dns=[]: Set custom dns servers for the container
      --dns-search=[]: Set custom DNS search domains for the container
      -v, --volume=[]: Create a bind mount to a directory or file with: [host-path]:[container-path]:[rw|ro]. If a directory "container-path" is missing, then docker creates a new volume.
      --volumes-from="": Mount all volumes from the given container(s)
      --entrypoint="": Overwrite the default entrypoint set by the image
      -w, --workdir="": Working directory inside the container
      --lxc-conf=[]: (lxc exec-driver only) Add custom lxc options --lxc-conf="lxc.cgroup.cpuset.cpus = 0,1"
      --sig-proxy=true: Proxify all received signal to the process (even in non-tty mode)
      --expose=[]: Expose a port from the container without publishing it to your host
      --link="": Add link to another container (name:alias)
      --name="": Assign the specified name to the container. If no name is specific docker will generate a random name
      -P, --publish-all=false: Publish all exposed ports to the host interfaces

The ``docker run`` command first ``creates`` a writeable container layer over
the specified image, and then ``starts`` it using the specified command. That
is, ``docker run`` is equivalent to the API ``/containers/create`` then
``/containers/(id)/start``.
Once the container is stopped it still exists and can be started back up.  See ``docker ps -a`` to view a list of all containers.

The ``docker run`` command can be used in combination with ``docker commit`` to
:ref:`change the command that a container runs <cli_commit_examples>`.

See :ref:`port_redirection` for more detailed information about the ``--expose``,
``-p``, ``-P`` and ``--link`` parameters, and :ref:`working_with_links_names` for
specific examples using ``--link``.

Known Issues (run --volumes-from)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

* :issue:`2702`: "lxc-start: Permission denied - failed to mount"
  could indicate a permissions problem with AppArmor. Please see the
  issue for a workaround.

Examples:
~~~~~~~~~

.. code-block:: bash

    $ sudo docker run --cidfile /tmp/docker_test.cid ubuntu echo "test"

This will create a container and print ``test`` to the console. The
``cidfile`` flag makes Docker attempt to create a new file and write the
container ID to it. If the file exists already, Docker will return an
error. Docker will close this file when ``docker run`` exits.

.. code-block:: bash

   $ sudo docker run -t -i --rm ubuntu bash
   root@bc338942ef20:/# mount -t tmpfs none /mnt
   mount: permission denied


This will *not* work, because by default, most potentially dangerous
kernel capabilities are dropped; including ``cap_sys_admin`` (which is
required to mount filesystems). However, the ``--privileged`` flag will
allow it to run:

.. code-block:: bash

   $ sudo docker run --privileged ubuntu bash
   root@50e3f57e16e6:/# mount -t tmpfs none /mnt
   root@50e3f57e16e6:/# df -h
   Filesystem      Size  Used Avail Use% Mounted on
   none            1.9G     0  1.9G   0% /mnt


The ``--privileged`` flag gives *all* capabilities to the container,
and it also lifts all the limitations enforced by the ``device``
cgroup controller. In other words, the container can then do almost
everything that the host can do. This flag exists to allow special
use-cases, like running Docker within Docker.

.. code-block:: bash

   $ sudo docker  run -w /path/to/dir/ -i -t  ubuntu pwd

The ``-w`` lets the command being executed inside directory given,
here ``/path/to/dir/``. If the path does not exists it is created inside the
container.

.. code-block:: bash

   $ sudo docker  run  -v `pwd`:`pwd` -w `pwd` -i -t  ubuntu pwd

The ``-v`` flag mounts the current working directory into the container.
The ``-w`` lets the command being executed inside the current
working directory, by changing into the directory to the value
returned by ``pwd``. So this combination executes the command
using the container, but inside the current working directory.

.. code-block:: bash

    $ sudo docker run -v /doesnt/exist:/foo -w /foo -i -t ubuntu bash

When the host directory of a bind-mounted volume doesn't exist, Docker
will automatically create this directory on the host for you. In the
example above, Docker will create the ``/doesnt/exist`` folder before
starting your container.

.. code-block:: bash

   $ sudo docker run -t -i -v /var/run/docker.sock:/var/run/docker.sock -v ./static-docker:/usr/bin/docker busybox sh

By bind-mounting the docker unix socket and statically linked docker binary
(such as that provided by https://get.docker.io), you give the container
the full access to create and manipulate the host's docker daemon.

.. code-block:: bash

    $ sudo docker run -p 127.0.0.1:80:8080 ubuntu bash

This binds port ``8080`` of the container to port ``80`` on ``127.0.0.1`` of the
host machine. :ref:`port_redirection` explains in detail how to manipulate ports
in Docker.

.. code-block:: bash

    $ sudo docker run --expose 80 ubuntu bash

This exposes port ``80`` of the container for use within a link without
publishing the port to the host system's interfaces. :ref:`port_redirection`
explains in detail how to manipulate ports in Docker.

.. code-block:: bash

    $ sudo docker run --name console -t -i ubuntu bash

This will create and run a new container with the container name
being ``console``.

.. code-block:: bash

    $ sudo docker run --link /redis:redis --name console ubuntu bash

The ``--link`` flag will link the container named ``/redis`` into the
newly created container with the alias ``redis``.  The new container
can access the network and environment of the redis container via
environment variables.  The ``--name`` flag will assign the name ``console``
to the newly created container.

.. code-block:: bash

   $ sudo docker run --volumes-from 777f7dc92da7,ba8c0c54f0f2:ro -i -t ubuntu pwd

The ``--volumes-from`` flag mounts all the defined volumes from the
referenced containers. Containers can be specified by a comma separated
list or by repetitions of the ``--volumes-from`` argument. The container
ID may be optionally suffixed with ``:ro`` or ``:rw`` to mount the volumes in
read-only or read-write mode, respectively. By default, the volumes are mounted
in the same mode (read write or read only) as the reference container.

A complete example
..................

.. code-block:: bash

   $ sudo docker run -d --name static static-web-files sh
   $ sudo docker run -d --expose=8098 --name riak riakserver
   $ sudo docker run -d -m 100m -e DEVELOPMENT=1 -e BRANCH=example-code -v $(pwd):/app/bin:ro --name app appserver
   $ sudo docker run -d -p 1443:443 --dns=dns.dev.org --dns-search=dev.org -v /var/log/httpd --volumes-from static --link riak --link app -h www.sven.dev.org --name web webserver
   $ sudo docker run -t -i --rm --volumes-from web -w /var/log/httpd busybox tail -f access.log

This example shows 5 containers that might be set up to test a web application change:

1. Start a pre-prepared volume image ``static-web-files`` (in the background) that has CSS, image and static HTML in it, (with a ``VOLUME`` instruction in the ``Dockerfile`` to allow the web server to use those files);
2. Start a pre-prepared ``riakserver`` image, give the container name ``riak`` and expose port ``8098`` to any containers that link to it;
3. Start the ``appserver`` image, restricting its memory usage to 100MB, setting two environment variables ``DEVELOPMENT`` and ``BRANCH`` and bind-mounting the current directory (``$(pwd)``) in the container in read-only mode as ``/app/bin``;
4. Start the ``webserver``, mapping port ``443`` in the container to port ``1443`` on the Docker server, setting the DNS server to ``dns.dev.org`` and DNS search domain to ``dev.org``, creating a volume to put the log files into (so we can access it from another container), then importing the files from the volume exposed by the ``static`` container, and linking to all exposed ports from ``riak`` and ``app``. Lastly, we set the hostname to ``web.sven.dev.org`` so its consistent with the pre-generated SSL certificate;
5. Finally, we create a container that runs ``tail -f access.log`` using the logs volume from the ``web`` container, setting the workdir to ``/var/log/httpd``. The ``--rm`` option means that when the container exits, the container's layer is removed.


.. _cli_save:

``save``
---------

::

    Usage: docker save IMAGE

    Save an image to a tar archive (streamed to stdout by default)

      -o, --output="": Write to an file, instead of STDOUT


Produces a tarred repository to the standard output stream.
Contains all parent layers, and all tags + versions, or specified repo:tag.

.. code-block:: bash

   $ sudo docker save busybox > busybox.tar
   $ ls -sh b.tar
   2.7M b.tar
   $ sudo docker save --output busybox.tar busybox
   $ ls -sh b.tar
   2.7M b.tar
   $ sudo docker save -o fedora-all.tar fedora
   $ sudo docker save -o fedora-latest.tar fedora:latest


.. _cli_search:

``search``
----------

::

    Usage: docker search TERM

    Search the docker index for images

     --no-trunc=false: Don't truncate output
     -s, --stars=0: Only displays with at least xxx stars
     -t, --trusted=false: Only show trusted builds

.. _cli_start:

``start``
---------

::

    Usage: docker start [OPTIONS] CONTAINER

    Start a stopped container

      -a, --attach=false: Attach container's stdout/stderr and forward all signals to the process
      -i, --interactive=false: Attach container's stdin

.. _cli_stop:

``stop``
--------

::

    Usage: docker stop [OPTIONS] CONTAINER [CONTAINER...]

    Stop a running container (Send SIGTERM, and then SIGKILL after grace period)

      -t, --time=10: Number of seconds to wait for the container to stop before killing it.

The main process inside the container will receive SIGTERM, and after a grace period, SIGKILL

.. _cli_tag:

``tag``
-------

::

    Usage: docker tag [OPTIONS] IMAGE [REGISTRYHOST/][USERNAME/]NAME[:TAG]

    Tag an image into a repository

      -f, --force=false: Force

.. _cli_top:

``top``
-------

::

    Usage: docker top CONTAINER [ps OPTIONS]

    Lookup the running processes of a container

.. _cli_version:

``version``
-----------

Show the version of the Docker client, daemon, and latest released version.


.. _cli_wait:

``wait``
--------

::

    Usage: docker wait [OPTIONS] NAME

    Block until a container stops, then print its exit code.
