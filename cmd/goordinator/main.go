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

	"github.com/simplesurance/goordinator/internal/autoupdate"
	"github.com/simplesurance/goordinator/internal/cfg"
	"github.com/simplesurance/goordinator/internal/githubclt"
	"github.com/simplesurance/goordinator/internal/goordinator"
	"github.com/simplesurance/goordinator/internal/logfields"
	"github.com/simplesurance/goordinator/internal/provider/github"
)

const appName = "goordinator"

var logger *zap.Logger

// Version is set via and ldflag on compilation
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
	Verbose           *bool
	ConfigFile        *string
	ShowVersion       *bool
	AutoupdaterDryRun *bool
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
		AutoupdaterDryRun: pflag.Bool(
			"autoupdater-dry-run",
			false,
			"simulate operations that would result in changes",
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
	// the logger is not intialized yet

	file, err := os.Open(*args.ConfigFile)
	exitOnErr("could not open configuration files", err)
	defer file.Close()

	config, err := cfg.Load(file)
	if err != nil {
		exitOnErr(fmt.Sprintf("could not load configuration file: %s", *args.ConfigFile), err)
	}

	return config
}

func initLogFmtLogger(config *cfg.Config) *zap.Logger {
	cfg := zapEncoderConfig(config)
	lvl := zapcore.InfoLevel

	if *args.Verbose {
		lvl = zapcore.DebugLevel
	}

	logger := zap.New(zapcore.NewCore(
		zaplogfmt.NewEncoder(cfg),
		os.Stdout,
		lvl),
	)

	return logger
}

func zapEncoderConfig(config *cfg.Config) zapcore.EncoderConfig {
	cfg := zap.NewProductionEncoderConfig()

	cfg.LevelKey = "loglevel"
	cfg.TimeKey = config.LogTimeKey
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder

	return cfg
}

func mustInitZapFormatLogger(config *cfg.Config) *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.Sampling = nil
	cfg.EncoderConfig = zapEncoderConfig(config)
	cfg.OutputPaths = []string{"stdout"}
	cfg.Encoding = config.LogFormat

	if *args.Verbose {
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}

	logger, err := cfg.Build()
	exitOnErr("could not initialize logger", err)

	return logger
}

func mustInitLogger(config *cfg.Config) {
	switch config.LogFormat {
	case "logfmt":
		logger = initLogFmtLogger(config)
	case "console", "json":
		logger = mustInitZapFormatLogger(config)
	default:
		fmt.Fprintf(os.Stderr, "unsupported log-format argument: %q\n", config.LogFormat)
		os.Exit(2)
	}

	logger = logger.Named("main")
	zap.ReplaceGlobals(logger)
}

func hide(in string) string {
	if in == "" {
		return in
	}

	return "**hidden**"
}

func startPullRequestAutoupdater(config *cfg.Config, githubClient *githubclt.Client, mux *http.ServeMux) (*autoupdate.Autoupdater, chan<- *github.Event) {
	if !config.Autoupdater.TriggerOnAutoMerge && len(config.Autoupdater.Labels) == 0 {
		logger.Info(
			"autoupdater is disabled, trigger_on_auto_merge is false and trigger_labels in config is empty",
			logfields.Event("autoupdater_disabled"),
		)

		return nil, nil
	}

	if len(config.GithubAPIToken) == 0 {
		fmt.Fprintf(os.Stderr, "ERROR: config file %s: github_api_token must be provided when autoupdater is enabled", *args.ConfigFile)
		os.Exit(1)
	}

	if len(config.HTTPGithubWebhookEndpoint) == 0 {
		fmt.Fprintf(os.Stderr, "ERROR: config file :%s: github_webhook_endpoint must be provided when autoupdater is enabled", *args.ConfigFile)
		os.Exit(1)
	}

	if len(config.Autoupdater.Repositories) == 0 {
		logger.Info("github pull request updater is disabled, repository list in config is empty")
		return nil, nil
	}

	repos := make([]autoupdate.Repository, 0, len(config.Autoupdater.Repositories))
	for _, r := range config.Autoupdater.Repositories {
		repos = append(repos, autoupdate.Repository{
			OwnerLogin:     r.Owner,
			RepositoryName: r.RepositoryName,
		})
	}

	ch := make(chan *github.Event, EventChannelBufferSize)

	autoupdater := autoupdate.NewAutoupdater(
		githubClient,
		ch,
		goordinator.NewRetryer(),
		repos,
		config.Autoupdater.TriggerOnAutoMerge,
		config.Autoupdater.Labels,
		autoupdate.DryRun(*args.AutoupdaterDryRun),
	)
	autoupdater.Start()

	if config.Autoupdater.Endpoint != "" {
		mux.HandleFunc(config.Autoupdater.Endpoint, autoupdater.HTTPHandlerList)

		logger.Info(
			"registered github pull request autoupdater http endpoint",
			logfields.Event("autoupdater_http_handler_registered"),
			zap.String("endpoint", config.Autoupdater.Endpoint),
		)
	}

	return autoupdater, ch
}

