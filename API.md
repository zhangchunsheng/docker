# Docker API

Status: DRAFT
Maintainer: Solomon Hykes <solomon@dotcloud.com>
Version 1.0alpha1


## Overview


## Container format

	* It''s a root filesystem...
	* ... With metadata in /.docker


## Execution environment

### Environment variables

### Command-line arguments

### Terminal allocation and interactive mode

### Logging

### Signals

### Network configuration

### Background processes

### Process privileges

### Persisting data

### Exposing a network service

### Consuming a network service

### Introspection

### Custom metadata

### Prompting for operator input


## Extending Docker with plugins


## Use cases

### Run docker on older kernels

### Run docker on kernels without aufs support

### Build and deploy new containers with "git push"

### Build and deploy a multi-component stack as a single unit (ie. wordpress + mysql + memcache)

### Build a very small container from a very large container (ie. 20MB postgres from 2GB buildroot)

### Run containers on a Mac or Windows machine using nothing other than 'docker run'

### Transfer containers directly between hosts

### Transfer updates using git packfiles

### Run docker inside docker

	* Run docker unit tests

### Write docker plugins in any language

	* Advanced transport and storage of images with bup (written in python)
	* Quick prototyping of plugins with shell scripts (requested by @jpetazzo and @destructuring)

### Dynamic service discovery across multiple hosts

### Smart caching

	* "I just installed version X of this deb package. My caching key is apt-foo-X"
	* "in this case I want to always install the latest version of this package. my caching key is NULL"

### Expose metadata to the outside world

	* Issue gh#1276: "after the username/password/dbname are created, I'd like to stick that data somewhere that the host can find it with a script"


### Better discovery of a container's ports

	* "If you don't want me to hardcode public ports in my Dockerfile, make it easier to find the ports of my containers!"

### Cross-platform containers


### Naming containers

See github issue #1.

### Grouping and tagging containers for operations

For example, use tags to automatically configure a load-balancer.
