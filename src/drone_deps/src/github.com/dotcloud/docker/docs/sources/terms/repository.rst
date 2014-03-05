:title: Repository
:description: Definition of an Repository
:keywords: containers, lxc, concepts, explanation, image, repository, container

.. _repository_def:

Repository
==========

A repository is a set of images either on your local Docker server, or
shared, by pushing it to a :ref:`Registry<registry_def>` server.

Images can be associated with a repository (or multiple) by giving them an image name 
using one of three different commands:

1. At build time (e.g. ``sudo docker build -t IMAGENAME``),
2. When committing a container (e.g. ``sudo docker commit CONTAINERID IMAGENAME``) or
3. When tagging an image id with an image name (e.g. ``sudo docker tag IMAGEID IMAGENAME``).

A `Fully Qualified Image Name` (FQIN) can be made up of 3 parts:

``[registry_hostname[:port]/][user_name/](repository_name[:version_tag])``

``version_tag`` defaults to ``latest``, ``username`` and ``registry_hostname`` default to an empty string.
When ``registry_hostname`` is an empty string, then ``docker push`` will push to ``index.docker.io:80``.

If you create a new repository which you want to share, you will need to set at least the 
``user_name``, as the 'default' blank ``user_name`` prefix is reserved for official Docker images.

For more information see :ref:`Working with Repositories<working_with_the_repository>`
