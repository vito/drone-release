:title: Registry API
:description: API Documentation for Docker Registry
:keywords: API, Docker, index, registry, REST, documentation

===================
Docker Registry API
===================


1. Brief introduction
=====================

- This is the REST API for the Docker Registry
- It stores the images and the graph for a set of repositories
- It does not have user accounts data
- It has no notion of user accounts or authorization
- It delegates authentication and authorization to the Index Auth service using tokens
- It supports different storage backends (S3, cloud files, local FS)
- It doesn’t have a local database
- It will be open-sourced at some point

We expect that there will be multiple registries out there. To help to grasp
the context, here are some examples of registries:

- **sponsor registry**: such a registry is provided by a third-party hosting infrastructure as a convenience for their customers and the docker community as a whole. Its costs are supported by the third party, but the management and operation of the registry are supported by dotCloud. It features read/write access, and delegates authentication and authorization to the Index.
- **mirror registry**: such a registry is provided by a third-party hosting infrastructure but is targeted at their customers only. Some mechanism (unspecified to date) ensures that public images are pulled from a sponsor registry to the mirror registry, to make sure that the customers of the third-party provider can “docker pull” those images locally.
- **vendor registry**: such a registry is provided by a software vendor, who wants to distribute docker images. It would be operated and managed by the vendor. Only users authorized by the vendor would be able to get write access. Some images would be public (accessible for anyone), others private (accessible only for authorized users). Authentication and authorization would be delegated to the Index. The goal of vendor registries is to let someone do “docker pull basho/riak1.3” and automatically push from the vendor registry (instead of a sponsor registry); i.e. get all the convenience of a sponsor registry, while retaining control on the asset distribution.
- **private registry**: such a registry is located behind a firewall, or protected by an additional security layer (HTTP authorization, SSL client-side certificates, IP address authorization...). The registry is operated by a private entity, outside of dotCloud’s control. It can optionally delegate additional authorization to the Index, but it is not mandatory.

.. note::

    Mirror registries and private registries which do not use the Index don’t even need to run the registry code. They can be implemented by any kind of transport implementing HTTP GET and PUT. Read-only registries can be powered by a simple static HTTP server.

.. note::

    The latter implies that while HTTP is the protocol of choice for a registry, multiple schemes are possible (and in some cases, trivial):
        - HTTP with GET (and PUT for read-write registries);
        - local mount point;
        - remote docker addressed through SSH.

The latter would only require two new commands in docker, e.g. ``registryget``
and ``registryput``, wrapping access to the local filesystem (and optionally
doing consistency checks). Authentication and authorization are then delegated
to SSH (e.g. with public keys).

2. Endpoints
============

2.1 Images
----------

Layer
*****

.. http:get:: /v1/images/(image_id)/layer 

    get image layer for a given ``image_id``

    **Example Request**:

    .. sourcecode:: http

        GET /v1/images/088b4505aa3adc3d35e79c031fa126b403200f02f51920fbd9b7c503e87c7a2c/layer HTTP/1.1
        Host: registry-1.docker.io
        Accept: application/json
        Content-Type: application/json
        Authorization: Token signature=123abc,repository="foo/bar",access=read

    :parameter image_id: the id for the layer you want to get

    **Example Response**:

    .. sourcecode:: http

        HTTP/1.1 200
        Vary: Accept
        X-Docker-Registry-Version: 0.6.0
        Cookie: (Cookie provided by the Registry)

        {layer binary data stream}

    :statuscode 200: OK
    :statuscode 401: Requires authorization
    :statuscode 404: Image not found


.. http:put:: /v1/images/(image_id)/layer 

    put image layer for a given ``image_id``

    **Example Request**:

    .. sourcecode:: http

        PUT /v1/images/088b4505aa3adc3d35e79c031fa126b403200f02f51920fbd9b7c503e87c7a2c/layer HTTP/1.1
        Host: registry-1.docker.io
        Transfer-Encoding: chunked
        Authorization: Token signature=123abc,repository="foo/bar",access=write

        {layer binary data stream}

    :parameter image_id: the id for the layer you want to get


    **Example Response**:

    .. sourcecode:: http
    
        HTTP/1.1 200
        Vary: Accept
        Content-Type: application/json
        X-Docker-Registry-Version: 0.6.0

        ""

    :statuscode 200: OK
    :statuscode 401: Requires authorization
    :statuscode 404: Image not found


Image
*****

