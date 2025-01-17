package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/skip-mev/slinky/providers/apis/marketmap"

	_ "net/http/pprof" //nolint: gosec

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	cmdconfig "github.com/skip-mev/slinky/cmd/slinky/config"
	"github.com/skip-mev/slinky/oracle"
	"github.com/skip-mev/slinky/oracle/config"

	"github.com/skip-mev/slinky/cmd/build"
	oraclemetrics "github.com/skip-mev/slinky/oracle/metrics"
	"github.com/skip-mev/slinky/oracle/orchestrator"
	"github.com/skip-mev/slinky/pkg/log"
	oraclemath "github.com/skip-mev/slinky/pkg/math/oracle"
	oraclefactory "github.com/skip-mev/slinky/providers/factories/oracle"
	mmservicetypes "github.com/skip-mev/slinky/service/clients/marketmap/types"
	oracleserver "github.com/skip-mev/slinky/service/servers/oracle"
	promserver "github.com/skip-mev/slinky/service/servers/prometheus"
	mmtypes "github.com/skip-mev/slinky/x/marketmap/types"
)

var (
	rootCmd = &cobra.Command{
		Use:   "oracle",
		Short: "Run the slinky oracle server.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runOracle()
		},
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version of the oracle.",
		Args:  cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println(build.Build)
		},
	}

	oracleCfgPath       string
	legacyOracleCfgPath string
	marketCfgPath       string
	marketMapProvider   string
	updateMarketCfgPath string
	runPprof            bool
	profilePort         string
	logLevel            string
	fileLogLevel        string
	writeLogsTo         string
	marketMapEndPoint   string
	maxLogSize          int
	maxBackups          int
	maxAge              int
	disableCompressLogs bool
	disableRotatingLogs bool
)

const (
	DefaultLegacyConfigPath = "./oracle.json"
)

func init() {
	rootCmd.Flags().StringVarP(
		&marketMapProvider,
		"marketmap-provider",
		"",
		marketmap.Name,
		"MarketMap provider to use (marketmap_api, dydx_api).",
	)
	rootCmd.Flags().StringVarP(
		&legacyOracleCfgPath,
		"oracle-config-path",
		"",
		"",
		"Path to the legacy oracle config file.",
	)
	rootCmd.Flags().StringVarP(
		&oracleCfgPath,
		"oracle-config",
		"",
		"",
		"Path to the oracle config file.",
	)
	rootCmd.Flags().StringVarP(
		&marketCfgPath,
		"market-config-path",
		"",
		"",
		"Path to the market config file. If you supplied a node URL in your config, this will not be required.",
	)
	rootCmd.Flags().StringVarP(
		&updateMarketCfgPath,
		"update-market-config-path",
		"",
		"",
		"Path where the current market config will be written. Overwrites any pre-existing file. Requires an http-node-url/marketmap provider in your oracle.json config.",
	)
	rootCmd.Flags().BoolVarP(
		&runPprof,
		"run-pprof",
		"",
		false,
		"Run pprof server.",
	)
	rootCmd.Flags().StringVarP(
		&profilePort,
		"pprof-port",
		"",
		"6060",
		"Port for the pprof server to listen on.",
	)
	rootCmd.Flags().StringVarP(
		&logLevel,
		"log-std-out-level",
		"",
		"info",
		"Log level (debug, info, warn, error, dpanic, panic, fatal).",
	)
	rootCmd.Flags().StringVarP(
		&fileLogLevel,
		"log-file-level",
		"",
		"info",
		"Log level for the file logger (debug, info, warn, error, dpanic, panic, fatal).",
	)
	rootCmd.Flags().StringVarP(
		&writeLogsTo,
		"log-file",
		"",
		"sidecar.log",
		"Write logs to a file.",
	)
	rootCmd.Flags().IntVarP(
		&maxLogSize,
		"log-max-size",
		"",
		100,
		"Maximum size in megabytes before log is rotated.",
	)
	rootCmd.Flags().IntVarP(
		&maxBackups,
		"log-max-backups",
		"",
		1,
		"Maximum number of old log files to retain.",
	)
	rootCmd.Flags().IntVarP(
		&maxAge,
		"log-max-age",
		"",
		3,
		"Maximum number of days to retain an old log file.",
	)
	rootCmd.Flags().BoolVarP(
		&disableCompressLogs,
		"log-file-disable-compression",
		"",
		false,
		"Compress rotated log files.",
	)
	rootCmd.Flags().BoolVarP(
		&disableRotatingLogs,
		"log-disable-file-rotation",
		"",
		false,
		"Disable writing logs to a file.",
	)
	rootCmd.Flags().StringVarP(
		&marketMapEndPoint,
		"market-map-endpoint",
		"",
		"",
		"Use a custom listen-to endpoint for market-map (overwrites what is provided in oracle-config).",
	)
	rootCmd.MarkFlagsMutuallyExclusive("update-market-config-path", "market-config-path")
	rootCmd.MarkFlagsMutuallyExclusive("market-map-endpoint", "market-config-path")

	rootCmd.AddCommand(versionCmd)
}

