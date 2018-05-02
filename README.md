# Golang Function Invoker [![Build Status](https://travis-ci.org/projectriff/go-function-invoker.svg?branch=master)](https://travis-ci.org/projectriff/go-function-invoker)

## Purpose
The *go function invoker* provides a Docker base layer for a function built as a [Go plugin](https://golang.org/pkg/plugin/).
It accepts gRPC requests, invokes the command for each request in the input stream,
and sends the command's output to the stream of gRPC responses.

## Install as a riff invoker

```bash
riff invokers apply -f go-invoker.yaml
```

## Writing go functions
The go function invoker supports both "streaming" and "direct" (traditional request/reply style functions).
Internally, the latter are converted to the streaming model, so let's start with streaming functions:

### Writing a streaming function
If the exposed function accepts a receiving channel as its sole parameter
and returns a receiving channel as its first return value, then the function
is considered to be "streaming":

```go
func Foo(input <-chan X) <-chan Y {
}
```

where `X` and `Y` can be anything that can be (un)marshalled via content negotiation.


Additionally, the function can return a second, receiving channel of type `error` to signal errors.
The function invocation will abort if any error is received (meaning only the first error is
ever going to be considered):

```go
func Foo(input <-chan X) (output <-chan Y, errs <-chan error) {
}
```
This is more or less the canonical form of functions described in [Pipelines and cancellation](https://blog.golang.org/pipelines).
Note that the "cancelation" part doesn't apply here (and hence functions don't get passed a closeable channel
to exit early) because there is only ever one function in riff. Canceallation
is signaled by closure of the _input_ channel.

The general contract of supported functions is the following:
* the function **must** have the signature(s) described above
* the function **must** return "immediately". Actual processing of data is
to be handled in a new goroutine
* the function is responsible for **creating** the result _output_ (as well as
the optional `error` channel)
* the function is responsible for **closing** the result and error channel(s)
* closure of the _input_ channel signals the end of input data.
* the goroutine should return after having written to the error channel

Given all of the above, the typical form of a streaming function is going to be this:
```go
func Foo(input <-chan X) (<-chan Y, <-chan error) {
  out := make(chan Y)
  errs := make(chan error)

  go func() {
    defer close(out)
    defer close(errs)
    for in := range input {
      ...
    }
  }()

  return out, errs
}
```

### Writing a "regular" function
If the exposed function doesn't accept/return channels, then it is considered
a "regular" request reply and will be wrapped inside an at-most-one streaming
function.

The typical form of a "regular" function is going to be
```go
func Foo(input X) Y {
}
```

where `X` and `Y` can be anything that can be (un)marshalled via content negotiation.

An optional second return value of type `error` is supported:
```go
func Foo(input X) (Y, error) {
}
```

In addition to those two common forms, the function can also elect to not
require input ("supplier" style), or to not return a value ("consumer" style).
Combined with the optional `error` second/last return value, this is eight possible
supported forms (not all of them make sense for real-world applications.)

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
      --handler string   the name of the function handler (name of Exported go function) (default "{{ .TitleCase .FunctionName }}")
  -h, --help             help for go
```

#### Options inherited from parent commands

```
  -a, --artifact string          path to the function artifact, source code or jar file
      --config string            config file (default is $HOME/.riff.yaml)
      --dry-run                  print generated function artifacts content to stdout only
  -f, --filepath string          path or directory used for the function resources (defaults to the current directory)
      --force                    overwrite existing functions artifacts
  -i, --input string             the name of the input topic (defaults to function name)
      --invoker-version string   the version of the invoker to use when building containers
  -n, --name string              the name of the function (defaults to the name of the current directory)
  -o, --output string            the name of the output topic (optional)
  -u, --useraccount string       the Docker user account to be used for the image repository (default "current OS user")
  -v, --version string           the version of the function image (default "0.0.1")
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
      --handler string     the name of the function handler (name of Exported go function) (default "{{ .TitleCase .FunctionName }}")
  -h, --help               help for go
      --namespace string   the namespace used for the deployed resources (defaults to kubectl's default)
      --push               push the image to Docker registry
```

#### Options inherited from parent commands

```
  -a, --artifact string          path to the function artifact, source code or jar file
      --config string            config file (default is $HOME/.riff.yaml)
      --dry-run                  print generated function artifacts content to stdout only
  -f, --filepath string          path or directory used for the function resources (defaults to the current directory)
      --force                    overwrite existing functions artifacts
  -i, --input string             the name of the input topic (defaults to function name)
      --invoker-version string   the version of the invoker to use when building containers
  -n, --name string              the name of the function (defaults to the name of the current directory)
  -o, --output string            the name of the output topic (optional)
  -u, --useraccount string       the Docker user account to be used for the image repository (default "current OS user")
  -v, --version string           the version of the function image (default "0.0.1")
```

#### SEE ALSO

* [riff create](https://github.com/projectriff/riff/blob/master/riff-cli/docs/riff_create.md)	 - Create a function (equivalent to init, build, apply)


<!-- /riff-create -->
