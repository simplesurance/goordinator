package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/pflag"
	zaplogfmt "github.com/sykesm/zap-logfmt"
	"github.com/thecodeteam/goodbye"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/simplesurance/goordinator/internal/cfg"
	"github.com/simplesurance/goordinator/internal/githubclt"
	"github.com/simplesurance/goordinator/internal/goordinator"
	"github.com/simplesurance/goordinator/internal/logfields"
	"github.com/simplesurance/goordinator/internal/provider/github"
)

const appName = "goordinator"

var logger *zap.Logger

// Version is set via a ldflag on compilation
var Version = "unknown"

const EventChannelBufferSize = 1024

func exitOnErr(msg string, err error) {
	if err == nil {
		return
	}

	fmt.Fprintln(os.Stderr, "ERROR:", msg+", error:", err.Error())
	os.Exit(1)
}

func panicHandler() {
	if r := recover(); r != nil {
		logger.Info(
			"panic caught , terminating gracefully",
			zap.String("panic", fmt.Sprintf("%v", r)),
			zap.StackSkip("stacktrace", 1),
		)

		ctx, cancelFn := context.WithTimeout(context.Background(), time.Minute)
		defer cancelFn()

		goodbye.Exit(ctx, 1)

	}
}

func startHTTPSServer(listenAddr string, certFile, keyFile string, mux *http.ServeMux) {
	httpsServer := http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	goodbye.Register(func(context.Context, os.Signal) {
		const shutdownTimeout = 30 * time.Second
		ctx, cancelFn := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancelFn()

		logger.Debug(
			"terminating https server",
			logfields.Event("https_server_terminating"),
			zap.Duration("shutdown_timeout", shutdownTimeout),
		)

		err := httpsServer.Shutdown(ctx)
		if err != nil {
			logger.Warn(
				"shutting down https server failed",
				logfields.Event("https_server_termination_failed"),
				zap.Error(err),
			)
		}
	})

	go func() {
		defer panicHandler()

		logger.Info(
			"https server started",
			logfields.Event("https_server_started"),
			zap.String("listenAddr", listenAddr),
		)

		err := httpsServer.ListenAndServeTLS(certFile, keyFile)
		if errors.Is(err, http.ErrServerClosed) {
			logger.Info("https server terminated", logfields.Event("http_server_terminated"))
			return
		}

		logger.Fatal(
			"https server terminated unexpectedly",
			logfields.Event("https_server_terminated_unexpectedly"),
			zap.Error(err),
		)
	}()
}

func startHTTPServer(listenAddr string, mux *http.ServeMux) {
	httpServer := http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	goodbye.Register(func(context.Context, os.Signal) {
		const shutdownTimeout = 30 * time.Second
		ctx, cancelFn := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancelFn()

		logger.Debug(
			"terminating http server",
			logfields.Event("http_server_terminating"),
			zap.Duration("shutdown_timeout", shutdownTimeout),
		)

		err := httpServer.Shutdown(ctx)
		if err != nil {
			logger.Warn(
				"shutting down http server failed",
				logfields.Event("http_server_termination_failed"),
				zap.Error(err),
			)
		}
	})

	go func() {
		defer panicHandler()

		logger.Info(
			"http server started",
			logfields.Event("http_server_started"),
			zap.String("listenAddr", listenAddr),
		)

		err := httpServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			logger.Info("http server terminated", logfields.Event("http_server_terminated"))
			return
		}

		logger.Fatal(
			"http server terminated unexpectedly",
			logfields.Event("http_server_terminated_unexpectedly"),
			zap.Error(err),
		)
	}()
}

type arguments struct {
	Verbose     *bool
	ConfigFile  *string
	ShowVersion *bool
}

var args arguments

const defConfigFile = "/etc/goordinator/config.toml"

func mustParseCommandlineParams() {
	args = arguments{
		Verbose: pflag.BoolP(
			"verbose",
			"v",
			false,
			"enable verbose logging",
		),
		ConfigFile: pflag.StringP(
			"cfg-file",
			"c",
			defConfigFile,
			"path to the goordinator configuration file",
		),
		ShowVersion: pflag.Bool(
			"version",
			false,
			"print the version and exit",
		),
	}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTION]\nReceive GitHub webHook events and trigger actions.\n", appName)
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		pflag.PrintDefaults()
	}

	pflag.Parse()
}

func mustParseCfg() *cfg.Config {
	// we use exitOnErr in this function instead of logger.Fatal() because
	// the logger is not initialized yet

	file, err := os.Open(*args.ConfigFile)
	exitOnErr("could not open configuration files", err)
	defer file.Close()

	config, err := cfg.Load(file)
	if err != nil {
		exitOnErr(fmt.Sprintf("could not load configuration file: %s", *args.ConfigFile), err)
	}

	return config
}

func initLogFmtLogger(config *cfg.Config, logLevel zapcore.Level) *zap.Logger {
	cfg := zapEncoderConfig(config)

	logger := zap.New(zapcore.NewCore(
		zaplogfmt.NewEncoder(cfg),
		os.Stdout,
		logLevel),
	)

	return logger
}