func main() {
	defer panicHandler()

	defer goodbye.Exit(context.Background(), 1)
	goodbye.Notify(context.Background())

	mustParseCommandlineParams()

	if *args.ShowVersion {
		fmt.Printf("%s %s\n", appName, Version)
		os.Exit(0)
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
		zap.String("http_server_listen_addr", config.HTTPSListenAddr),
		zap.String("https_server_listen_addr", config.HTTPListenAddr),
		zap.String("github_webhook_endpoint", config.HTTPGithubWebhookEndpoint),
		zap.String("github_webhook_secret", hide(config.GithubWebHookSecret)),
		zap.String("github_api_token", hide(config.GithubAPIToken)),
		zap.String("log_format", config.LogFormat),
		zap.String("log_time_key", config.LogTimeKey),
		zap.Bool("autoupdater.trigger_on_auto_merge", config.Autoupdater.TriggerOnAutoMerge),
		zap.Strings("autoupdater.labels", config.Autoupdater.Labels),
		zap.Any("autoupdater.repositories", config.Autoupdater.Repositories),
		zap.String("autoupdater.http_endpoint", config.Autoupdater.Endpoint),
		zap.String("rules", rules.String()),
	)

	goodbye.Register(func(_ context.Context, sig os.Signal) {
		logger.Info(fmt.Sprintf("terminating, received signal %s", sig.String()))
	})

	var chans []chan<- *github.Event

	mux := http.NewServeMux()

	autoupdater, ch := startPullRequestAutoupdater(config, githubClient, mux)
	if ch != nil {
		chans = append(chans, ch)
	}

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
		logger.Debug("No rules defined, event-loop is not started",
			logfields.Event("event_loop_not_started"),
		)

		if autoupdater == nil {
			fmt.Fprintf(os.Stderr, "ERROR: config file %s does not define any rules and autoupdater triggers are disabled, nothing to do\n", *args.ConfigFile)
			os.Exit(1)
		}
	}

	gh := github.New(
		chans,
		github.WithPayloadSecret(config.GithubWebHookSecret),
	)

	mux.HandleFunc(config.HTTPGithubWebhookEndpoint, gh.HTTPHandler)
	logger.Debug(
		"registered github webhook event handler",
		logfields.Event("github_http_handler_registered"),
		zap.String("endpoint", config.HTTPGithubWebhookEndpoint),
	)

	if config.HTTPListenAddr == "" && config.HTTPSListenAddr == "" {
		logger.Warn("https_server_listen_addr and http_server_listen_addr configuration parameters are empty, no http server is started")
	}

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

	if autoupdater != nil {
		if err := autoupdater.Sync(context.Background()); err != nil {
			logger.Error(
				"autoupdater: initial synchronization failed",
				logfields.Event("autoupdate_initial_sync_failed"),
				zap.Error(err),
			)
		}

		autoupdater.Start()
	}

	select {} // TODO: refactor this, allow clean shutdown
}
