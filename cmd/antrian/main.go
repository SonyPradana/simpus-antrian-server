package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/SonyPradana/simpus-antrian-server/internal/config"
	"github.com/SonyPradana/simpus-antrian-server/internal/db"
	"github.com/SonyPradana/simpus-antrian-server/internal/httpapi"
	"github.com/SonyPradana/simpus-antrian-server/internal/model"
	"github.com/SonyPradana/simpus-antrian-server/internal/service"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return serve(os.Args[1:])
	}
	switch os.Args[1] {
	case "serve":
		return serve(os.Args[2:])
	case "poli":
		return listPoli(os.Args[2:])
	case "dokter":
		return listDoctors(os.Args[2:])
	case "lookup":
		return lookup(os.Args[2:])
	case "register":
		return register(os.Args[2:])
	case "options":
		return registrationOptions(os.Args[2:])
	case "empty":
		return queueOnly(os.Args[2:], "empty")
	case "new":
		return queueOnly(os.Args[2:], "new")
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: antrian <serve|poli|dokter|lookup|register|options|empty|new> [flags]")
}

func newApp(ctx context.Context) (*service.Service, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	conn, err := db.Open(ctx, cfg.DBDSN)
	if err != nil {
		return nil, nil, err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	svc, err := service.New(conn, cfg, logger)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return svc, func() { conn.Close() }, nil
}

func serve(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := flags.String("addr", "", "HTTP listen address")
	if err := flags.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	svc, closeApp, err := newApp(ctx)
	if err != nil {
		return err
	}
	defer closeApp()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if *addr != "" {
		cfg.Addr = *addr
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.New(svc, logger, cfg.APMAPIKey, cfg.CORSOrigin).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Info("http server started", "addr", cfg.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func listPoli(args []string) error {
	flags := flag.NewFlagSet("poli", flag.ContinueOnError)
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	svc, closeApp, err := newApp(ctx)
	if err != nil {
		return err
	}
	defer closeApp()

	items, err := svc.ListPoli(ctx)
	if err != nil {
		return err
	}
	return printJSON(items)
}

func listDoctors(args []string) error {
	flags := flag.NewFlagSet("dokter", flag.ContinueOnError)
	poliID := flags.String("poli", "", "poli id")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	svc, closeApp, err := newApp(ctx)
	if err != nil {
		return err
	}
	defer closeApp()

	result, err := svc.ListDoctors(ctx, *poliID)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func lookup(args []string) error {
	flags := flag.NewFlagSet("lookup", flag.ContinueOnError)
	nik := flags.String("nik", "", "NIK")
	noBPJS := flags.String("no-bpjs", "", "BPJS card number")
	noRM := flags.String("no-rm", "", "medical record number")
	pcare := flags.Bool("pcare", true, "verify BPJS via PCare")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	svc, closeApp, err := newApp(ctx)
	if err != nil {
		return err
	}
	defer closeApp()

	result, err := svc.LookupPatient(ctx, service.LookupRequest{NIK: *nik, NoBPJS: *noBPJS, NoRM: *noRM, PCare: pcare})
	if err != nil {
		return err
	}
	return printJSON(result)
}

func register(args []string) error {
	flags := flag.NewFlagSet("register", flag.ContinueOnError)
	nik := flags.String("nik", "", "NIK")
	noBPJS := flags.String("no-bpjs", "", "BPJS card number")
	noRM := flags.String("no-rm", "", "medical record number")
	poliID := flags.String("poli", "", "poli id")
	doctor := flags.String("dokter", "", "doctor code")
	dryRun := flags.Bool("dry-run", false, "preview only")
	pcare := flags.Bool("pcare", true, "verify BPJS via PCare")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	svc, closeApp, err := newApp(ctx)
	if err != nil {
		return err
	}
	defer closeApp()

	result, err := svc.Register(ctx, service.RegisterRequest{
		NIK:        *nik,
		NoBPJS:     *noBPJS,
		NoRM:       *noRM,
		PoliID:     *poliID,
		DoctorCode: *doctor,
		DryRun:     *dryRun,
		PCare:      pcare,
	})
	if err != nil {
		var already *service.AlreadyRegisteredError
		if errors.As(err, &already) {
			_ = printJSON(already.Result)
		}
		return err
	}
	return printJSON(result)
}

func registrationOptions(args []string) error {
	flags := flag.NewFlagSet("options", flag.ContinueOnError)
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	svc, closeApp, err := newApp(ctx)
	if err != nil {
		return err
	}
	defer closeApp()

	options, err := svc.RegistrationOptions(ctx)
	if err != nil {
		return err
	}
	return printJSON(options)
}

func queueOnly(args []string, kind string) error {
	flags := flag.NewFlagSet(kind, flag.ContinueOnError)
	poliID := flags.String("poli", "", "poli id")
	doctor := flags.String("dokter", "", "doctor code")
	dryRun := flags.Bool("dry-run", false, "preview only")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	svc, closeApp, err := newApp(ctx)
	if err != nil {
		return err
	}
	defer closeApp()

	req := service.QueueOnlyRequest{
		PoliID:     *poliID,
		DoctorCode: *doctor,
		DryRun:     *dryRun,
	}
	var result model.QueueOnlyResult
	if kind == "new" {
		result, err = svc.CreateNewQueue(ctx, req)
	} else {
		result, err = svc.CreateEmptyQueue(ctx, req)
	}
	if err != nil {
		return err
	}
	return printJSON(result)
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