.. http:put:: /v1/images/(image_id)/json

    put image for a given ``image_id``

    **Example Request**:

    .. sourcecode:: http

        PUT /v1/images/088b4505aa3adc3d35e79c031fa126b403200f02f51920fbd9b7c503e87c7a2c/json HTTP/1.1
        Host: registry-1.docker.io
        Accept: application/json
        Content-Type: application/json
        Cookie: (Cookie provided by the Registry)

        {
            id: "088b4505aa3adc3d35e79c031fa126b403200f02f51920fbd9b7c503e87c7a2c",
            parent: "aeee6396d62273d180a49c96c62e45438d87c7da4a5cf5d2be6bee4e21bc226f",
            created: "2013-04-30T17:46:10.843673+03:00",
            container: "8305672a76cc5e3d168f97221106ced35a76ec7ddbb03209b0f0d96bf74f6ef7",
            container_config: {
                Hostname: "host-test",
                User: "",
                Memory: 0,
                MemorySwap: 0,
                AttachStdin: false,
                AttachStdout: false,
                AttachStderr: false,
                PortSpecs: null,
                Tty: false,
                OpenStdin: false,
                StdinOnce: false,
                Env: null,
                Cmd: [
                "/bin/bash",
                "-c",
                "apt-get -q -yy -f install libevent-dev"
                ],
                Dns: null,
                Image: "imagename/blah",
                Volumes: { },
                VolumesFrom: ""
            },
            docker_version: "0.1.7"
        }

    :parameter image_id: the id for the layer you want to get

    **Example Response**:

    .. sourcecode:: http
    
        HTTP/1.1 200
        Vary: Accept
        Content-Type: application/json
        X-Docker-Registry-Version: 0.6.0

        ""

    :statuscode 200: OK
    :statuscode 401: Requires authorization

.. http:get:: /v1/images/(image_id)/json

    get image for a given ``image_id``

    **Example Request**:

    .. sourcecode:: http

        GET /v1/images/088b4505aa3adc3d35e79c031fa126b403200f02f51920fbd9b7c503e87c7a2c/json HTTP/1.1
        Host: registry-1.docker.io
        Accept: application/json
        Content-Type: application/json
        Cookie: (Cookie provided by the Registry)

    :parameter image_id: the id for the layer you want to get

    **Example Response**:

    .. sourcecode:: http

        HTTP/1.1 200
        Vary: Accept
        Content-Type: application/json
        X-Docker-Registry-Version: 0.6.0
        X-Docker-Size: 456789
        X-Docker-Checksum: b486531f9a779a0c17e3ed29dae8f12c4f9e89cc6f0bc3c38722009fe6857087

        {
            id: "088b4505aa3adc3d35e79c031fa126b403200f02f51920fbd9b7c503e87c7a2c",
            parent: "aeee6396d62273d180a49c96c62e45438d87c7da4a5cf5d2be6bee4e21bc226f",
            created: "2013-04-30T17:46:10.843673+03:00",
            container: "8305672a76cc5e3d168f97221106ced35a76ec7ddbb03209b0f0d96bf74f6ef7",
            container_config: {
                Hostname: "host-test",
                User: "",
                Memory: 0,
                MemorySwap: 0,
                AttachStdin: false,
                AttachStdout: false,
                AttachStderr: false,
                PortSpecs: null,
                Tty: false,
                OpenStdin: false,
                StdinOnce: false,
                Env: null,
                Cmd: [
                "/bin/bash",
                "-c",
                "apt-get -q -yy -f install libevent-dev"
                ],
                Dns: null,
                Image: "imagename/blah",
                Volumes: { },
                VolumesFrom: ""
            },
            docker_version: "0.1.7"
        }

    :statuscode 200: OK
    :statuscode 401: Requires authorization
    :statuscode 404: Image not found


Ancestry
********

.. http:get:: /v1/images/(image_id)/ancestry

    get ancestry for an image given an ``image_id``

    **Example Request**:

    .. sourcecode:: http

        GET /v1/images/088b4505aa3adc3d35e79c031fa126b403200f02f51920fbd9b7c503e87c7a2c/ancestry HTTP/1.1
        Host: registry-1.docker.io
        Accept: application/json
        Content-Type: application/json
        Cookie: (Cookie provided by the Registry)

    :parameter image_id: the id for the layer you want to get

    **Example Response**:

    .. sourcecode:: http

        HTTP/1.1 200
        Vary: Accept
        Content-Type: application/json
        X-Docker-Registry-Version: 0.6.0

        ["088b4502f51920fbd9b7c503e87c7a2c05aa3adc3d35e79c031fa126b403200f",
         "aeee63968d87c7da4a5cf5d2be6bee4e21bc226fd62273d180a49c96c62e4543",
         "bfa4c5326bc764280b0863b46a4b20d940bc1897ef9c1dfec060604bdc383280",
         "6ab5893c6927c15a15665191f2c6cf751f5056d8b95ceee32e43c5e8a3648544"]

    :statuscode 200: OK
    :statuscode 401: Requires authorization
    :statuscode 404: Image not found


2.2 Tags
--------

