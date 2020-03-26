package tracer

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"sync"

	"github.com/fatih/color"
	"github.com/inconshreveable/log15"
	"github.com/sourcegraph/sourcegraph/internal/conf"
	"github.com/sourcegraph/sourcegraph/internal/env"
	"github.com/sourcegraph/sourcegraph/internal/trace"
	"go.uber.org/automaxprocs/maxprocs"

	opentracing "github.com/opentracing/opentracing-go"
	jaeger "github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
	jaegermetrics "github.com/uber/jaeger-lib/metrics"
)

var (
	lightstepIncludeSensitive, _ = strconv.ParseBool(env.Get("LIGHTSTEP_INCLUDE_SENSITIVE", "", "send span logs to LightStep"))
	logColors                    = map[log15.Lvl]color.Attribute{
		log15.LvlCrit:  color.FgRed,
		log15.LvlError: color.FgRed,
		log15.LvlWarn:  color.FgYellow,
		log15.LvlInfo:  color.FgCyan,
		log15.LvlDebug: color.Faint,
	}
	// We'd prefer these in caps, not lowercase, and don't need the 4-character alignment
	logLabels = map[log15.Lvl]string{
		log15.LvlCrit:  "CRITICAL",
		log15.LvlError: "ERROR",
		log15.LvlWarn:  "WARN",
		log15.LvlInfo:  "INFO",
		log15.LvlDebug: "DEBUG",
	}
)

func init() {
	// Tune GOMAXPROCS for kubernetes. All our binaries import this package,
	// so we tune for all of them.
	//
	// TODO it is surprising that we do this here. We should create a standard
	// import for sourcegraph binaries which would have less surprising
	// behaviour.
	if _, err := maxprocs.Set(); err != nil {
		log15.Error("automaxprocs failed", "error", err)
	}
}

func condensedFormat(r *log15.Record) []byte {
	colorAttr := logColors[r.Lvl]
	text := logLabels[r.Lvl]
	var msg bytes.Buffer
	if colorAttr != 0 {
		fmt.Print(color.New(colorAttr).Sprint(text) + " " + r.Msg)
	} else {
		fmt.Print(&msg, r.Msg)
	}
	if len(r.Ctx) > 0 {
		for i := 0; i < len(r.Ctx); i += 2 {
			// not as smart about printing things as log15's internal magic
			fmt.Fprintf(&msg, ", %s: %v", r.Ctx[i].(string), r.Ctx[i+1])
		}
	}
	msg.WriteByte('\n')
	return msg.Bytes()
}

// Options control the behavior of a tracer.
type Options struct {
	filters     []func(*log15.Record) bool
	serviceName string
}

// If this idiom seems strange:
// https://github.com/tmrts/go-patterns/blob/master/idiom/functional-options.md
type Option func(*Options)

func ServiceName(s string) Option {
	return func(o *Options) {
		o.serviceName = s
	}
}

func Filter(f func(*log15.Record) bool) Option {
	return func(o *Options) {
		o.filters = append(o.filters, f)
	}
}

func init() {
	// Enable colors by default but support https://no-color.org/
	color.NoColor = env.Get("NO_COLOR", "", "Disable colored output") != ""
}

func Init(options ...Option) {
	opts := &Options{}
	for _, setter := range options {
		setter(opts)
	}
	if opts.serviceName == "" {
		opts.serviceName = env.MyName
	}
	var handler log15.Handler
	switch env.LogFormat {
	case "condensed":
		handler = log15.StreamHandler(os.Stderr, log15.FormatFunc(condensedFormat))
	case "logfmt":
		fallthrough
	default:
		handler = log15.StreamHandler(os.Stderr, log15.LogfmtFormat())
	}
	for _, filter := range opts.filters {
		handler = log15.FilterHandler(filter, handler)
	}
	// Filter log output by level.
	lvl, err := log15.LvlFromString(env.LogLevel)
	if err == nil {
		handler = log15.LvlFilterHandler(lvl, handler)
	}
	log15.Root().SetHandler(log15.LvlFilterHandler(lvl, handler))

	initTracer(opts)
}

// initTracer is a helper that should be called exactly once (from Init).
func initTracer(opts *Options) {
	// State
	var jaegerEnabled bool
	var jaegerCloser io.Closer
	var jaegerEnabledMu sync.Mutex

	// Watch loop
	conf.Watch(func() {
		jaegerEnabledMu.Lock()
		defer jaegerEnabledMu.Unlock()

		if useJaeger := conf.Get().UseJaeger; useJaeger && !jaegerEnabled {
			log15.Info("Distributed tracing enabled", "tracer", "jaeger")
			cfg, err := jaegercfg.FromEnv()
			cfg.ServiceName = opts.serviceName
			if err != nil {
				log15.Warn("Could not initialize jaeger tracer from env", "error", err.Error())
				return
			}
			if reflect.DeepEqual(cfg.Sampler, &jaegercfg.SamplerConfig{}) {
				// Default sampler configuration for when it is not specified via
				// JAEGER_SAMPLER_* env vars. In most cases, this is sufficient
				// enough to connect Sourcegraph to Jaeger without any env vars.
				cfg.Sampler.Type = jaeger.SamplerTypeConst
				cfg.Sampler.Param = 1
			}
			tracer, closer, err := cfg.NewTracer(
				jaegercfg.Logger(jaegerlog.StdLogger),
				jaegercfg.Metrics(jaegermetrics.NullFactory),
			)
			if err != nil {
				log15.Warn("Could not initialize jaeger tracer", "error", err.Error())
				return
			}
			opentracing.SetGlobalTracer(tracer)
			jaegerCloser = closer
			trace.SpanURL = jaegerSpanURL
			jaegerEnabled = true
		} else if !useJaeger && jaegerEnabled {
			log15.Info("Distributed tracing disabled")
			if existingJaegerCloser := jaegerCloser; existingJaegerCloser != nil {
				go func() { // do outside critical region
					err := existingJaegerCloser.Close()
					if err != nil {
						log15.Warn("Unable to close Jaeger client", "error", err)
					}
				}()
			}
			opentracing.SetGlobalTracer(opentracing.NoopTracer{})
			jaegerCloser = nil
			trace.SpanURL = trace.NoopSpanURL
			jaegerEnabled = false
		}
	})

}

func jaegerSpanURL(span opentracing.Span) string {
	spanCtx := span.Context().(jaeger.SpanContext)
	return spanCtx.TraceID().String()
}
