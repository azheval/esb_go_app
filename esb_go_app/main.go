package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"esb-go-app/admin"
	"esb-go-app/api"
	"esb-go-app/collector"
	"esb-go-app/config"
	"esb-go-app/logger"
	"esb-go-app/metrics"
	"esb-go-app/rabbitmq"
	"esb-go-app/scripting"
	"esb-go-app/storage"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robfig/cron/v3"
)

var version = "2.0.0"

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	log, err := logger.New(cfg.LogDir, version, cfg.LogLevel)
	if err != nil {
		slog.Error("failed to setup logger", "error", err)
		os.Exit(1)
	}
	log.Info("logger initialized successfully")
	log.Info("config loaded", "port", cfg.Port, "log_dir", cfg.LogDir, "db_path", cfg.DBPath, "rabbitmq_dsn", cfg.RabbitMQ.DSN)

	dataStore, err := storage.NewStore(cfg.DBPath, log)
	if err != nil {
		log.Error("failed to create data store", "error", err)
		os.Exit(1)
	}
	defer dataStore.Close()
	log.Info("data store initialized")

	scriptingHTTPClient := scripting.NewHTTPClient(log)
	scriptingService := scripting.NewService(log, scriptingHTTPClient, dataStore)

	rmq, err := rabbitmq.New(&cfg.RabbitMQ, log, dataStore, scriptingService)
	if err != nil {
		log.Error("failed to connect to rabbitmq", "error", err)
		os.Exit(1)
	}
	defer rmq.Close()

	collectorService := collector.NewService(dataStore, scriptingService, rmq, log)

	log.Info("initializing workers for existing channels...")
	apps, err := dataStore.GetAllApplications()
	if err != nil {
		log.Error("failed to get applications for worker init", "error", err)
	} else {
		for _, app := range apps {
			channels, err := dataStore.GetChannelsByAppID(app.ID)
			if err != nil {
				log.Error("failed to get channels for worker init", "app_id", app.ID, "error", err)
				continue
			}
			for _, ch := range channels {
				log.Info("setting up topology and starting worker on boot", "channel_name", ch.Name, "destination", ch.Destination, "direction", ch.Direction)
				if err := rmq.SetupDurableTopology(ch.Destination); err != nil {
					log.Error("failed to setup durable topology on boot", "channel_name", ch.Name, "error", err)
					continue
				}

				if ch.Direction == "inbound" {
					rmq.StartInboundForwarder(ch.Destination)
				} else if ch.Direction == "outbound" {
					rmq.StartOutboundCollector(ch.Destination)
				} else {
					log.Warn("unknown channel direction, no worker started", "channel_name", ch.Name, "direction", ch.Direction)
				}
			}
		}
	}
	log.Info("worker initialization complete")

	log.Info("initializing routers for existing routes...")
	routes, err := dataStore.GetAllRoutes()
	if err != nil {
		log.Error("failed to get routes for router init", "error", err)
	} else {
		for _, route := range routes {
			log.Info("starting router worker", "route_id", route.ID, "source_id", route.SourceChannelID, "route_type", route.RouteType, "route_name", route.Name)
			if route.SourceChannelID != "" {
				rmq.StartRouter(route.ID, route.Name, route.SourceChannelID)
			} else {
				log.Error("route missing source channel ID, skipping router start", "route_id", route.ID)
			}
		}
	}
	log.Info("router initialization complete")

	log.Info("initializing collectors...")
	c := cron.New()
	collectors, err := dataStore.GetAllCollectors()
	if err != nil {
		log.Error("failed to get collectors", "error", err)
	} else {
		for _, coll := range collectors {
			// Capture the collector in a local variable for the closure
			collectorToRun := coll
			_, err := c.AddFunc(collectorToRun.Schedule, func() {
				collectorService.RunCollector(collectorToRun.ID)
			})
			if err != nil {
				log.Error("failed to add collector to scheduler", "collector_id", collectorToRun.ID, "collector_name", collectorToRun.Name, "error", err)
			}
		}
	}
	c.Start()
	log.Info("collectors scheduled", "count", len(collectors))

	mux := http.NewServeMux()
	adminHandler := admin.NewHandler(dataStore, rmq, log, scriptingService, version)
	apiHandler := api.NewHandler(dataStore, rmq, log, scriptingService)

	metrics.Register()

	mux.Handle("/admin", adminHandler)
	mux.Handle("/admin/", adminHandler)
	mux.Handle("/auth/oidc/token", apiHandler)
	mux.Handle("/applications/", apiHandler)
	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintln(w, "Go 1C:ESB Fake API is running. Visit /admin to configure.")
	})

	log.Info("starting server", "port", cfg.Port)
	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("server failed to start", "error", err)
		os.Exit(1)
	}
}
