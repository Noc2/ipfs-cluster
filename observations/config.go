package observations

import (
	"encoding/json"
	"expvar"
	"net/http"
	"net/http/pprof"
	"os"
	"strconv"
	"time"

	prom "github.com/gxed/client_golang/prometheus"
	"github.com/gxed/opencensus-go/exporter/jaeger"
	"github.com/gxed/opencensus-go/exporter/prometheus"
	"github.com/gxed/opencensus-go/plugin/ochttp"
	"github.com/gxed/opencensus-go/stats/view"
	"github.com/gxed/opencensus-go/trace"
	"github.com/gxed/opencensus-go/zpages"
	"github.com/kelseyhightower/envconfig"
	ocgorpc "github.com/lanzafame/go-libp2p-ocgorpc"

	"github.com/ipfs/ipfs-cluster/config"
)

const configKey = "observations"
const envConfigKey = "cluster_observations"

// Default values for this Config.
const (
	DefaultEnableMetrics            = true
	DefaultPrometheusEndpoint       = ":8888"
	DefaultMetricsReportingInterval = 2 * time.Second

	DefaultEnableTracing       = false
	DefaultJaegerAgentEndpoint = "0.0.0.0:6831"
	// DefaultJaegerCollectorEndpoint = "http://0.0.0.0:14268/api/traces"
	DefaultJaegerCollectorEndpoint = "http://0.0.0.0:14268"
	DefaultTracingSamplingProb     = 0.3
	DefaultTracingServiceName      = "cluster-daemon"
)

// Config allows to initialize observation tooling
// (metrics and tracing) with customized parameters.
type Config struct {
	config.Saver

	EnableMetrics            bool
	PrometheusEndpoint       string
	MetricsReportingInterval time.Duration

	EnableTracing           bool
	JaegerAgentEndpoint     string
	JaegerCollectorEndpoint string
	TracingSamplingProb     float64
	TracingServiceName      string
}

type jsonConfig struct {
	EnableMetrics            string
	PrometheusEndpoint       string
	MetricsReportingInterval string

	EnableTracing           string
	JaegerAgentEndpoint     string
	JaegerCollectorEndpoint string
	TracingSamplingProb     float64
	TracingServiceName      string
}

// ConfigKey provides a human-friendly identifier for this type of Config.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default sets the fields of this Config to sensible values.
func (cfg *Config) Default() error {
	cfg.EnableMetrics = DefaultEnableMetrics
	cfg.PrometheusEndpoint = DefaultPrometheusEndpoint
	cfg.MetricsReportingInterval = DefaultMetricsReportingInterval

	cfg.EnableTracing = DefaultEnableTracing
	cfg.JaegerAgentEndpoint = DefaultJaegerAgentEndpoint
	cfg.JaegerCollectorEndpoint = DefaultJaegerCollectorEndpoint
	cfg.TracingSamplingProb = DefaultTracingSamplingProb
	cfg.TracingServiceName = DefaultTracingServiceName
	return nil
}

// Validate checks that the fields of this Config have working values,
// at least in appearance.
func (cfg *Config) Validate() error {
	//TODO(lanzafame)
	return nil
}

// LoadJSON sets the fields of this Config to the values defined by the JSON
// representation of it, as generated by ToJSON.
func (cfg *Config) LoadJSON(raw []byte) error {
	jcfg := &jsonConfig{}
	err := json.Unmarshal(raw, jcfg)
	if err != nil {
		logger.Error("Error unmarshaling observations config")
		return err
	}

	cfg.Default()

	// override json config with env var
	err = envconfig.Process(envConfigKey, jcfg)
	if err != nil {
		return err
	}

	err = cfg.loadMetricsOptions(jcfg)
	if err != nil {
		return err
	}

	err = cfg.loadTracingOptions(jcfg)
	if err != nil {
		return err
	}

	return cfg.Validate()
}

func (cfg *Config) loadMetricsOptions(jcfg *jsonConfig) error {
	var err error
	cfg.EnableMetrics, err = strconv.ParseBool(jcfg.EnableMetrics)
	if err != nil {
		return err
	}
	cfg.PrometheusEndpoint = jcfg.PrometheusEndpoint

	return config.ParseDurations(
		configKey,
		&config.DurationOpt{Duration: jcfg.MetricsReportingInterval, Dst: &cfg.MetricsReportingInterval, Name: "metrics_reporting_interval"},
	)
}

func (cfg *Config) loadTracingOptions(jcfg *jsonConfig) error {
	var err error
	cfg.EnableTracing, err = strconv.ParseBool(jcfg.EnableTracing)
	if err != nil {
		return err
	}
	cfg.JaegerAgentEndpoint = jcfg.JaegerAgentEndpoint
	cfg.JaegerCollectorEndpoint = jcfg.JaegerCollectorEndpoint
	cfg.TracingSamplingProb = jcfg.TracingSamplingProb
	cfg.TracingServiceName = jcfg.TracingServiceName

	return nil
}

