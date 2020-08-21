package main

import (
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/tencentyun/tencentcloud-exporter/pkg/collector"
	"github.com/tencentyun/tencentcloud-exporter/pkg/config"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"net/http"
	"os"
)

func newHandler(c *config.TencentConfig, includeExporterMetrics bool, maxRequests int, logger log.Logger) (*http.Handler, error) {
	exporterMetricsRegistry := prometheus.NewRegistry()
	if includeExporterMetrics {
		exporterMetricsRegistry.MustRegister(
			prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
			prometheus.NewGoCollector(),
		)
	}

	nc, err := collector.NewTcMonitorCollector(c, logger)
	if err != nil {
		return nil, fmt.Errorf("couldn't create collector: %s", err)
	}
	r := prometheus.NewRegistry()
	r.MustRegister(version.NewCollector("qcloud_exporter"))
	if err := r.Register(nc); err != nil {
		return nil, fmt.Errorf("couldn't register tencent cloud monitor collector: %s", err)
	}

	handler := promhttp.HandlerFor(
		prometheus.Gatherers{exporterMetricsRegistry, r},
		promhttp.HandlerOpts{
			ErrorHandling:       promhttp.ContinueOnError,
			MaxRequestsInFlight: maxRequests,
			Registry:            exporterMetricsRegistry,
		},
	)
	if includeExporterMetrics {
		handler = promhttp.InstrumentMetricHandler(
			exporterMetricsRegistry, handler,
		)
	}
	return &handler, nil

}

func main() {
	var (
		listenAddress = kingpin.Flag(
			"web.listen-address",
			"Address on which to expose metrics and web interface.",
		).Default(":9123").String()
		metricsPath = kingpin.Flag(
			"web.telemetry-path",
			"Path under which to expose metrics.",
		).Default("/metrics").String()
		enableExporterMetrics = kingpin.Flag(
			"web.enable-exporter-metrics",
			"Include metrics about the exporter itself (promhttp_*, process_*, go_*).",
		).Default("false").Bool()
		maxRequests = kingpin.Flag(
			"web.max-requests",
			"Maximum number of parallel scrape requests. Use 0 to disable.",
		).Default("0").Int()
		configFile = kingpin.Flag(
			"config.file", "Tencent qcloud exporter configuration file.",
		).Default("qcloud.yml").String()
	)

	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print("qcloud_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", "Starting qcloud_exporter", "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())

	tencentConfig := config.NewConfig()
	if err := tencentConfig.LoadFile(*configFile); err != nil {
		level.Error(logger).Log("msg", "Load config error", "err", err)
		os.Exit(1)
	} else {
		level.Info(logger).Log("msg", "Load config ok")
	}

	handler, err := newHandler(tencentConfig, *enableExporterMetrics, *maxRequests, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Create handler fail", "err", err)
		os.Exit(1)
	}

	http.Handle(*metricsPath, *handler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>QCloud Exporter</title></head>
			<body>
			<h1>QCloud Exporter</h1>
			<p><a href="` + *metricsPath + `">Metrics</a></p>
			</body>
			</html>`))
	})

	level.Info(logger).Log("msg", "Listening on", "address", *listenAddress)
	err = http.ListenAndServe(*listenAddress, nil)
	if err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
}
