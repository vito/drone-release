:title: Python Web app example
:description: Building your own python web app using docker
:keywords: docker, example, python, web app

.. _python_web_app:

Python Web App
==============

.. include:: example_header.inc

The goal of this example is to show you how you can author your own
Docker images using a parent image, making changes to it, and then
saving the results as a new image. We will do that by making a simple
hello Flask web application image.

**Steps:**

.. code-block:: bash

    sudo docker pull shykes/pybuilder

We are downloading the ``shykes/pybuilder`` Docker image

.. code-block:: bash

    URL=http://github.com/shykes/helloflask/archive/master.tar.gz

We set a ``URL`` variable that points to a tarball of a simple helloflask web app

.. code-block:: bash

    BUILD_JOB=$(sudo docker run -d -t shykes/pybuilder:latest /usr/local/bin/buildapp $URL)

Inside of the ``shykes/pybuilder`` image there is a command called
``buildapp``, we are running that command and passing the ``$URL`` variable
from step 2 to it, and running the whole thing inside of a new
container. The ``BUILD_JOB`` environment variable will be set with the new container ID.

.. code-block:: bash

    sudo docker attach -sig-proxy=false $BUILD_JOB
    [...]

While this container is running, we can attach to the new container to
see what is going on. The flag ``--sig-proxy`` set as ``false`` allows you to connect and
disconnect (Ctrl-C) to it without stopping the container.

.. code-block:: bash

    sudo docker ps -a

List all Docker containers. If this container has already finished
running, it will still be listed here.

.. code-block:: bash

    BUILD_IMG=$(sudo docker commit $BUILD_JOB _/builds/github.com/shykes/helloflask/master)

Save the changes we just made in the container to a new image called
``_/builds/github.com/hykes/helloflask/master`` and save the image ID in
the ``BUILD_IMG`` variable name.

.. code-block:: bash

    WEB_WORKER=$(sudo docker run -d -p 5000 $BUILD_IMG /usr/local/bin/runapp)

- **"docker run -d "** run a command in a new container. We pass "-d"
  so it runs as a daemon.
- **"-p 5000"** the web app is going to listen on this port, so it
  must be mapped from the container to the host system.
- **"$BUILD_IMG"** is the image we want to run the command inside of.
- **/usr/local/bin/runapp** is the command which starts the web app.

Use the new image we just created and create a new container with
network port 5000, and return the container ID and store in the
``WEB_WORKER`` variable.

.. code-block:: bash

    sudo docker logs $WEB_WORKER
     * Running on http://0.0.0.0:5000/

View the logs for the new container using the ``WEB_WORKER`` variable, and
if everything worked as planned you should see the line ``Running on
http://0.0.0.0:5000/`` in the log output.

.. code-block:: bash

    WEB_PORT=$(sudo docker port $WEB_WORKER 5000 | awk -F: '{ print $2 }')

Look up the public-facing port which is NAT-ed. Find the private port
used by the container and store it inside of the ``WEB_PORT`` variable.

.. code-block:: bash

    # install curl if necessary, then ...
    curl http://127.0.0.1:$WEB_PORT
      Hello world!

Access the web app using the ``curl`` binary. If everything worked as planned you
should see the line ``Hello world!`` inside of your console.

**Video:**

See the example in action

.. raw:: html

   <iframe width="720" height="400" frameborder="0"
           sandbox="allow-same-origin allow-scripts" 
   srcdoc="<body><script type=&quot;text/javascript&quot; 
           src=&quot;https://asciinema.org/a/2573.js&quot; 
           id=&quot;asciicast-2573&quot; async></script></body>">
   </iframe>

Continue to :ref:`running_ssh_service`.
