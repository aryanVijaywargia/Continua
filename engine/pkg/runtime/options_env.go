package runtime

import (
	"io"
	"time"

	"github.com/continua-ai/continua/engine/internal/config"
)

// OptionsFromEnv loads the ENGINE_* environment settings that
// `continua-engine serve` reads and returns them as Options. The caller adds
// Workflows and Activities. Structured logs go to logOutput, or to os.Stderr
// when logOutput is nil.
func OptionsFromEnv(logOutput io.Writer) (Options, error) {
	cfg, err := config.Load()
	if err != nil {
		return Options{}, err
	}
	return Options{
		DatabaseURL:                cfg.Database.URL,
		DBMaxConns:                 cfg.Database.MaxConns,
		DBMinConns:                 cfg.Database.MinConns,
		DBMinConnsSet:              true,
		DBMaxConnLifetime:          cfg.Database.MaxConnLifetime,
		DBMaxConnIdleTime:          cfg.Database.MaxConnIdleTime,
		DBHealthCheckPeriod:        cfg.Database.HealthCheckPeriod,
		Logger:                     config.NewLogger(cfg.Logging, logOutput),
		ProjectID:                  cfg.Runtime.ProjectIDFilter,
		ProjectorBatchSize:         cfg.Runtime.ProjectorBatchSize,
		MaxChildDepth:              cfg.Runtime.MaxChildDepth,
		MaxContinuationFollowDepth: cfg.Runtime.MaxContinuationFollowDepth,
		WorkflowPollInterval:       cfg.Runtime.WorkflowPollInterval,
		ActivityPollInterval:       cfg.Runtime.ActivityPollInterval,
		MaintenancePollInterval:    cfg.Runtime.MaintenancePollInterval,
		MetricsSampleInterval:      cfg.Runtime.MetricsSampleInterval,
		RunLeaseTTL:                cfg.Runtime.RunLeaseTTL,
		ActivityLeaseTTL:           cfg.Runtime.ActivityLeaseTTL,
		DisableNotify:              !cfg.Runtime.NotifyEnabled,
		NotifyFallbackInterval:     cfg.Runtime.NotifyFallbackInterval,
		ShutdownGrace:              zeroDisablesOption(cfg.Runtime.ShutdownGrace),
		RetentionTerminalRuns:      zeroDisablesOption(cfg.Runtime.RetentionTerminalRuns),
		RetentionDedupeGrace:       zeroDisablesOption(cfg.Runtime.RetentionDedupeGrace),
		RetentionBatchSize:         cfg.Runtime.RetentionBatchSize,
		MetricsAddr:                cfg.Runtime.MetricsAddr,
		HTTPAddr:                   cfg.Runtime.HTTPAddr,
		LeaseCompletionGrace:       cfg.Runtime.LeaseCompletionGrace,
	}, nil
}

// zeroDisablesOption maps an environment value of 0 to the negative Options
// sentinel. In env config 0 means "off" (no shutdown grace, no retention
// class); in Options 0 means "use the default" and a negative value means off.
func zeroDisablesOption(configured time.Duration) time.Duration {
	if configured == 0 {
		return -1
	}
	return configured
}
