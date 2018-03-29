# Golang Function Invoker [![Build Status](https://travis-ci.org/projectriff/go-function-invoker.svg?branch=master)](https://travis-ci.org/projectriff/go-function-invoker)

## Purpose
The *go function invoker* provides a Docker base layer for a function built as a [Go plugin](https://golang.org/pkg/plugin/).
It accepts gRPC requests, invokes the command for each request in the input stream,
and sends the command's output to the stream of gRPC responses.

## Install as a riff invoker

```bash
riff invokers apply -f go-invoker.yaml
```

## Development

### Prerequisites

The following tools are required to build this project:

- `make`
- Docker
- [Glide](https://github.com/Masterminds/glide#install) for dependency management

If you intend to re-generate mocks for testing, install:

- [Mockery](https://github.com/vektra/mockery#installation)

If you would like to run tests using the `ginkgo` command, install:

- [Ginkgo](http://onsi.github.io/ginkgo/)

If you need to re-compile the protobuf protocol, install:

- Google's [protocol compiler](https://github.com/google/protobuf)

### Get the source

```bash
go get github.com/projectriff/go-function-invoker
cd $(go env GOPATH)/src/github.com/projectriff/go-function-invoker
```

### Building

To build locally (this will produce a binary named `go-function-invoker` on _your_ machine):

```bash
make build
```

To build the Docker base layer:

```bash
make dockerize
```

This assumes that your docker client is correctly configured to target the daemon where you want the image built.

To run tests:

```bash
make test
```

To attach a [delve capable](https://github.com/derekparker/delve/blob/master/Documentation/EditorIntegration.md) debugger (such as Goland)
to a `go-function-invoker` running _inside_ k8s:

```bash
make debug-dockerize
```

Then expose the `2345` port as a service, using `config/debug-service.yaml`:

```bash
kubectl apply -f config/debug-service.yaml
```

Finally, update the function you would like to debug so that it picks up the new base layer.
Then you can connect the debugger through port `30111`.

### Compiling the Protocol

The gRPC protocol for the go function invoker is defined in [function.proto](https://github.com/projectriff/riff/blob/master/function-proto/function.proto).

Clone https://github.com/projectriff/riff and set `$FN_PROTO_PATH` to point at the cloned directory. Then issue:

```bash
make gen-proto
```

## riff Commands

- [riff init go](#riff-init-go)
- [riff create go](#riff-create-go)

<!-- riff-init -->

### riff init go

Initialize a go function

#### Synopsis

Generate the function based on a shared '.so' library file specified as the filename
and exported symbol name specified as the handler.

For example, type:

    riff init go -i words -l go -n rot13 --handler=Encode

to generate the resource definitions using sensible defaults.


```
riff init go [flags]
```

#### Options

```
      --handler string           the name of the function handler (name of Exported go function) (default "{{ .TitleCase .FunctionName }}")
  -h, --help                     help for go
      --invoker-version string   the version of invoker to use when building containers (default "0.0.2-snapshot")
```

#### Options inherited from parent commands

```
  -a, --artifact string      path to the function artifact, source code or jar file
      --config string        config file (default is $HOME/.riff.yaml)
      --dry-run              print generated function artifacts content to stdout only
  -f, --filepath string      path or directory used for the function resources (defaults to the current directory)
      --force                overwrite existing functions artifacts
  -i, --input string         the name of the input topic (defaults to function name)
  -n, --name string          the name of the function (defaults to the name of the current directory)
  -o, --output string        the name of the output topic (optional)
  -u, --useraccount string   the Docker user account to be used for the image repository (default "current OS user")
  -v, --version string       the version of the function image (default "0.0.1")
```

#### SEE ALSO

* [riff init](https://github.com/projectriff/riff/blob/master/riff-cli/docs/riff_init.md)	 - Initialize a function


<!-- /riff-init -->

<!-- riff-create -->

### riff create go

Create a go function

#### Synopsis

Create the function based on a shared '.so' library file specified as the filename
and exported symbol name specified as the handler.

For example, type:

    riff create go -i words -l go -n rot13 --handler=Encode

to create the resource definitions, and apply the resources, using sensible defaults.


```
riff create go [flags]
```

#### Options

```
      --handler string           the name of the function handler (name of Exported go function) (default "{{ .TitleCase .FunctionName }}")
  -h, --help                     help for go
      --invoker-version string   the version of invoker to use when building containers (default "0.0.2-snapshot")
      --namespace string         the namespace used for the deployed resources (defaults to kubectl's default)
      --push                     push the image to Docker registry
```

#### Options inherited from parent commands

```
  -a, --artifact string      path to the function artifact, source code or jar file
      --config string        config file (default is $HOME/.riff.yaml)
      --dry-run              print generated function artifacts content to stdout only
  -f, --filepath string      path or directory used for the function resources (defaults to the current directory)
      --force                overwrite existing functions artifacts
  -i, --input string         the name of the input topic (defaults to function name)
  -n, --name string          the name of the function (defaults to the name of the current directory)
  -o, --output string        the name of the output topic (optional)
  -u, --useraccount string   the Docker user account to be used for the image repository (default "current OS user")
  -v, --version string       the version of the function image (default "0.0.1")
```

#### SEE ALSO

* [riff create](https://github.com/projectriff/riff/blob/master/riff-cli/docs/riff_create.md)	 - Create a function (equivalent to init, build, apply)


<!-- /riff-create -->