.. http:get:: /v1/repositories/(namespace)/(repository)/tags

    get all of the tags for the given repo.

    **Example Request**:

    .. sourcecode:: http

        GET /v1/repositories/foo/bar/tags HTTP/1.1
        Host: registry-1.docker.io
        Accept: application/json
        Content-Type: application/json
        X-Docker-Registry-Version: 0.6.0
        Cookie: (Cookie provided by the Registry)

    :parameter namespace: namespace for the repo
    :parameter repository: name for the repo

    **Example Response**:

    .. sourcecode:: http

        HTTP/1.1 200
        Vary: Accept
        Content-Type: application/json
        X-Docker-Registry-Version: 0.6.0

        {
            "latest": "9e89cc6f0bc3c38722009fe6857087b486531f9a779a0c17e3ed29dae8f12c4f",
            "0.1.1":  "b486531f9a779a0c17e3ed29dae8f12c4f9e89cc6f0bc3c38722009fe6857087"
        }

    :statuscode 200: OK
    :statuscode 401: Requires authorization
    :statuscode 404: Repository not found


.. http:get:: /v1/repositories/(namespace)/(repository)/tags/(tag)

    get a tag for the given repo.

    **Example Request**:

    .. sourcecode:: http

        GET /v1/repositories/foo/bar/tags/latest HTTP/1.1
        Host: registry-1.docker.io
        Accept: application/json
        Content-Type: application/json
        X-Docker-Registry-Version: 0.6.0
        Cookie: (Cookie provided by the Registry)

    :parameter namespace: namespace for the repo
    :parameter repository: name for the repo
    :parameter tag: name of tag you want to get

    **Example Response**:

    .. sourcecode:: http

        HTTP/1.1 200
        Vary: Accept
        Content-Type: application/json
        X-Docker-Registry-Version: 0.6.0

        "9e89cc6f0bc3c38722009fe6857087b486531f9a779a0c17e3ed29dae8f12c4f"

    :statuscode 200: OK
    :statuscode 401: Requires authorization
    :statuscode 404: Tag not found

.. http:delete:: /v1/repositories/(namespace)/(repository)/tags/(tag)

    delete the tag for the repo

    **Example Request**:

    .. sourcecode:: http

        DELETE /v1/repositories/foo/bar/tags/latest HTTP/1.1
        Host: registry-1.docker.io
        Accept: application/json
        Content-Type: application/json
        Cookie: (Cookie provided by the Registry)

    :parameter namespace: namespace for the repo
    :parameter repository: name for the repo
    :parameter tag: name of tag you want to delete

    **Example Response**:

    .. sourcecode:: http

        HTTP/1.1 200
        Vary: Accept
        Content-Type: application/json
        X-Docker-Registry-Version: 0.6.0

        ""

    :statuscode 200: OK
    :statuscode 401: Requires authorization
    :statuscode 404: Tag not found


.. http:put:: /v1/repositories/(namespace)/(repository)/tags/(tag)

    put a tag for the given repo.

    **Example Request**:

    .. sourcecode:: http

        PUT /v1/repositories/foo/bar/tags/latest HTTP/1.1
        Host: registry-1.docker.io
        Accept: application/json
        Content-Type: application/json
        Cookie: (Cookie provided by the Registry)

        "9e89cc6f0bc3c38722009fe6857087b486531f9a779a0c17e3ed29dae8f12c4f"

    :parameter namespace: namespace for the repo
    :parameter repository: name for the repo
    :parameter tag: name of tag you want to add

    **Example Response**:

    .. sourcecode:: http

        HTTP/1.1 200
        Vary: Accept
        Content-Type: application/json
        X-Docker-Registry-Version: 0.6.0

        ""

    :statuscode 200: OK
    :statuscode 400: Invalid data
    :statuscode 401: Requires authorization
    :statuscode 404: Image not found

2.3 Repositories
----------------

.. http:delete:: /v1/repositories/(namespace)/(repository)/

    delete a repository

    **Example Request**:

    .. sourcecode:: http

        DELETE /v1/repositories/foo/bar/ HTTP/1.1
        Host: registry-1.docker.io
        Accept: application/json
        Content-Type: application/json
        Cookie: (Cookie provided by the Registry)

        ""

    :parameter namespace: namespace for the repo
    :parameter repository: name for the repo

    **Example Response**:

    .. sourcecode:: http

        HTTP/1.1 200
        Vary: Accept
        Content-Type: application/json
        X-Docker-Registry-Version: 0.6.0

        ""

    :statuscode 200: OK
    :statuscode 401: Requires authorization
    :statuscode 404: Repository not found

2.4 Status
----------

.. http:get:: /v1/_ping

    Check status of the registry. This endpoint is also used to determine if
    the registry supports SSL.

    **Example Request**:

    .. sourcecode:: http

        GET /v1/_ping HTTP/1.1
        Host: registry-1.docker.io
        Accept: application/json
        Content-Type: application/json

        ""

    **Example Response**:

    .. sourcecode:: http

        HTTP/1.1 200
        Vary: Accept
        Content-Type: application/json
        X-Docker-Registry-Version: 0.6.0

        ""

    :statuscode 200: OK


3 Authorization
===============
This is where we describe the authorization process, including the tokens and cookies. 

TODO: add more info.
