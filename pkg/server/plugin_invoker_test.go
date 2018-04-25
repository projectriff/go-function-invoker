package server

import (
	"runtime"

	"os"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/projectriff/go-function-invoker/pkg/function"
	"golang.org/x/net/context"

	"fmt"
	"time"

	"github.com/onsi/gomega/types"
	"google.golang.org/grpc"
	"math/rand"
	"net"
	"io"
	"net/http"
)

const (
	builtPlugin = "../../fixtures/function_plugin.so"
)

var _ = BeforeSuite(func() {
	// Go plugins are only supported on *nix and Mac
	Expect(runtime.GOOS).NotTo(Equal("windows"))

	command := exec.Command("go", "build", "-buildmode=plugin", "-o", builtPlugin, sourceOf(builtPlugin))
	command.Stderr = os.Stderr
	command.Stdout = os.Stdout
	err := command.Run()
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	os.Remove(builtPlugin)
})

type TestCase struct {
	ContentType   string
	Accept        string
	In            []byte
	Expected      types.GomegaMatcher
	ExpectedError errorCode
}

var _ = Describe("PluginInvoker", func() {

	var (
		invoker    *pluginInvoker
		handler    string
		gRpcServer *grpc.Server
		sidecar    function.MessageFunction_CallClient
		cancel     context.CancelFunc
	)

	JustBeforeEach(func() {
		var err error

		invoker, err = NewInvoker(fmt.Sprintf("%s?%s=%s", builtPlugin, Handler, handler))
		Expect(err).NotTo(HaveOccurred())

		port := 1024 + rand.Intn(65536-1024)

		gRpcServer = grpc.NewServer()
		listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
		function.RegisterMessageFunctionServer(gRpcServer, invoker)
		go func() {
			gRpcServer.Serve(listener)
		}()

		ctx, _ := context.WithTimeout(context.Background(), 60*time.Second)
		conn, err := grpc.DialContext(ctx, fmt.Sprintf("localhost:%v", port), grpc.WithInsecure(), grpc.WithBlock())
		Expect(err).NotTo(HaveOccurred())

		ctx, cancel = context.WithCancel(context.Background())

		//sidecar, err = function.NewMessageFunctionClient(conn).Call(context.Background())
		sidecar, err = function.NewMessageFunctionClient(conn).Call(ctx)
		Expect(err).NotTo(HaveOccurred())

	})

	AfterEach(func() {
		gRpcServer.Stop()
	})

	Context("with 'direct' functions", func() {
		BeforeEach(func() {
			handler = "StringInStringOut"
		})

		It("should assume input content type is text/plain, accepted output is text/plain", func() {
			go func() {
				defer GinkgoRecover()
				err := sidecar.Send(msg("world"))
				Expect(err).NotTo(HaveOccurred())
				err = sidecar.CloseSend()
				Expect(err).NotTo(HaveOccurred())
			}()

			result, err := sidecar.Recv()
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Payload).To(Equal([]byte("Hello world")))
		})

		It("should know how to return text/plain", func() {
			go func() {
				defer GinkgoRecover()
				err := sidecar.Send(msg("world", "Content-Type", "text/plain", "Accept", "text/plain"))
				Expect(err).NotTo(HaveOccurred())
				err = sidecar.CloseSend()
				Expect(err).NotTo(HaveOccurred())
			}()
			result, err := sidecar.Recv()
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Payload).To(Equal([]byte("Hello world")))
		})

		It("should know how to return application/json", func() {
			go func() {
				defer GinkgoRecover()

				err := sidecar.Send(msg("world", "Content-Type", "text/plain", "Accept", "application/json"))
				Expect(err).NotTo(HaveOccurred())
				err = sidecar.CloseSend()
				Expect(err).NotTo(HaveOccurred())
			}()

			result, err := sidecar.Recv()
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Payload).To(Equal([]byte("\"Hello world\"\n")))
		})

		It("should know how to handle incoming application/json", func() {
			go func() {
				defer GinkgoRecover()
				err := sidecar.Send(msg("\"world\"", "Content-Type", "application/json", "Accept", "text/plain"))
				Expect(err).NotTo(HaveOccurred())
				err = sidecar.CloseSend()
				Expect(err).NotTo(HaveOccurred())
			}()

			result, err := sidecar.Recv()
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Payload).To(Equal([]byte("Hello world")))
		})

		It("should cope with Close() in any order", func() {
			err := sidecar.Send(msg("world", "Content-Type", "text/plain", "Accept", "text/plain"))
			Expect(err).NotTo(HaveOccurred())

			result, err := sidecar.Recv()
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Payload).To(Equal([]byte("Hello world")))

			err = sidecar.CloseSend()
			Expect(err).NotTo(HaveOccurred())
			result, err = sidecar.Recv()
			Expect(err).To(MatchError(io.EOF))
		})

		It("idiomatic go errors are propagated back", func() {
			go func() {
				defer GinkgoRecover()
				err := sidecar.Send(msg("Riff", "Content-Type", "text/plain", "Accept", "text/plain"))
				Expect(err).NotTo(HaveOccurred())
				err = sidecar.CloseSend()
				Expect(err).NotTo(HaveOccurred())
			}()

			_, err := sidecar.Recv()
			Expect(err).To(MatchError(ContainSubstring("error condition")))

		})

		It("unmarshalling errors abort and are propagated", func() {
			go func() {
				defer GinkgoRecover()
				err := sidecar.Send(msg("world", "Content-Type", "text/foobar", "Accept", "text/plain"))
				Expect(err).NotTo(HaveOccurred())
			}()

			_, err := sidecar.Recv()
			Expect(err).To(MatchError(ContainSubstring("Unsupported Content-Type: text/foobar")))
		})

		It("marshalling errors abort and are propagated", func() {
			go func() {
				defer GinkgoRecover()
				err := sidecar.Send(msg("world", "Content-Type", "text/plain", "Accept", "text/foobar"))
				Expect(err).NotTo(HaveOccurred())
			}()

			_, err := sidecar.Recv()
			Expect(err).To(MatchError(ContainSubstring("unsupported content types: [text/foobar]")))
		})
	})

	Context("with 'streaming' functions that accept a string", func() {
		BeforeEach(func() {
			handler = "RunLengthEncode"
		})

		It("should cope with independent input/output streams", func() {
			go func() {
				defer GinkgoRecover()
				err := sidecar.Send(msg("world", "Content-Type", "text/plain", "Accept", "application/json"))
				Expect(err).NotTo(HaveOccurred())
				err = sidecar.Send(msg("world", "Content-Type", "text/plain", "Accept", "application/json"))
				Expect(err).NotTo(HaveOccurred())
				err = sidecar.Send(msg("hello", "Content-Type", "text/plain", "Accept", "application/json"))
				Expect(err).NotTo(HaveOccurred())
				err = sidecar.Send(msg("world", "Content-Type", "text/plain", "Accept", "application/json"))
				Expect(err).NotTo(HaveOccurred())

				err = sidecar.CloseSend()
				Expect(err).NotTo(HaveOccurred())
			}()

			result, err := sidecar.Recv()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Payload).To(Equal([]byte(`{"Word":"world","Count":2}` + "\n")))

			result, err = sidecar.Recv()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Payload).To(Equal([]byte(`{"Word":"hello","Count":1}` + "\n")))

			result, err = sidecar.Recv()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Payload).To(Equal([]byte(`{"Word":"world","Count":1}` + "\n")))

			result, err = sidecar.Recv()
			Expect(err).To(MatchError(io.EOF))
		})

		It("should propagate user function errors back", func() {
			go func() {
				defer GinkgoRecover()
				err := sidecar.Send(msg("hello", "Content-Type", "text/plain", "Accept", "application/json"))
				Expect(err).NotTo(HaveOccurred())
				err = sidecar.Send(msg("world", "Content-Type", "text/plain", "Accept", "application/json"))
				Expect(err).NotTo(HaveOccurred())
				err = sidecar.Send(msg("world", "Content-Type", "text/plain", "Accept", "application/json"))
				Expect(err).NotTo(HaveOccurred())
				err = sidecar.Send(msg("world", "Content-Type", "text/plain", "Accept", "application/json"))
				Expect(err).NotTo(HaveOccurred())

				err = sidecar.CloseSend()
				Expect(err).NotTo(HaveOccurred())
			}()

			result, err := sidecar.Recv()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Payload).To(Equal([]byte(`{"Word":"hello","Count":1}` + "\n")))

			result, err = sidecar.Recv()
			Expect(err).To(MatchError(ContainSubstring("Too many occurrences of world")))
		})

	})
	Context("with Supplier-style functions", func() {
		BeforeEach(func() {
			handler = "SupplierFunc"
		})

		It("should support supplier-style functions", func() {
			result, err := sidecar.Recv()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Payload).To(Equal([]byte("0")))

			result, err = sidecar.Recv()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Payload).To(Equal([]byte("1")))

			go func() {
				time.Sleep(100 * time.Millisecond)
				err = sidecar.CloseSend()

			}()

			Eventually(func() error {
				_, err := sidecar.Recv()
				return err
			}, 1*time.Second, 10*time.Millisecond).Should(MatchError(io.EOF))

		})

	})

	Context("with 'direct' style functions", func() {
		Context("with f(X) (Y, error)", func() {
			BeforeEach(func() {
				handler = "Direct1"
			})
			It("should support successful invocation", func() {
				go func() {
					defer GinkgoRecover()
					err := sidecar.Send(msg("21", "Content-Type", "text/plain", "Accept", "text/plain"))
					Expect(err).NotTo(HaveOccurred())
				}()

				result, err := sidecar.Recv()
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Payload).To(Equal([]byte("42")))

			})
			It("should support error invocation", func() {
				go func() {
					defer GinkgoRecover()
					err := sidecar.Send(msg("foo", "Content-Type", "text/plain", "Accept", "text/plain"))
					Expect(err).NotTo(HaveOccurred())
				}()

				_, err := sidecar.Recv()
				Expect(err).To(MatchError(ContainSubstring(`strconv.Atoi: parsing "foo": invalid syntax`)))

			})
		})
		Context("with f(X) Y", func() {
			BeforeEach(func() {
				handler = "Direct2"
			})
			It("should support successful invocation", func() {
				go func() {
					defer GinkgoRecover()
					err := sidecar.Send(msg("21", "Content-Type", "text/plain", "Accept", "text/plain"))
					Expect(err).NotTo(HaveOccurred())
				}()

				result, err := sidecar.Recv()
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Payload).To(Equal([]byte("42")))
			})
		})

		Context("with f(X)", func() {
			BeforeEach(func() {
				handler = "Direct3"
			})
			It("should support successful invocation", func() {

				port := 1024 + rand.Intn(65536-1024)
				c := make(chan struct{})
				server := http.Server{
					Addr:    fmt.Sprintf(":%v", port),
					Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(c) }),
				}
				go func() {
					server.ListenAndServe()
				}()

				err := sidecar.Send(msg(fmt.Sprintf("http://localhost:%v", port), "Content-Type", "text/plain"))
				Expect(err).NotTo(HaveOccurred())
				err = sidecar.CloseSend()
				Expect(err).NotTo(HaveOccurred())
				<-c
				server.Close()
			})
		})

		Context("with f(X) error", func() {
			BeforeEach(func() {
				handler = "Direct4"
			})
			It("should support successful invocation", func() {
				go func() {
					defer GinkgoRecover()
					err := sidecar.Send(msg("riff", "Content-Type", "text/plain", "Accept", "text/plain"))
					Expect(err).NotTo(HaveOccurred())

					err = sidecar.CloseSend()
					Expect(err).NotTo(HaveOccurred())
				}()

				_, err := sidecar.Recv()
				Expect(err).To(MatchError(io.EOF))
			})
			It("should support error invocations", func() {
				go func() {
					defer GinkgoRecover()
					err := sidecar.Send(msg("Hello Riff", "Content-Type", "text/plain", "Accept", "text/plain"))
					Expect(err).NotTo(HaveOccurred())

					err = sidecar.CloseSend()
					Expect(err).NotTo(HaveOccurred())
				}()

				_, err := sidecar.Recv()
				Expect(err).To(MatchError(ContainSubstring("Hello Riff contained 'Riff'")))
			})
		})

		Context("with f() (Y, error)", func() {
			BeforeEach(func() {
				handler = "Direct5"
			})
			It("should support successful invocation", func() {
				go func() {
					defer GinkgoRecover()
					err := sidecar.CloseSend()
					Expect(err).NotTo(HaveOccurred())
				}()

				result, err := sidecar.Recv()
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Payload).To(Equal([]byte("5")))
			})
		})
		Context("with f() (Y, error)", func() {
			BeforeEach(func() {
				handler = "Direct5e"
			})
			It("should support error invocations", func() {
				go func() {
					defer GinkgoRecover()
					err := sidecar.CloseSend()
					Expect(err).NotTo(HaveOccurred())
				}()

				_, err := sidecar.Recv()
				Expect(err).To(MatchError(ContainSubstring("Direct5e error")))
			})
		})
		Context("with f() Y", func() {
			BeforeEach(func() {
				handler = "Direct6"
			})
			It("should support successful invocations", func() {
				go func() {
					defer GinkgoRecover()
					err := sidecar.CloseSend()
					Expect(err).NotTo(HaveOccurred())
				}()

				result, err := sidecar.Recv()
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Payload).To(Equal([]byte("42")))
			})
		})
		Context("with f() error", func() {
			BeforeEach(func() {
				handler = "Direct8"
			})
			It("should support successful invocations", func() {
				go func() {
					defer GinkgoRecover()
					err := sidecar.CloseSend()
					Expect(err).NotTo(HaveOccurred())
				}()

				_, err := sidecar.Recv()
				Expect(err).To(MatchError(io.EOF))
			})
		})
		Context("with f() error", func() {
			BeforeEach(func() {
				handler = "Direct8e"
			})
			It("should support error invocations", func() {
				go func() {
					defer GinkgoRecover()
					err := sidecar.CloseSend()
					Expect(err).NotTo(HaveOccurred())
				}()

				_, err := sidecar.Recv()
				Expect(err).To(MatchError(ContainSubstring("Direct8e error")))
			})
		})
	})
})

func msg(payload string, headers ... string) *function.Message {
	m := make(map[string]*function.Message_HeaderValue, len(headers)/2)
	for i := 0; i < len(headers); i += 2 {
		m[headers[i]] = &function.Message_HeaderValue{Values: []string{headers[i+1]}}
	}
	return &function.Message{Payload: []byte(payload), Headers: m}

}

func sourceOf(lib string) string {
	result := []rune(lib)
	result[len(lib)-len("so")] = 'g'
	return string(result)
}