// start the oracle-grpc server + oracle process, cancel on interrupt or terminate.
func main() {
	rootCmd.Execute()
}

func runOracle() error {
	// channel with width for either signal
	sigs := make(chan os.Signal, 1)

	// gracefully trigger close on interrupt or terminate signals
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up logging.
	logCfg := log.NewDefaultConfig()
	logCfg.StdOutLogLevel = logLevel
	logCfg.FileOutLogLevel = fileLogLevel
	logCfg.DisableRotating = disableRotatingLogs
	logCfg.WriteTo = writeLogsTo
	logCfg.MaxSize = maxLogSize
	logCfg.MaxBackups = maxBackups
	logCfg.MaxAge = maxAge
	logCfg.Compress = !disableCompressLogs

	// Build logger.
	logger := log.NewLogger(logCfg)
	defer logger.Sync()

	var cfg config.OracleConfig
	var err error

	if legacyPath, legacyConfigInUse := useLegacyOracleConfig(logger); legacyConfigInUse {
		cfg, err = cmdconfig.GetLegacyOracleConfig(legacyPath)
		if err != nil {
			return fmt.Errorf("failed to read legacy oracle config file: %w", err)
		}
	} else {
		cfg, err = cmdconfig.ReadOracleConfigWithOverrides(oracleCfgPath, marketMapProvider)
		if err != nil {
			return fmt.Errorf("failed to get oracle config: %w", err)
		}
	}

	// overwrite endpoint
	if marketMapEndPoint != "" {
		cfg, err = overwriteMarketMapEndpoint(cfg, marketMapEndPoint)
		if err != nil {
			return fmt.Errorf("failed to overwrite market endpoint %s: %w", marketMapEndPoint, err)
		}
	}

	var marketCfg mmtypes.MarketMap
	if marketCfgPath != "" {
		marketCfg, err = mmtypes.ReadMarketMapFromFile(marketCfgPath)
		if err != nil {
			return fmt.Errorf("failed to read market config file: %w", err)
		}
	}

	logger.Info(
		"successfully read in configs",
		zap.String("oracle_config_path", oracleCfgPath),
		zap.String("market_config_path", marketCfgPath),
	)

	metrics := oraclemetrics.NewMetricsFromConfig(cfg.Metrics)
	aggregator, err := oraclemath.NewIndexPriceAggregator(
		logger,
		marketCfg,
		metrics,
	)
	if err != nil {
		return fmt.Errorf("failed to create data aggregator: %w", err)
	}

	// Define the orchestrator and oracle options. These determine how the orchestrator and oracle are created & executed.
	orchestratorOpts := []orchestrator.Option{
		orchestrator.WithLogger(logger),
		orchestrator.WithMarketMap(marketCfg),
		orchestrator.WithPriceAPIQueryHandlerFactory(oraclefactory.APIQueryHandlerFactory),             // Replace with custom API query handler factory.
		orchestrator.WithPriceWebSocketQueryHandlerFactory(oraclefactory.WebSocketQueryHandlerFactory), // Replace with custom websocket query handler factory.
		orchestrator.WithMarketMapperFactory(oraclefactory.MarketMapProviderFactory),
		orchestrator.WithAggregator(aggregator),
	}
	if updateMarketCfgPath != "" {
		orchestratorOpts = append(orchestratorOpts, orchestrator.WithWriteTo(updateMarketCfgPath))
	}
	oracleOpts := []oracle.Option{
		oracle.WithLogger(logger),
		oracle.WithUpdateInterval(cfg.UpdateInterval),
		oracle.WithMetrics(metrics),
		oracle.WithMaxCacheAge(cfg.MaxPriceAge),
		oracle.WithPriceAggregator(aggregator),
	}

	// Create the orchestrator and start the orchestrator.
	orch, err := orchestrator.NewProviderOrchestrator(
		cfg,
		orchestratorOpts...,
	)
	if err != nil {
		return fmt.Errorf("failed to create provider orchestrator: %w", err)
	}

	if err := orch.Start(ctx); err != nil {
		return fmt.Errorf("failed to start provider orchestrator: %w", err)
	}
	defer orch.Stop()

	// Create the oracle and start the oracle server.
	oracleOpts = append(oracleOpts, oracle.WithProviders(orch.GetPriceProviders()))
	orc, err := oracle.New(oracleOpts...)
	if err != nil {
		return fmt.Errorf("failed to create oracle: %w", err)
	}
	srv := oracleserver.NewOracleServer(orc, logger)

	// cancel oracle on interrupt or terminate
	go func() {
		<-sigs
		logger.Info("received interrupt or terminate signal; closing oracle")

		cancel()
	}()

	// start prometheus metrics
	if cfg.Metrics.Enabled {
		logger.Info("starting prometheus metrics", zap.String("address", cfg.Metrics.PrometheusServerAddress))
		ps, err := promserver.NewPrometheusServer(cfg.Metrics.PrometheusServerAddress, logger)
		if err != nil {
			return fmt.Errorf("failed to start prometheus metrics: %w", err)
		}

		go ps.Start()

		// close server on shut-down
		go func() {
			<-ctx.Done()
			logger.Info("stopping prometheus metrics")
			ps.Close()
		}()
	}

	if runPprof {
		endpoint := fmt.Sprintf("%s:%s", cfg.Host, profilePort)
		// Start pprof server
		go func() {
			logger.Info("Starting pprof server", zap.String("endpoint", endpoint))
			if err := http.ListenAndServe(endpoint, nil); err != nil { //nolint: gosec
				logger.Error("pprof server failed", zap.Error(err))
			}
		}()
	}

	// start oracle + server, and wait for either to finish
	if err := srv.StartServer(ctx, cfg.Host, cfg.Port); err != nil {
		logger.Error("stopping server", zap.Error(err))
	}
	return nil
}

