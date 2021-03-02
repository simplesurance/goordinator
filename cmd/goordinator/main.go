package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/jamiealquiza/envy"
	"github.com/simplesurance/goordinator/internal/cfg"
	"github.com/simplesurance/goordinator/internal/goordinator"
	"github.com/simplesurance/goordinator/internal/logfields"
	"github.com/simplesurance/goordinator/internal/provider/github"
	zaplogfmt "github.com/sykesm/zap-logfmt"
	"github.com/thecodeteam/goodbye"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const appName = "goordinator"

var logger *zap.Logger

// Version is set via and ldflag on compilation
var Version = "unknown"

func exitOnErr(msg string, err error) {
	if err == nil {
		return
	}

	fmt.Fprint(os.Stderr, msg, ", error: ", err.Error())
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

func startHttpServer(listenAddr string, mux *http.ServeMux) {
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
	Verbose             *bool
	LogFormat           *string
	HTTPListenAddr      *string
	GithubWebhookSecret *string
	GithubHTTPEndpoint  *string
	RulesCfgFile        *string
	ShowVersion         *bool
}

var args arguments

func mustParseCommandlineParams() {
	args = arguments{
		Verbose: flag.Bool("verbose", false, "enable verbose logging"),
		HTTPListenAddr: flag.String(
			"http-listen-addr",
			":8084",
			"address where the http server listens for requests, the http server processes github webhook events",
		),
		RulesCfgFile: flag.String("rules-file",
			"/etc/goordinator/rules.toml",
			"path to the rules.toml configuration files",
		),
		GithubWebhookSecret: flag.String("gh-webhook-secret",
			"",
			"github webhook secret\n(https://docs.github.com/en/developers/webhooks-and-events/creating-webhooks#secret)",
		),
		GithubHTTPEndpoint: flag.String("gh-webhook-endpoint",
			"/listener/github",
			"set the http endpoint that receives github webhook events",
		),
		LogFormat: flag.String("log-format",
			"logfmt",
			"define the format in that logs are printed, supported values: 'logfmt', 'json', 'plain'",
		),

		ShowVersion: flag.Bool("version",
			false,
			"print the version and exit",
		),
	}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTION]\nReceive GitHub webHook events and trigger actions.\n", appName)
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}

	envy.Parse("GR")
	flag.Parse()

	if *args.LogFormat != "logfmt" && *args.LogFormat != "plain" && *args.LogFormat != "json" {
		fmt.Fprintf(os.Stderr, "unsupported log-format argument: %q\n", *args.LogFormat)
		os.Exit(2)
	}
}

func initLogFmtLogger() *zap.Logger {
	cfg := zapEncoderConfig()
	lvl := zapcore.InfoLevel

	if *args.Verbose {
		lvl = zapcore.DebugLevel
	}

	logger := zap.New(zapcore.NewCore(
		zaplogfmt.NewEncoder(cfg),
		os.Stdout,
		lvl),
	)

	zap.ReplaceGlobals(logger)
	logger = logger.Named("main")

	return logger
}

func zapEncoderConfig() zapcore.EncoderConfig {
	cfg := zap.NewProductionEncoderConfig()

	cfg.LevelKey = "loglevel"
	cfg.TimeKey = "time_iso8601"
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder

	return cfg
}

func mustInitZapFormatLogger() *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.Sampling = nil
	cfg.EncoderConfig = zapEncoderConfig()
	cfg.OutputPaths = []string{"stdout"}
	cfg.Encoding = *args.LogFormat
	if *args.LogFormat == "plain" {
		cfg.Encoding = "console"
	}

	if *args.Verbose {
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}

	logger, err := cfg.Build()
	exitOnErr("could not initialize logger", err)

	return logger
}

func mustInitLogger() {
	if *args.LogFormat == "logfmt" {
		logger = initLogFmtLogger()
	} else {
		logger = mustInitZapFormatLogger()
	}

	logger = logger.Named("main")
	zap.ReplaceGlobals(logger)
}

func mustParseRulesCfg() goordinator.Rules {
	file, err := os.Open(*args.RulesCfgFile)
	if err != nil {
		logger.Fatal(
			"could not open rules configuration file",
			logfields.Event("rules_cfg_opening_file_failed"),
			zap.String("cfg_file", *args.RulesCfgFile),
			zap.Error(err),
		)
	}
	defer file.Close()

	rulesCfg, err := cfg.LoadRules(file)
	if err != nil {
		logger.Fatal("could not load rules cfg file",
			logfields.Event("rules_cfg_loading_failed"),
			zap.String("cfg_file", *args.RulesCfgFile),
			zap.Error(err),
		)
	}

	rules, err := goordinator.RulesFromCfg(rulesCfg)
	if err != nil {
		logger.Fatal("could not parse rules cfg file",
			logfields.Event("rules_cfg_parsing_failed"),
			zap.String("cfg_file", *args.RulesCfgFile),
			zap.Error(err),
		)
	}

	logger.Info(
		"loaded rules from cfg file",
		logfields.Event("rules_cfg_loaded"),
		zap.String("cfg_file", *args.RulesCfgFile),
		zap.String("rules", rules.String()),
	)

	return rules
}

func main() {
	defer panicHandler()

	mustParseCommandlineParams()

	if *args.ShowVersion {
		fmt.Printf("%s %s\n", appName, Version)
		os.Exit(0)
	}

	mustInitLogger()

	defer goodbye.Exit(context.Background(), 1)
	goodbye.Notify(context.Background())

	goodbye.Register(func(_ context.Context, sig os.Signal) {
		logger.Info(fmt.Sprintf("terminating, received signal %s", sig.String()))
	})

	rules := mustParseRulesCfg()

	evLoop := goordinator.NewEventLoop(
		rules,
		goordinator.WithActionRoutineDeferFunc(panicHandler),
	)

	gh := github.New(
		evLoop.C(),
		github.WithPayloadSecret(*args.GithubWebhookSecret),
	)

	mux := http.NewServeMux()
	mux.HandleFunc(*args.GithubHTTPEndpoint, gh.HttpHandler)
	logger.Debug(
		"registered github webhook event handler",
		logfields.Event("github_http_handler_registered"),
		zap.String("endpoint", *args.GithubHTTPEndpoint),
	)
	startHttpServer(*args.HTTPListenAddr, mux)

	goodbye.Register(func(context.Context, os.Signal) {
		logger.Debug(
			"stopping event loop",
			logfields.Event("event_loop_stopping"),
		)
	})

	evLoop.Start()
}