func zapEncoderConfig(config *cfg.Config) zapcore.EncoderConfig {
	cfg := zap.NewProductionEncoderConfig()

	cfg.LevelKey = "loglevel"
	cfg.TimeKey = config.LogTimeKey
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncodeDuration = zapcore.StringDurationEncoder

	return cfg
}

func mustInitZapFormatLogger(config *cfg.Config, logLevel zapcore.Level) *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.Sampling = nil
	cfg.EncoderConfig = zapEncoderConfig(config)
	cfg.OutputPaths = []string{"stdout"}
	cfg.Encoding = config.LogFormat
	cfg.Level = zap.NewAtomicLevelAt(logLevel)

	logger, err := cfg.Build()
	exitOnErr("could not initialize logger", err)

	return logger
}

func mustInitLogger(config *cfg.Config) {
	var logLevel zapcore.Level
	if *args.Verbose {
		logLevel = zapcore.DebugLevel
	} else {
		if err := (&logLevel).Set(config.LogLevel); err != nil {
			fmt.Fprintf(os.Stderr, "can not set log level to %q: %s \n", config.LogFormat, err)
			os.Exit(2)
		}
	}

	switch config.LogFormat {
	case "logfmt":
		logger = initLogFmtLogger(config, logLevel)
	case "console", "json":
		logger = mustInitZapFormatLogger(config, logLevel)
	default:
		fmt.Fprintf(os.Stderr, "unsupported log-format argument: %q\n", config.LogFormat)
		os.Exit(2)
	}

	logger = logger.Named("main")
	zap.ReplaceGlobals(logger)

	goodbye.Register(func(context.Context, os.Signal) {
		if err := logger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "flushing logs failed: %s\n", err)
		}
	})
}

func hide(in string) string {
	if in == "" {
		return in
	}

	return "**hidden**"
}

func main() {
	defer panicHandler()

	defer goodbye.Exit(context.Background(), 1)
	goodbye.Notify(context.Background())

	mustParseCommandlineParams()

	if *args.ShowVersion {
		fmt.Printf("%s %s\n", appName, Version)
		os.Exit(0) // nolint:gocritic // defer functions won't run
	}

	config := mustParseCfg()

	mustInitLogger(config)

	githubClient := githubclt.New(config.GithubAPIToken)

	rules, err := goordinator.RulesFromCfg(config, githubClient)
	exitOnErr(fmt.Sprintf("could not parse rules from configuration file: %s", *args.ConfigFile), err)

	logger.Info(
		"loaded cfg file",
		logfields.Event("cfg_loaded"),
		zap.String("cfg_file", *args.ConfigFile),
		zap.String("http_server_listen_addr", config.HTTPListenAddr),
		zap.String("https_server_listen_addr", config.HTTPSListenAddr),
		zap.String("github_webhook_endpoint", config.HTTPGithubWebhookEndpoint),
		zap.String("github_webhook_secret", hide(config.GithubWebHookSecret)),
		zap.String("github_api_token", hide(config.GithubAPIToken)),
		zap.String("log_format", config.LogFormat),
		zap.String("log_time_key", config.LogTimeKey),
		zap.String("log_level", config.LogLevel),
		zap.String("rules", rules.String()),
	)

	goodbye.Register(func(_ context.Context, sig os.Signal) {
		logger.Info(fmt.Sprintf("terminating, received signal %s", sig.String()))
	})

	if config.HTTPListenAddr == "" && config.HTTPSListenAddr == "" {
		fmt.Fprintf(os.Stderr, "https_server_listen_addr or http_server_listen_addr must be defined in the config file, both are unset")
		os.Exit(1)
	}

	var chans []chan<- *github.Event

	mux := http.NewServeMux()

	if len(rules) > 0 {
		evLoopchan := make(chan *github.Event, EventChannelBufferSize)
		chans = append(chans, evLoopchan)

		evLoop := goordinator.NewEventLoop(
			evLoopchan,
			rules,
			goordinator.WithActionRoutineDeferFunc(panicHandler),
		)

		go evLoop.Start() // todo: implement ordered shutdown, wait for termination
	} else {
		fmt.Fprintf(os.Stderr, "ERROR: config file %s does not define any rules, nothing to do\n", *args.ConfigFile)
		os.Exit(1)
	}

	gh := github.New(
		chans,
		github.WithPayloadSecret(config.GithubWebHookSecret),
	)

	mux.HandleFunc(config.HTTPGithubWebhookEndpoint, gh.HTTPHandler)
	logger.Info(
		"registered github webhook event http endpoint",
		logfields.Event("github_http_handler_registered"),
		zap.String("endpoint", config.HTTPGithubWebhookEndpoint),
	)

	if config.HTTPListenAddr != "" {
		startHTTPServer(config.HTTPListenAddr, mux)
	}

	if config.HTTPSListenAddr != "" {
		startHTTPSServer(
			config.HTTPSListenAddr,
			config.HTTPSCertFile,
			config.HTTPSKeyFile,
			mux,
		)
	}

	goodbye.Register(func(context.Context, os.Signal) {
		logger.Debug(
			"stopping event loop",
			logfields.Event("event_loop_stopping"),
		)
	})

	select {} // TODO: refactor this, allow clean shutdown
}