// useLegacyOracleConfig returns true if a legacy oracle config should be used
// based on the provided flags.
func useLegacyOracleConfig(logger *zap.Logger) (string, bool) {
	// if --oracle-config has been specified, use that
	if oracleCfgPath != "" {
		return oracleCfgPath, false
	}

	// if a value is provided for the --oracle-config-path flag, use it
	if legacyOracleCfgPath != "" {
		logger.Warn("DEPRECATION WARNING:: The --oracle-config-path flag is deprecated and will be removed in v1.0.0. Please use --default-config --oracle-config instead.")
		return legacyOracleCfgPath, true
	}

	// if a legacy oracle config exists at the default path, use it
	if legacyOracleConfigExists() {
		logger.Warn(
			"DEPRECATION WARNING:: Neither --oracle-config-path, nor --oracle-config has been specified, unmarshalling the oracle.json in the working directory as a legacy config. NOTE: this behavior will be deprecated in v1.0.0, either point to config overrides via --oracle-config, or remove oracle.json + specify config overrides via environment variables.",
			zap.String("path", DefaultLegacyConfigPath),
		)
		return DefaultLegacyConfigPath, true
	}

	return "", false
}

// legacyOracleConfigExists checks if the legacy oracle config file exists at DefaultLegacyConfigPath.
func legacyOracleConfigExists() bool {
	_, err := os.Stat(DefaultLegacyConfigPath)
	return !os.IsNotExist(err)
}

func overwriteMarketMapEndpoint(cfg config.OracleConfig, overwrite string) (config.OracleConfig, error) {
	for i, provider := range cfg.Providers {
		if provider.Type == mmservicetypes.ConfigType {
			provider.API.URL = overwrite
			cfg.Providers[i] = provider
			return cfg, cfg.ValidateBasic()
		}
	}

	return cfg, fmt.Errorf("no market-map provider found in config")
}
