package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"ccLoad/internal/debuganalysis"
	"ccLoad/internal/debuglog"
)

type options struct {
	inputDir        string
	outputDir       string
	logID           int64
	sinceLogID      int64
	limit           int
	force           bool
	watch           bool
	follow          bool
	interval        float64
	retentionDays   float64
	cleanupInterval float64
	cleanupBatch    int
	cleanupSleep    float64
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	if err := run(); err != nil {
		log.Printf("[ERROR] %v", err)
		os.Exit(1)
	}
}

func run() error {
	var opts options
	flag.StringVar(&opts.inputDir, "input-dir", debuglog.DirFromEnv(), "Debug log input directory")
	flag.StringVar(&opts.outputDir, "out-dir", envString("CCLOAD_DEBUG_ANALYSIS_DIR", "data/debug-analysis"), "Analysis JSON output directory")
	flag.Int64Var(&opts.logID, "log-id", 0, "Analyze one log ID")
	flag.Int64Var(&opts.sinceLogID, "since-log-id", 0, "Analyze log IDs greater than this value")
	flag.IntVar(&opts.limit, "limit", 100, "Maximum logs per scan; <=0 means unlimited")
	flag.BoolVar(&opts.force, "force", false, "Overwrite current analysis files")
	flag.BoolVar(&opts.watch, "watch", false, "Poll continuously")
	flag.BoolVar(&opts.follow, "follow", false, "Poll continuously (PM2-friendly alias)")
	flag.Float64Var(&opts.interval, "interval", envFloat("DEBUG_ANALYSIS_INTERVAL", 5), "Watch interval in seconds")
	flag.Float64Var(&opts.retentionDays, "retention-days", envFloat("DEBUG_ANALYSIS_RETENTION_DAYS", 5), "Analysis retention in days; <=0 disables cleanup")
	flag.Float64Var(&opts.cleanupInterval, "cleanup-interval", envFloat("DEBUG_ANALYSIS_CLEANUP_INTERVAL", 300), "Output cleanup interval in seconds")
	flag.IntVar(&opts.cleanupBatch, "cleanup-batch-size", envInt("DEBUG_ANALYSIS_CLEANUP_BATCH_SIZE", 500), "Maximum outputs deleted per cleanup; <=0 means unlimited")
	flag.Float64Var(&opts.cleanupSleep, "cleanup-sleep", envFloat("DEBUG_ANALYSIS_CLEANUP_SLEEP", 0.005), "Delay between output deletions in seconds")
	flag.Parse()

	opts.watch = opts.watch || opts.follow
	if opts.watch && opts.logID > 0 {
		return fmt.Errorf("--watch/--follow cannot be combined with --log-id")
	}
	if opts.interval <= 0 {
		return fmt.Errorf("--interval must be positive")
	}
	if opts.cleanupInterval <= 0 {
		return fmt.Errorf("--cleanup-interval must be positive")
	}
	if opts.cleanupSleep < 0 {
		return fmt.Errorf("--cleanup-sleep must be non-negative")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	runner := &debuganalysis.Runner{
		Store:     debuglog.NewFileStore(opts.inputDir),
		OutputDir: opts.outputDir,
	}
	if opts.logID > 0 {
		if err := runner.AnalyzeID(ctx, opts.logID); err != nil {
			return err
		}
		log.Printf("analyzed log_id=%d output=%s", opts.logID, opts.outputDir)
		return nil
	}

	lastLogID := opts.sinceLogID
	if opts.watch {
		maxOutputID, err := debuganalysis.MaxOutputLogID(opts.outputDir)
		if err != nil {
			return fmt.Errorf("scan existing analysis outputs: %w", err)
		}
		if maxOutputID > lastLogID {
			lastLogID = maxOutputID
		}
	}
	lastCleanup := time.Time{}
	for {
		now := time.Now()
		minCreatedAt := int64(0)
		if opts.retentionDays > 0 {
			retention := time.Duration(opts.retentionDays * float64(24*time.Hour))
			cutoff := now.Add(-retention)
			minCreatedAt = cutoff.Unix()
			if lastCleanup.IsZero() || !opts.watch || now.Sub(lastCleanup) >= seconds(opts.cleanupInterval) {
				removed, err := debuganalysis.CleanupOutputs(
					ctx,
					opts.outputDir,
					opts.inputDir,
					cutoff,
					opts.cleanupBatch,
					seconds(opts.cleanupSleep),
				)
				if err != nil && ctx.Err() == nil {
					return fmt.Errorf("clean analysis outputs: %w", err)
				}
				if removed > 0 {
					log.Printf("cleaned %d old analysis file(s), output=%s", removed, opts.outputDir)
				}
				lastCleanup = now
			}
		}

		batch, err := runner.AnalyzeBatch(ctx, lastLogID, minCreatedAt, opts.limit, opts.force)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if batch.MaxLogID > lastLogID {
			lastLogID = batch.MaxLogID
		}
		log.Printf("analyzed=%d skipped=%d last_log_id=%d output=%s", batch.Analyzed, batch.Skipped, lastLogID, opts.outputDir)
		if !opts.watch {
			return nil
		}

		timer := time.NewTimer(seconds(opts.interval))
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil
		}
	}
}

func seconds(value float64) time.Duration {
	return time.Duration(value * float64(time.Second))
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
