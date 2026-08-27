package api

import (
	"context"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/logstore"
	"haovpn/internal/logger"
	"haovpn/internal/persist"
)

// RunDataRetention 清理超期审计、连接事件与结构化日志。
func RunDataRetention(store *persist.Store, logs *logstore.Store, cfg *config.ServerConfig) {
	if store == nil || cfg == nil {
		return
	}
	now := time.Now()

	if days := cfg.Database.AuditRetentionDays; days > 0 {
		cutoff := now.AddDate(0, 0, -days)
		n, err := store.PruneAuditLogs(cutoff)
		if err != nil {
			logger.Warn("审计日志清理失败: %v", err)
		} else if n > 0 {
			logger.Info("已清理审计日志 %d 条（保留 %d 天）", n, days)
		}
	}

	if days := cfg.Database.ConnectionEventsRetentionDays; days > 0 {
		cutoff := now.AddDate(0, 0, -days)
		n, err := store.PruneConnectionEvents(cutoff)
		if err != nil {
			logger.Warn("连接事件清理失败: %v", err)
		} else if n > 0 {
			logger.Info("已清理连接事件 %d 条（保留 %d 天）", n, days)
		}
	}

	if logs != nil && cfg.HistoryLogEnabled() {
		if days := cfg.Log.HistoryRetentionDays; days > 0 {
			cutoff := now.AddDate(0, 0, -days)
			n, err := logs.Prune(cutoff)
			if err != nil {
				logger.Warn("历史日志库清理失败: %v", err)
			} else if n > 0 {
				logger.Info("已清理历史日志 %d 条（保留 %d 天）", n, days)
			}
		}
	}
}

// StartRetentionLoop 启动时执行一次并每日重复数据保留清理。
func StartRetentionLoop(ctx context.Context, store *persist.Store, logs *logstore.Store, cfg *config.ServerConfig) {
	RunDataRetention(store, logs, cfg)
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				RunDataRetention(store, logs, cfg)
			}
		}
	}()
}

// clampLimit 限制 API 分页参数。
func clampLimit(n, def, max int) int {
	if n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}