// ToJSON generates a human-friendly JSON representation of this Config.
func (cfg *Config) ToJSON() ([]byte, error) {
	jcfg := &jsonConfig{
		EnableMetrics:            strconv.FormatBool(cfg.EnableMetrics),
		PrometheusEndpoint:       cfg.PrometheusEndpoint,
		MetricsReportingInterval: cfg.MetricsReportingInterval.String(),
		EnableTracing:            strconv.FormatBool(cfg.EnableTracing),
		JaegerAgentEndpoint:      cfg.JaegerAgentEndpoint,
		JaegerCollectorEndpoint:  cfg.JaegerCollectorEndpoint,
		TracingSamplingProb:      cfg.TracingSamplingProb,
		TracingServiceName:       cfg.TracingServiceName,
	}

	return config.DefaultJSONMarshal(jcfg)
}

// Setup configures and starts metrics and tracing tooling,
// if enabled.
func Setup(cfg *Config) {
	if cfg.EnableMetrics {
		logger.Error("metrics enabled...")
		setupMetrics(cfg)
	}

	if cfg.EnableTracing {
		logger.Error("tracing enabled...")
		SetupTracing(cfg)
	}
}

func setupMetrics(cfg *Config) {
	// setup Prometheus
	registry := prom.NewRegistry()
	goCollector := prom.NewGoCollector()
	procCollector := prom.NewProcessCollector(os.Getpid(), "")
	registry.MustRegister(goCollector, procCollector)
	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "cluster",
		Registry:  registry,
	})
	if err != nil {
		logger.Fatalf("Failed to create Prometheus exporter: %v", err)
	}

	// register prometheus with opencensus
	view.RegisterExporter(pe)
	view.SetReportingPeriod(cfg.MetricsReportingInterval)

	// register the metrics views of interest
	if err := view.Register(DefaultViews...); err != nil {
		logger.Fatalf("failed to register views: %v", err)
	}
	if err := view.Register(
		ochttp.ClientCompletedCount,
		ochttp.ClientRoundtripLatencyDistribution,
		ochttp.ClientReceivedBytesDistribution,
		ochttp.ClientSentBytesDistribution,
	); err != nil {
		logger.Fatalf("failed to register views: %v", err)
	}
	if err := view.Register(
		ochttp.ServerRequestCountView,
		ochttp.ServerRequestBytesView,
		ochttp.ServerResponseBytesView,
		ochttp.ServerLatencyView,
		ochttp.ServerRequestCountByMethod,
		ochttp.ServerResponseCountByStatusCode,
	); err != nil {
		logger.Fatalf("failed to register views: %v", err)
	}
	if err := view.Register(ocgorpc.DefaultServerViews...); err != nil {
		logger.Fatalf("failed to register views: %v", err)
	}

	go func() {
		mux := http.NewServeMux()
		zpages.Handle(mux, "/debug")
		mux.Handle("/metrics", pe)
		mux.Handle("/debug/vars", expvar.Handler())
		mux.HandleFunc("/debug/pprof", pprof.Index)
		mux.HandleFunc("/debug/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/profile", pprof.Profile)
		mux.HandleFunc("/debug/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/trace", pprof.Trace)
		mux.Handle("/debug/block", pprof.Handler("block"))
		mux.Handle("/debug/goroutine", pprof.Handler("goroutine"))
		mux.Handle("/debug/heap", pprof.Handler("heap"))
		mux.Handle("/debug/mutex", pprof.Handler("mutex"))
		mux.Handle("/debug/threadcreate", pprof.Handler("threadcreate"))
		if err := http.ListenAndServe(cfg.PrometheusEndpoint, mux); err != nil {
			logger.Fatalf("Failed to run Prometheus /metrics endpoint: %v", err)
		}
	}()
}

// SetupTracing configures a OpenCensus Tracing exporter for Jaeger.
func SetupTracing(cfg *Config) *jaeger.Exporter {
	// setup Jaeger
	je, err := jaeger.NewExporter(jaeger.Options{
		AgentEndpoint: cfg.JaegerAgentEndpoint,
		// CollectorEndpoint: cfg.JaegerCollectorEndpoint,
		// Endpoint:    cfg.JaegerCollectorEndpoint,
		ServiceName: cfg.TracingServiceName,
	})
	if err != nil {
		logger.Fatalf("Failed to create the Jaeger exporter: %v", err)
	}

	// register jaeger with opencensus
	trace.RegisterExporter(je)
	// configure tracing
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.ProbabilitySampler(cfg.TracingSamplingProb)})
	return je
}