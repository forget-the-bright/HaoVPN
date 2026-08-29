package maintenance

import (
	"context"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/logstore"
	"haovpn/internal/logger"
	"haovpn/internal/persist"
	"haovpn/internal/safeutil"
)

// RunDataRetention 清理超期审计、连接事件与结构化历史日志。
//
// 参数：store — haovpn.db；logs — 可选 logs.db；cfg — 保留天数配置。
// 副作用：DELETE 超期行；成功时打 Info 日志。
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

	if days := cfg.Security.ProbeDefense.EventRetentionDays; days > 0 {
		cutoff := now.AddDate(0, 0, -days)
		n, err := store.PruneSecurityEvents(cutoff)
		if err != nil {
			logger.Warn("安全事件清理失败: %v", err)
		} else if n > 0 {
			logger.Info("已清理安全事件 %d 条（保留 %d 天）", n, days)
		}
	}
	// 过期封禁清理与事件保留天数解耦：只要有 store 就尝试停用已过期 ip_blocks。
	n2, err := store.PruneExpiredIPBlocks(now)
	if err != nil {
		logger.Warn("过期封禁清理失败: %v", err)
	} else if n2 > 0 {
		logger.Info("已停用过期封禁 %d 条", n2)
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
//
// 参数：ctx — 取消时停止 ticker；其余同 RunDataRetention。
// 副作用：经 safeutil.GoSafe 启动 goroutine（panic 可恢复）；阻塞直到 ctx 取消。
func StartRetentionLoop(ctx context.Context, store *persist.Store, logs *logstore.Store, cfg *config.ServerConfig) {
	RunDataRetention(store, logs, cfg)
	safeutil.GoSafe("data-retention", func() {
		safeutil.RunTicker(ctx, 24*time.Hour, func(context.Context) {
			RunDataRetention(store, logs, cfg)
		})
	})
}
