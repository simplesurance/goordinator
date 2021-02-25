package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/simplesurance/goordinator/internal/cfg"
	"github.com/simplesurance/goordinator/internal/goordinator"
	"github.com/simplesurance/goordinator/internal/logfields"
	"github.com/simplesurance/goordinator/internal/provider/github"
	"github.com/spf13/pflag"
	"github.com/thecodeteam/goodbye"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const appName = "goordinator"

func exitOnErr(msg string, err error) {
	if err == nil {
		return
	}

	fmt.Fprint(os.Stderr, msg, ", error: ", err.Error())
	os.Exit(1)
}

var logger *zap.Logger

func startHttpServer(listenAddr string, mux *http.ServeMux) {
	httpServer := http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	goodbye.Register(func(context.Context, os.Signal) {
		const shutdownTimeout = time.Minute
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
}

var args arguments

func mustParseCommandlineParams() {
	args = arguments{
		Verbose: pflag.BoolP("verbose", "v", false, "enable verbose logging"),
		HTTPListenAddr: pflag.StringP(
			"http-listen-addr", "l",
			":8084",
			"address where the http server listens for requests, the http server processes github webhook events",
		),
		RulesCfgFile: pflag.StringP("rules-file", "f",
			"/etc/goordinator/rules.toml",
			"path to the rules.toml configuration files",
		),
		GithubWebhookSecret: pflag.String("gh-webhook-secret",
			"",
			"github webhook secret\n(https://docs.github.com/en/developers/webhooks-and-events/creating-webhooks#secret)",
		),
		GithubHTTPEndpoint: pflag.String("gh-webhook-endpoint",
			github.DefaultEndpoint,
			"set the http endpoint that receives github webhook events",
		),
		LogFormat: pflag.String("log-format",
			"json",
			"define the format in that logs are printed, supported values: 'json', 'plain'",
		),
	}

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTION]\nReceive GitHub webHook events and trigger actions.\n", appName)
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		pflag.PrintDefaults()
	}

	pflag.Parse()

	if *args.LogFormat != "plain" && *args.LogFormat != "json" {
		fmt.Fprintf(os.Stderr, "unsupported log-format argument: %q\n", *args.LogFormat)
		os.Exit(2)
	}
}

func mustInitLogger() {
	var err error

	cfg := zap.NewProductionConfig()
	cfg.Sampling = nil
	cfg.EncoderConfig.LevelKey = "loglevel"
	cfg.EncoderConfig.TimeKey = "time_iso8601"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.OutputPaths = []string{"stdout"}
	cfg.Encoding = *args.LogFormat
	if *args.LogFormat == "plain" {
		cfg.Encoding = "console"
	}

	if *args.Verbose {
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}

	logger, err = cfg.Build()
	exitOnErr("could not initialize logger", err)

	zap.ReplaceGlobals(logger)
	logger = logger.Named("main")
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

	mustParseCommandlineParams()

	mustInitLogger()

	defer goodbye.Exit(context.Background(), 1)
	goodbye.Notify(context.Background())

	goodbye.Register(func(_ context.Context, sig os.Signal) {
		logger.Info(fmt.Sprintf("terminating, received signal %s", sig.String()))
	})

	rules := mustParseRulesCfg()

	evLoop := goordinator.NewEventLoop(rules)

	mux := http.NewServeMux()
	startHttpServer(*args.HTTPListenAddr, mux)

	gh := github.New(
		evLoop.C(),
		github.WithPayloadSecret(*args.GithubWebhookSecret),
		github.WithCustomEndpoint(*args.GithubHTTPEndpoint),
	)
	gh.RegisterHTTPHandler(mux)

	goodbye.Register(func(context.Context, os.Signal) {
		logger.Debug(
			"stopping event loop",
			logfields.Event("event_loop_stopping"),
		)
	})

	evLoop.Start()
}
