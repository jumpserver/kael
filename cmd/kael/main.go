package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jumpserver/kael/internal/api"
	"github.com/jumpserver/kael/internal/component"
	"github.com/jumpserver/kael/internal/config"
	"github.com/jumpserver/kael/internal/event"
	"github.com/jumpserver/kael/internal/identity"
	"github.com/jumpserver/kael/internal/model"
	"github.com/jumpserver/kael/internal/platformgateway"
	"github.com/jumpserver/kael/internal/ports"
	"github.com/jumpserver/kael/internal/service"
	"github.com/jumpserver/kael/internal/store"
	"go.uber.org/zap"
)

var (
	Version    = "dev"
	Buildstamp = "unknown"
	Githash    = "unknown"
	Goversion  = "unknown"
)

func main() {
	configPath := flag.String("f", "", "path to the Kael YAML configuration")
	showVersion := flag.Bool("version", false, "print build information")
	flag.Parse()
	if *showVersion {
		fmt.Printf("kael %s (%s, %s, %s)\n", Version, Githash, Buildstamp, Goversion)
		return
	}
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintln(os.Stderr, "create logger:", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()
	if err = run(*configPath, logger); err != nil {
		logger.Fatal("kael stopped", zap.Error(err))
	}
}

func run(configPath string, logger *zap.Logger) error {
	settings, err := config.Load(configPath)
	if err != nil {
		return err
	}
	tlsVerify := !settings.IgnoreVerifyCerts
	componentClient, err := component.Connect(component.Options{CoreURL: settings.CoreHost, TLSVerify: tlsVerify, Timeout: settings.HTTPRequestTimeout, Name: settings.Name, BootstrapToken: settings.BootstrapToken, AccessKeyFile: settings.AccessKeyFilePath})
	if err != nil {
		return err
	}
	provider, err := model.NewDynamicProvider(context.Background(), componentClient.ModelConfig, time.Minute)
	if err != nil {
		return err
	}
	var runtimeStore ports.Store
	switch settings.RuntimeStore {
	case "core":
		runtimeStore, err = store.NewCore(componentClient)
	case "jsonl":
		runtimeStore, err = store.NewJSONL(settings.RuntimeDataFolderPath)
	default:
		return fmt.Errorf("unsupported runtime store %q", settings.RuntimeStore)
	}
	if err != nil {
		return err
	}
	defer runtimeStore.Close()
	authenticator := identity.NewCoreAuthenticator(settings.CoreHost, tlsVerify, 15*time.Second)
	bus := event.NewBus()
	var capability ports.CapabilityProvider
	if settings.PlatformGatewayEnabled {
		capability, err = platformgateway.New(platformgateway.Config{CoreURL: settings.CoreHost, CoreTLSVerify: tlsVerify, DelegationKey: settings.PlatformDelegationKey, DelegationKeyID: settings.PlatformDelegationID, Issuer: settings.PlatformIssuer, Audience: settings.PlatformAudience, CACert: settings.PlatformCACert, ClientCert: settings.PlatformClientCert, ClientKey: settings.PlatformClientKey, AllowedMethods: settings.PlatformAllowedMethods, RegistryTTL: settings.PlatformRegistryTTL, Timeout: settings.PlatformTimeout, MaxResponse: settings.PlatformMaxResponse, OpenAPILoader: componentClient.OpenAPISchema})
		if err != nil {
			return err
		}
	}
	runtimeService, err := service.New(service.Options{Store: runtimeStore, Provider: provider, Bus: bus, Logger: logger, InstanceID: settings.Name, Workers: 4, ArtifactDir: settings.ArtifactFolderPath, Capability: capability, StorageKind: settings.RuntimeStore, StorageDurable: true})
	if err != nil {
		return err
	}
	startup, cancelStartup := context.WithTimeout(context.Background(), 2*time.Minute)
	err = runtimeService.Start(startup)
	cancelStartup()
	if err != nil {
		return err
	}
	defer runtimeService.Close()
	apiServer, err := api.New(api.Options{Service: runtimeService, Authenticator: authenticator, Origin: identity.NewOriginVerifier(settings.AllowedOrigins, settings.TrustForwardedHeaders), Logger: logger})
	if err != nil {
		return err
	}
	httpServer := &http.Server{Addr: fmt.Sprintf("%s:%d", settings.BindHost, settings.HTTPPort), Handler: apiServer.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 5 * time.Minute, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1024 * 1024}
	cancelRequests := bindServerContext(httpServer)
	defer cancelRequests()
	serverError := make(chan error, 1)
	go func() { serverError <- httpServer.ListenAndServe() }()
	heartbeatContext, cancelHeartbeat := context.WithCancel(context.Background())
	defer cancelHeartbeat()
	go componentClient.RunHeartbeat(heartbeatContext, logger)
	logger.Info("kael started", zap.String("address", httpServer.Addr), zap.String("version", Version), zap.String("component", settings.Name), zap.String("model", provider.Info().Model), zap.String("storage", settings.RuntimeStore))
	signals, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var serveErr error
	select {
	case <-signals.Done():
		stop()
		logger.Info("kael stopping")
	case serveErr = <-serverError:
	}
	cancelRequests()
	cancelHeartbeat()
	shutdownErr := shutdownHTTPServer(httpServer)
	runtimeService.Close()
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return shutdownErr
}

func bindServerContext(server *http.Server) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	server.BaseContext = func(net.Listener) context.Context { return ctx }
	return cancel
}

func shutdownHTTPServer(server *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := server.Shutdown(ctx)
	if errors.Is(err, context.DeadlineExceeded) {
		_ = server.Close()
	}
	return err
}
