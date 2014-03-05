## libcontainer - reference implementation for containers

#### background

libcontainer specifies configuration options for what a container is.  It provides a native Go implementation 
for using linux namespaces with no external dependencies.  libcontainer provides many convience functions for working with namespaces, networking, and management.  


#### container
A container is a self contained directory that is able to run one or more processes without 
affecting the host system.  The directory is usually a full system tree.  Inside the directory
a `container.json` file is placed with the runtime configuration for how the processes 
should be contained and ran.  Environment, networking, and different capabilities for the 
process are specified in this file.  The configuration is used for each process executed inside the container.

Sample `container.json` file:
```json
{
    "hostname": "koye",
    "tty": true,
    "environment": [
        "HOME=/",
        "PATH=PATH=$PATH:/bin:/usr/bin:/sbin:/usr/sbin",
        "container=docker",
        "TERM=xterm-256color"
    ],
    "namespaces": [
        "NEWIPC",
        "NEWNS",
        "NEWPID",
        "NEWUTS",
        "NEWNET"
    ],
    "capabilities": [
        "SETPCAP",
        "SYS_MODULE",
        "SYS_RAWIO",
        "SYS_PACCT",
        "SYS_ADMIN",
        "SYS_NICE",
        "SYS_RESOURCE",
        "SYS_TIME",
        "SYS_TTY_CONFIG",
        "MKNOD",
        "AUDIT_WRITE",
        "AUDIT_CONTROL",
        "MAC_OVERRIDE",
        "MAC_ADMIN",
        "NET_ADMIN"
    ],
    "networks": [{
            "type": "veth",
            "context": {
                "bridge": "docker0",
                "prefix": "dock"
            },
            "address": "172.17.0.100/16",
            "gateway": "172.17.42.1",
            "mtu": 1500
        }
    ],
    "cgroups": {
        "name": "docker-koye",
        "parent": "docker",
        "memory": 5248000
    }
}
```

Using this configuration and the current directory holding the rootfs for a process, one can use libcontainer to exec the container. Running the life of the namespace, a `pid` file 
is written to the current directory with the pid of the namespaced process to the external world.  A client can use this pid to wait, kill, or perform other operation with the container.  If a user tries to run an new process inside an existing container with a live namespace the namespace will be joined by the new process.


You may also specify an alternate root place where the `container.json` file is read and where the `pid` file will be saved.

#### nsinit

`nsinit` is a cli application used as the reference implementation of libcontainer.  It is able to 
spawn or join new containers giving the current directory.  To use `nsinit` cd into a linux 
rootfs and copy a `container.json` file into the directory with your specified configuration.

To execute `/bin/bash` in the current directory as a container just run:
```bash
nsinit exec /bin/bash
```

If you wish to spawn another process inside the container while your current bash session is 
running just run the exact same command again to get another bash shell or change the command.  If the original process dies, PID 1, all other processes spawned inside the container will also be killed and the namespace will be removed. 

You can identify if a process is running in a container by looking to see if `pid` is in the root of the directory.   
